package proxy

import (
	"encoding/base64"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/conn"
	"github.com/djylb/nps/lib/file"
)

// ---------------------------------------------------------------------------
// end to end: HTTP CONNECT and SOCKS5 must behave identically
// ---------------------------------------------------------------------------

// fakeBridge captures the client chosen by the unified proxy and hands back a
// live pipe so the data path can be exercised.
type fakeBridge struct {
	mu        sync.Mutex
	chosenIDs []int
	target    net.Conn
}

func (b *fakeBridge) IsServer() bool { return true }

func (b *fakeBridge) SendLinkInfo(clientId int, link *conn.Link, t *file.Tunnel) (net.Conn, error) {
	b.mu.Lock()
	b.chosenIDs = append(b.chosenIDs, clientId)
	b.mu.Unlock()
	return b.target, nil
}

func (b *fakeBridge) CliProcess(c *conn.Conn, tunnelType string) {}

func (b *fakeBridge) picked() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]int, len(b.chosenIDs))
	copy(out, b.chosenIDs)
	return out
}

// waitPicked polls until at least n clients have been recorded by the bridge.
func (b *fakeBridge) waitPicked(t *testing.T, n int) []int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		ids := b.picked()
		if len(ids) >= n {
			return ids
		}
		if time.Now().After(deadline) {
			t.Fatalf("bridge never picked %d client(s), got %v", n, ids)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newUnifiedE2EServer prepares a unified proxy server with two tagged clients.
func newUnifiedE2EServer(t *testing.T, password string) (*TunnelModeServer, *fakeBridge) {
	t.Helper()

	db := file.GetDb()
	clients := []*file.Client{
		newTestClient(9001, "gz"),
		newTestClient(9002, "jp"),
	}
	for _, c := range clients {
		c.Cnf = &file.Config{Crypt: false, Compress: false}
		c.Rate = nil
		db.JsonDb.Clients.Store(c.Id, c)
	}
	t.Cleanup(func() {
		for _, c := range clients {
			db.JsonDb.Clients.Delete(c.Id)
		}
	})

	bridge := &fakeBridge{}
	task := &file.Tunnel{
		Id:        4242,
		Mode:      "unifiedProxy",
		Password:  password,
		CacheTime: 10,
		Flow:      new(file.Flow),
		Target:    &file.Target{},
	}
	s := NewTunnelModeServer(ProcessUnified, bridge, task, false)
	s.stickyStore = newMemoryStickyStore()
	return s, bridge
}

func TestProcessUnifiedHttpRequiresPassword(t *testing.T) {
	s, _ := newUnifiedE2EServer(t, "s3cret")

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("auto:wrong")) + "\r\n\r\n"
	if _, err := clientSide.Write([]byte(req)); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(common.ProxyAuthRequiredBytes))
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("read 407: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 407") {
		t.Fatalf("expected a 407 challenge, got %q", string(buf))
	}
	<-done
}

func TestProcessUnifiedHttpRejectsEmptyPasswordTask(t *testing.T) {
	s, _ := newUnifiedE2EServer(t, "")

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("auto:")) + "\r\n\r\n"
	if _, err := clientSide.Write([]byte(req)); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(common.ProxyAuthRequiredBytes))
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("read 407: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 407") {
		t.Fatalf("an empty task password must always challenge, got %q", string(buf))
	}
	<-done
}

func TestProcessUnifiedHttpConnectPicksTaggedClient(t *testing.T) {
	s, bridge := newUnifiedE2EServer(t, "s3cret")

	backendServer, backendClient := net.Pipe()
	defer func() { _ = backendClient.Close() }()
	bridge.target = backendServer

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("gz.auto:s3cret")) + "\r\n\r\n"
	if _, err := clientSide.Write([]byte(req)); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 39)
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("read 200: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 200") {
		t.Fatalf("expected 200 Connection established, got %q", string(buf))
	}

	ids := bridge.waitPicked(t, 1)
	if ids[0] != 9001 {
		t.Fatalf("gz.auto must pick client 9001, picked %v", ids)
	}
}
func TestProcessUnifiedSocks5PicksTaggedClient(t *testing.T) {
	s, bridge := newUnifiedE2EServer(t, "s3cret")

	backendServer, backendClient := net.Pipe()
	defer func() { _ = backendClient.Close() }()
	bridge.target = backendServer

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnified(conn.NewConn(serverSide), s)
	}()

	if _, err := clientSide.Write([]byte{5, 1, 2}); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read method selection: %v", err)
	}
	if reply[0] != 5 || reply[1] != 2 {
		t.Fatalf("method selection = %v, want [5 2]", reply)
	}

	auth := []byte{1, 7}
	auth = append(auth, []byte("gz.auto")...)
	auth = append(auth, 6)
	auth = append(auth, []byte("s3cret")...)
	if _, err := clientSide.Write(auth); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read auth reply: %v", err)
	}
	if reply[0] != 1 || reply[1] != 0 {
		t.Fatalf("auth reply = %v, want [1 0]", reply)
	}

	req := []byte{5, 1, 0, 3, 11}
	req = append(req, []byte("example.com")...)
	req = append(req, 0, 80)
	if _, err := clientSide.Write(req); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	resp := make([]byte, 10)
	if _, err := io.ReadFull(clientSide, resp); err != nil {
		t.Fatalf("read connect reply: %v", err)
	}
	if resp[1] != 0 {
		t.Fatalf("connect reply code = %d, want 0", resp[1])
	}

	ids := bridge.waitPicked(t, 1)
	if ids[0] != 9001 {
		t.Fatalf("gz.auto must pick client 9001, picked %v", ids)
	}
}
