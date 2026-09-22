package proxy

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"example.com/svcmgr/lib/conn"
	"example.com/svcmgr/lib/file"
	"example.com/svcmgr/lib/logs"
)

// dialEgressTimeout bounds how long the upstream proxy handshake may take.
const dialEgressTimeout = 15 * time.Second

// relayEgress pipes the client connection and the upstream tunnel and accounts
// the traffic on the unified proxy task flow.
func (s *TunnelModeServer) relayEgress(c *conn.Conn, upstream net.Conn, rb []byte) {
	conn.CopyWaitGroup(upstream, c.Conn, false, false, nil, []*file.Flow{s.Task.Flow}, true, 0, rb, s.Task, false, false)
}

// handleSocks5ViaProxy serves a SOCKS5 CONNECT entirely through an external
// SOCKS5 upstream proxy. BIND / UDP ASSOCIATE are not supported on this path.
func (s *TunnelModeServer) handleSocks5ViaProxy(c net.Conn, node *file.ProxyNode) {
	addr, err := readSocks5Request(c)
	if err != nil {
		logs.Warn("unified proxy: socks5 request via proxy %d failed: %v", node.Id, err)
		sendSocks5Reply(c, repGeneralFailure)
		_ = c.Close()
		return
	}
	upstream, err := dialViaProxyNode(node, protoSocks5, addr, dialEgressTimeout)
	if err != nil {
		logs.Warn("unified proxy: socks5 dial via %s to %s failed: %v", node.String(), addr, err)
		sendSocks5Reply(c, repGeneralFailure)
		_ = c.Close()
		return
	}
	sendSocks5Reply(c, repSucceeded)
	s.relayEgress(conn.NewConn(c), upstream, nil)
}

// readSocks5Request parses the SOCKS5 request (RFC 1928) that follows the
// method negotiation and returns "host:port". Only CONNECT is accepted.
func readSocks5Request(c net.Conn) (string, error) {
	var header [3]byte
	if _, err := io.ReadFull(c, header[:]); err != nil {
		return "", err
	}
	if header[0] != 5 || header[2] != 0 {
		return "", errors.New("invalid socks5 request header")
	}
	if header[1] != 1 {
		return "", errors.New("only the CONNECT command is supported through an external proxy")
	}
	var addrType [1]byte
	if _, err := io.ReadFull(c, addrType[:]); err != nil {
		return "", err
	}
	var host string
	switch addrType[0] {
	case 1:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(c, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case 4:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(c, buf); err != nil {
			return "", err
		}
		host = net.IP(buf).String()
	case 3:
		var l [1]byte
		if _, err := io.ReadFull(c, l[:]); err != nil {
			return "", err
		}
		buf := make([]byte, int(l[0]))
		if _, err := io.ReadFull(c, buf); err != nil {
			return "", err
		}
		host = string(buf)
	default:
		return "", errors.New("unsupported address type")
	}
	var portBuf [2]byte
	if _, err := io.ReadFull(c, portBuf[:]); err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBuf[:])))), nil
}

// SOCKS5 reply codes used on the external proxy path.
const (
	repSucceeded      = 0
	repGeneralFailure = 1
)

// sendSocks5Reply writes a minimal SOCKS5 reply with a 0.0.0.0:0 bound
// address, which x/net/proxy and every mainstream client accepts.
func sendSocks5Reply(c net.Conn, rep byte) {
	_, _ = c.Write([]byte{5, rep, 0, 1, 0, 0, 0, 0, 0, 0})
}

// startProxyHealthLoopIfConfigured launches the background health scheduler
// exactly once per process.
var startHealthLoopOnce sync.Once

func startProxyHealthLoopIfConfigured() {
	startHealthLoopOnce.Do(func() {
		StartProxyHealthLoop()
	})
}

// handleHttpViaProxy serves an HTTP proxy request through an external upstream
// proxy. CONNECT tunnels the raw stream; plain requests are forwarded by
// opening a tunnel to the target and replaying the (buffered) request bytes.
func (s *TunnelModeServer) handleHttpViaProxy(c *conn.Conn, node *file.ProxyNode, addr string, rb []byte, r *http.Request) error {
	upstream, err := dialViaProxyNode(node, protoHttp, addr, dialEgressTimeout)
	if err != nil {
		logs.Warn("unified proxy: http dial via %s to %s failed: %v", node.String(), addr, err)
		_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
		_ = c.Close()
		return err
	}
	if r.Method == http.MethodConnect {
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
		s.relayEgress(c, upstream, rb)
		return nil
	}
	// Plain HTTP: the request line/headers were pre-read into rb; send them
	// through the tunnel, then keep relaying (keep-alive aware).
	if _, err := upstream.Write(rb); err != nil {
		_ = upstream.Close()
		_ = c.Close()
		return err
	}
	s.relayEgress(c, upstream, nil)
	return nil
}
