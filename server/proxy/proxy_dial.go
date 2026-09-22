package proxy

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"example.com/svcmgr/lib/file"
)

// dialViaProxyNode opens a tunnel to target ("host:port") through the given
// external proxy node. The protocol argument selects which upstream protocol
// must be used for plain HTTP/SOCKS5 nodes; nodes that carry a tunnel scheme
// (ss, vmess, trojan, ...) ignore it and dial with their own protocol.
func dialViaProxyNode(node *file.ProxyNode, protocol egressProtocol, target string, timeout time.Duration) (net.Conn, error) {
	if node == nil {
		return nil, errors.New("proxy node is nil")
	}
	if node.IsTunnel() {
		return dialTunnelNode(node, target, timeout)
	}
	switch protocol {
	case protoHttp:
		if !node.SupportsHttp() {
			return nil, fmt.Errorf("proxy node %d does not support HTTP", node.Id)
		}
		return dialHttpProxy(node, target, timeout)
	case protoSocks5:
		if !node.SupportsSocks5() {
			return nil, fmt.Errorf("proxy node %d does not support SOCKS5", node.Id)
		}
		return dialSocks5Proxy(node, target, timeout)
	default:
		return nil, errors.New("unknown proxy protocol")
	}
}

// dialSocks5Proxy dials target through an upstream SOCKS5 proxy using the
// golang.org/x/net/proxy client (RFC 1928 + RFC 1929 username/password auth).
func dialSocks5Proxy(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	var auth *proxy.Auth
	if user, password := node.Credentials(); user != "" || password != "" {
		auth = &proxy.Auth{User: user, Password: password}
	}
	base := &net.Dialer{Timeout: timeout}
	dialer, err := proxy.SOCKS5("tcp", node.Addr(), auth, base)
	if err != nil {
		return nil, err
	}
	type deadliner interface{ SetDeadline(time.Time) error }
	conn, err := dialer.Dial("tcp", target)
	if err != nil {
		return nil, err
	}
	if d, ok := conn.(deadliner); ok {
		_ = d.SetDeadline(time.Now().Add(timeout))
	}
	return conn, nil
}

// dialHttpProxy dials target through an upstream HTTP proxy using the CONNECT
// method. Any bytes the proxy sent after the response headers are preserved
// via bufferedConn so no early data is lost.
func dialHttpProxy(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", node.Addr(), timeout)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	var req strings.Builder
	req.WriteString("CONNECT ")
	req.WriteString(target)
	req.WriteString(" HTTP/1.1\r\nHost: ")
	req.WriteString(target)
	req.WriteString("\r\n")
	if user, password := node.Credentials(); user != "" || password != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
		req.WriteString("Proxy-Authorization: Basic ")
		req.WriteString(cred)
		req.WriteString("\r\n")
	}
	req.WriteString("Proxy-Connection: Keep-Alive\r\n\r\n")

	if _, err = io.WriteString(conn, req.String()); err != nil {
		_ = conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy connect response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		_ = conn.Close()
		return nil, fmt.Errorf("proxy connect rejected with status %d", resp.StatusCode)
	}

	_ = conn.SetDeadline(time.Time{})
	if br.Buffered() > 0 {
		return &bufferedConn{Conn: conn, r: br}, nil
	}
	return conn, nil
}

// bufferedConn makes sure bytes that were already buffered while parsing the
// CONNECT response are returned before reading from the socket again.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(b []byte) (int, error) {
	return c.r.Read(b)
}

// probeProxy performs a full end-to-end health check through the proxy node:
//
//	connect proxy -> protocol handshake/auth -> CONNECT/TLS/HTTP -> 204
//
// It returns the total elapsed time and nil only when the probe answered
// HTTP 204. Any other status, dial failure or TLS failure is reported as error.
func probeProxy(node *file.ProxyNode, checkURL string, protocol egressProtocol, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	u, err := url.Parse(checkURL)
	if err != nil {
		return 0, fmt.Errorf("invalid check url: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return 0, errors.New("invalid check url: missing host")
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	target := net.JoinHostPort(host, port)

	conn, err := dialViaProxyNode(node, protocol, target, timeout)
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	var rw io.ReadWriter = conn
	if u.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err = tlsConn.Handshake(); err != nil {
			return 0, fmt.Errorf("tls handshake: %w", err)
		}
		rw = tlsConn
	}

	path := u.Path
	if path == "" {
		path = "/generate_204"
	}
	req := "GET " + path + " HTTP/1.1\r\nHost: " + host + "\r\nUser-Agent: nps-health-check\r\nConnection: close\r\n\r\n"
	if _, err = io.WriteString(rw, req); err != nil {
		return 0, err
	}

	br := bufio.NewReader(rw)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		return 0, fmt.Errorf("unexpected status %d (want 204)", resp.StatusCode)
	}
	_ = resp.Body.Close()

	return time.Since(start), nil
}
