package proxy

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/djylb/nps/lib/conn"
	"github.com/djylb/nps/lib/file"
)

// fakeHttpProxyServer is a minimal HTTP CONNECT upstream used to prove that
// the unified proxy really tunnels through an external HTTP proxy.
func startFakeHttpProxy(t *testing.T, user, pass string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				if req.Method != http.MethodConnect {
					_, _ = c.Write([]byte("HTTP/1.1 405 Method Not Allowed\r\n\r\n"))
					return
				}
				if user != "" || pass != "" {
					u, p, ok := parseBasic(req.Header.Get("Proxy-Authorization"))
					if !ok || u != user || p != pass {
						_, _ = c.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\n\r\n"))
						return
					}
				}
				upstream, err := net.DialTimeout("tcp", req.Host, 5*time.Second)
				if err != nil {
					_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
					return
				}
				_, _ = c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
				go func() { _, _ = io.Copy(upstream, br) }()
				_, _ = io.Copy(c, upstream)
				_ = upstream.Close()
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func parseBasic(h string) (string, string, bool) {
	const p = "Basic "
	if !strings.HasPrefix(h, p) {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h[len(p):]))
	if err != nil {
		return "", "", false
	}
	i := strings.IndexByte(string(raw), ':')
	if i < 0 {
		return "", "", false
	}
	return string(raw[:i]), string(raw[i+1:]), true
}

// startFakeSocks5Proxy is a minimal SOCKS5 upstream (RFC 1928 + RFC 1929).
func startFakeSocks5Proxy(t *testing.T, user, pass string) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				ver, err := br.ReadByte()
				if err != nil || ver != 5 {
					return
				}
				n, err := br.ReadByte()
				if err != nil {
					return
				}
				if _, err = io.ReadFull(br, make([]byte, int(n))); err != nil {
					return
				}
				_, _ = c.Write([]byte{5, 2})
				// user/pass auth
				av, err := br.ReadByte()
				if err != nil || av != 1 {
					return
				}
				ul, _ := br.ReadByte()
				ub := make([]byte, int(ul))
				_, _ = io.ReadFull(br, ub)
				pl, _ := br.ReadByte()
				pb := make([]byte, int(pl))
				_, _ = io.ReadFull(br, pb)
				if string(ub) != user || string(pb) != pass {
					_, _ = c.Write([]byte{1, 1})
					return
				}
				_, _ = c.Write([]byte{1, 0})
				// connect request
				hdr := make([]byte, 4)
				if _, err = io.ReadFull(br, hdr); err != nil {
					return
				}
				var host string
				switch hdr[3] {
				case 1:
					b := make([]byte, 4)
					_, _ = io.ReadFull(br, b)
					host = net.IP(b).String()
				case 3:
					l, _ := br.ReadByte()
					b := make([]byte, int(l))
					_, _ = io.ReadFull(br, b)
					host = string(b)
				case 4:
					b := make([]byte, 16)
					_, _ = io.ReadFull(br, b)
					host = net.IP(b).String()
				default:
					return
				}
				var pb2 [2]byte
				_, _ = io.ReadFull(br, pb2[:])
				target := net.JoinHostPort(host, itoa(int(binary.BigEndian.Uint16(pb2[:]))))
				upstream, err := net.DialTimeout("tcp", target, 5*time.Second)
				if err != nil {
					_, _ = c.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
					return
				}
				_, _ = c.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})
				go func() { _, _ = io.Copy(upstream, br) }()
				_, _ = io.Copy(c, upstream)
				_ = upstream.Close()
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close() }
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

var _ = file.ProxyNode{}

func startTinyHTTPServer(t *testing.T, status int) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				_ = req
				// A deliberate 20ms delay makes the measured latency > 0ms,
				// so tests can assert on it deterministically.
				time.Sleep(20 * time.Millisecond)
				resp := "HTTP/1.1 " + itoa(status) + " No Content\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
				_, _ = c.Write([]byte(resp))
			}(c)
		}
	}()
	return "http://" + ln.Addr().String() + "/generate_204", func() { _ = ln.Close() }
}

func splitAddr(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	port := 0
	for _, r := range portStr {
		port = port*10 + int(r-'0')
	}
	return host, port
}
func TestProbeProxyThroughFakeHttpProxy(t *testing.T) {
	targetURL, closeTarget := startTinyHTTPServer(t, 204)
	defer closeTarget()

	proxyAddr, closeProxy := startFakeHttpProxy(t, "", "")
	defer closeProxy()

	node := file.NewProxyNode()
	node.Host, node.Port = splitAddr(t, proxyAddr)
	node.Http = true

	latency, err := probeProxy(node, targetURL, protoHttp, 5*time.Second)
	if err != nil {
		t.Fatalf("probe via HTTP proxy failed: %v", err)
	}
	if latency <= 0 {
		t.Fatalf("latency must be positive, got %v", latency)
	}
}

func TestProbeProxyThroughFakeSocks5Proxy(t *testing.T) {
	targetURL, closeTarget := startTinyHTTPServer(t, 204)
	defer closeTarget()

	proxyAddr, closeProxy := startFakeSocks5Proxy(t, "u1", "p1")
	defer closeProxy()

	node := file.NewProxyNode()
	node.Host, node.Port = splitAddr(t, proxyAddr)
	node.Socks5 = true
	node.Username = "u1"
	node.Password = "p1"

	if _, err := probeProxy(node, targetURL, protoSocks5, 5*time.Second); err != nil {
		t.Fatalf("probe via SOCKS5 proxy failed: %v", err)
	}
}

func TestProbeProxyRejectsNon204(t *testing.T) {
	targetURL, closeTarget := startTinyHTTPServer(t, 200)
	defer closeTarget()

	proxyAddr, closeProxy := startFakeHttpProxy(t, "", "")
	defer closeProxy()

	node := file.NewProxyNode()
	node.Host, node.Port = splitAddr(t, proxyAddr)
	node.Http = true

	if _, err := probeProxy(node, targetURL, protoHttp, 5*time.Second); err == nil {
		t.Fatalf("status 200 must fail the 204-only probe")
	}
}
func TestUnifiedHttpConnectViaExternalProxy(t *testing.T) {
	targetURL, closeTarget := startTinyHTTPServer(t, 204)
	defer closeTarget()
	targetAddr := strings.TrimPrefix(targetURL, "http://")
	targetAddr = strings.TrimSuffix(targetAddr, "/generate_204")

	proxyAddr, closeProxy := startFakeHttpProxy(t, "", "")
	defer closeProxy()

	db := file.GetDb()
	node := file.NewProxyNode()
	node.Id = 9101
	node.Host, node.Port = splitAddr(t, proxyAddr)
	node.Http = true
	node.Enabled = true
	node.MarkCheckSuccess(5*time.Millisecond, time.Now(), 1)
	db.JsonDb.Proxies.Store(node.Id, node)
	t.Cleanup(func() { db.JsonDb.Proxies.Delete(node.Id) })

	bridge := &fakeBridge{}
	task := &file.Tunnel{
		Id:        4243,
		Mode:      "unifiedProxy",
		Password:  "s3cret",
		CacheTime: 10,
		Flow:      new(file.Flow),
		Target:    &file.Target{},
	}
	s := NewTunnelModeServer(ProcessUnifiedHttp, bridge, task, false)
	s.stickyStore = newMemoryStickyStore()

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT " + targetAddr + " HTTP/1.1\r\nHost: " + targetAddr + "\r\n" +
		"Proxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("auto:s3cret")) + "\r\n\r\n"
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

	// The tunnel is live: a plain HTTP request must reach the target through
	// the external proxy.
	_, _ = clientSide.Write([]byte("GET /generate_204 HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
	resp := make([]byte, 64)
	n, _ := clientSide.Read(resp)
	if !strings.Contains(string(resp[:n]), "204") {
		t.Fatalf("tunnel through external proxy returned %q", string(resp[:n]))
	}

	// The external proxy is not an NPS client: the bridge must never be called.
	if ids := bridge.picked(); len(ids) != 0 {
		t.Fatalf("external proxy egress must not touch the bridge, picked %v", ids)
	}
}
func TestUnifiedSocks5ViaExternalProxy(t *testing.T) {
	targetURL, closeTarget := startTinyHTTPServer(t, 204)
	defer closeTarget()
	rest := strings.TrimPrefix(targetURL, "http://")
	rest = strings.TrimSuffix(rest, "/generate_204")
	targetHost, targetPort := splitAddr(t, rest)

	proxyAddr, closeProxy := startFakeSocks5Proxy(t, "u1", "p1")
	defer closeProxy()

	db := file.GetDb()
	node := file.NewProxyNode()
	node.Id = 9102
	node.Host, node.Port = splitAddr(t, proxyAddr)
	node.Socks5 = true
	node.Username = "u1"
	node.Password = "p1"
	node.Enabled = true
	node.MarkCheckSuccess(5*time.Millisecond, time.Now(), 1)
	db.JsonDb.Proxies.Store(node.Id, node)
	t.Cleanup(func() { db.JsonDb.Proxies.Delete(node.Id) })

	bridge := &fakeBridge{}
	task := &file.Tunnel{
		Id:        4244,
		Mode:      "unifiedProxy",
		Password:  "s3cret",
		CacheTime: 10,
		Flow:      new(file.Flow),
		Target:    &file.Target{},
	}
	s := NewTunnelModeServer(ProcessUnified, bridge, task, false)
	s.stickyStore = newMemoryStickyStore()

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnified(conn.NewConn(serverSide), s)
	}()

	// methods handshake
	if _, err := clientSide.Write([]byte{5, 1, 2}); err != nil {
		t.Fatalf("write methods: %v", err)
	}
	methodReply := make([]byte, 2)
	if _, err := io.ReadFull(clientSide, methodReply); err != nil {
		t.Fatalf("read method: %v", err)
	}
	if methodReply[0] != 5 || methodReply[1] != 2 {
		t.Fatalf("method reply = %v, want [5 2]", methodReply)
	}

	// RFC1929 user/pass
	auth := []byte{1, 4}
	auth = append(auth, []byte("auto")...)
	auth = append(auth, 6)
	auth = append(auth, []byte("s3cret")...)
	if _, err := clientSide.Write(auth); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	authReply := make([]byte, 2)
	if _, err := io.ReadFull(clientSide, authReply); err != nil {
		t.Fatalf("read auth: %v", err)
	}
	if authReply[1] != 0 {
		t.Fatalf("auth rejected: %v", authReply)
	}

	// CONNECT request to the tiny target
	connReq := []byte{5, 1, 0, 3, byte(len(targetHost))}
	connReq = append(connReq, []byte(targetHost)...)
	connReq = append(connReq, byte(targetPort>>8), byte(targetPort&0xff))
	if _, err := clientSide.Write(connReq); err != nil {
		t.Fatalf("write connect: %v", err)
	}
	reply := make([]byte, 10)
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read connect reply: %v", err)
	}
	if reply[1] != 0 {
		t.Fatalf("connect reply code = %d, want 0", reply[1])
	}

	// data flows through the external SOCKS5 proxy to the tiny target
	if _, err := clientSide.Write([]byte("GET /generate_204 HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write get: %v", err)
	}
	resp := make([]byte, 64)
	n, _ := clientSide.Read(resp)
	if !strings.Contains(string(resp[:n]), "204") {
		t.Fatalf("tunnel via SOCKS5 proxy returned %q", string(resp[:n]))
	}
	if ids := bridge.picked(); len(ids) != 0 {
		t.Fatalf("external SOCKS5 egress must not touch the bridge, picked %v", ids)
	}
}
