package proxy

import (
	"net"
	"testing"
	"time"

	"example.com/svcmgr/lib/file"
)

// captureConn records everything written to it and never returns data, so a
// dial test can prove the client handshake was produced without standing up a
// matching server for every protocol.
type captureConn struct {
	net.Conn
	buf []byte
}

func (c *captureConn) Write(b []byte) (int, error) {
	c.buf = append(c.buf, b...)
	return len(b), nil
}

func (c *captureConn) Read(b []byte) (int, error) { return 0, net.ErrClosed }

// listenCapture accepts one connection and wraps it so writes are recorded.
func listenCapture(t *testing.T) (addr string, got chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got = make(chan []byte, 1)
	go func() {
		defer ln.Close()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		cap := &captureConn{Conn: c}
		// Drain whatever the client writes.
		buf := make([]byte, 4096)
		for {
			n, err := c.Read(buf)
			cap.buf = append(cap.buf, buf[:n]...)
			if err != nil {
				break
			}
		}
		got <- cap.buf
	}()
	return ln.Addr().String(), got
}

func TestDialShadowsocksWritesHandshake(t *testing.T) {
	addr, got := listenCapture(t)
	host, port := splitListenAddr(t, addr)
	node := &file.ProxyNode{Scheme: file.SchemeSS, Host: host, Port: port, Method: "aes-256-gcm", Password: "pass"}
	conn, err := dialShadowsocks(node, "example.com:80", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("payload"))
	_ = conn.Close()
	data := <-got
	if len(data) < 32 {
		t.Fatalf("handshake too short: %d bytes", len(data))
	}
}

func TestDialShadowsocksStreamWritesHandshake(t *testing.T) {
	addr, got := listenCapture(t)
	host, port := splitListenAddr(t, addr)
	node := &file.ProxyNode{Scheme: file.SchemeSSR, Host: host, Port: port, Method: "aes-256-cfb", Password: "pass"}
	conn, err := dialShadowsocks(node, "example.com:80", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("payload"))
	_ = conn.Close()
	data := <-got
	if len(data) == 0 {
		t.Fatal("no data written")
	}
}

func TestDialVMessWritesHandshake(t *testing.T) {
	addr, got := listenCapture(t)
	host, port := splitListenAddr(t, addr)
	node := &file.ProxyNode{Scheme: file.SchemeVMess, Host: host, Port: port,
		Password: "b831381d-6324-4d53-ad4f-8cda48b30811", Method: "aes-128-gcm"}
	conn, err := dialVMess(node, "example.com:80", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("payload"))
	_ = conn.Close()
	data := <-got
	if len(data) < 16 {
		t.Fatalf("vmess handshake too short: %d", len(data))
	}
}

func TestDialVMessRejectsBadID(t *testing.T) {
	node := &file.ProxyNode{Scheme: file.SchemeVMess, Host: "127.0.0.1", Port: 1, Password: "", Method: "auto"}
	if _, err := dialVMess(node, "example.com:80", time.Second); err == nil {
		t.Fatal("expected error for empty user id")
	}
}

func TestDialTrojanWritesRequest(t *testing.T) {
	addr, got := listenCapture(t)
	host, port := splitListenAddr(t, addr)
	node := &file.ProxyNode{Scheme: file.SchemeTrojan, Host: host, Port: port, Password: "secret", TLS: false}
	conn, err := dialTrojan(node, "example.com:80", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	data := <-got
	if len(data) < 60 || data[58] != 0x01 {
		t.Fatalf("trojan request malformed: %d bytes", len(data))
	}
}

func TestUnsupportedScheme(t *testing.T) {
	node := &file.ProxyNode{Scheme: file.SchemeTUIC, Host: "127.0.0.1", Port: 1, Id: 7}
	if _, err := dialTunnelNode(node, "example.com:80", time.Second); err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}

func splitListenAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := parsePort(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}
