package proxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/sagernet/sing-shadowsocks"
	"github.com/sagernet/sing-shadowsocks/shadowaead"
	"github.com/sagernet/sing-shadowsocks/shadowstream"
	vmess "github.com/sagernet/sing-vmess"
	"github.com/sagernet/sing-vmess/vless"
	M "github.com/sagernet/sing/common/metadata"

	"example.com/svcmgr/lib/file"
)

// dialTunnelNode dials target through a tunnel-protocol node. Every supported
// scheme is reduced to one net.Conn, so the caller (unified proxy relay and
// health probe) never has to know which protocol is underneath.
func dialTunnelNode(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	scheme := node.SchemeName()
	switch scheme {
	case file.SchemeSS, file.SchemeSSR:
		return dialShadowsocks(node, target, timeout)
	case file.SchemeVMess:
		return dialVMess(node, target, timeout)
	case file.SchemeVLESS:
		return dialVLESS(node, target, timeout)
	case file.SchemeTrojan:
		return dialTrojan(node, target, timeout)
	default:
		return nil, fmt.Errorf("proxy node %d: scheme %q is not dialable yet", node.Id, scheme)
	}
}

func dialTCP(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	return conn, nil
}

// dialShadowsocks covers SS (AEAD) and SSR (stream ciphers). The cipher
// method decides which implementation is used.
func dialShadowsocks(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	method, err := shadowsocksMethod(node.MethodName(), node.PasswordValue())
	if err != nil {
		return nil, err
	}
	conn, err := dialTCP(node.Addr(), timeout)
	if err != nil {
		return nil, err
	}
	// shadowstream.DialConn panics when the destination address is written
	// into a zero-length buffer (a bug in sing-shadowsocks v0.2.7), so the
	// stream ciphers use DialEarlyConn and flush the request header with the
	// first real write instead.
	if isStreamCipher(node.MethodName()) {
		return method.DialEarlyConn(conn, M.ParseSocksaddr(target)), nil
	}
	out, err := method.DialConn(conn, M.ParseSocksaddr(target))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("shadowsocks handshake: %w", err)
	}
	return out, nil
}

func isStreamCipher(method string) bool {
	switch method {
	case "aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
		"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
		"rc4-md5", "chacha20-ietf", "xchacha20":
		return true
	default:
		return false
	}
}

func shadowsocksMethod(method, password string) (shadowsocks.Method, error) {
	if method == "" {
		method = "aes-256-gcm"
	}
	if isAEAD(method) {
		if m, err := shadowaead.New(method, nil, password); err == nil {
			return m, nil
		}
	}
	if m, err := shadowstream.New(method, nil, password); err == nil {
		return m, nil
	}
	return nil, fmt.Errorf("unsupported shadowsocks method %q", method)
}

func isAEAD(method string) bool {
	switch method {
	case "aes-128-gcm", "aes-192-gcm", "aes-256-gcm", "chacha20-ietf-poly1305", "xchacha20-ietf-poly1305":
		return true
	default:
		return false
	}
}

// dialVMess dials through a VMess node. Transport is TCP or TLS; security
// defaults to auto when unset.
func dialVMess(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	if node.PasswordValue() == "" {
		return nil, errors.New("vmess: missing user id")
	}
	security := node.MethodName()
	if security == "" {
		security = "auto"
	}
	client, err := vmess.NewClient(node.PasswordValue(), security, node.AlterIdValue())
	if err != nil {
		return nil, fmt.Errorf("vmess client: %w", err)
	}
	conn, err := dialTransport(node, timeout)
	if err != nil {
		return nil, err
	}
	out, err := client.DialConn(conn, M.ParseSocksaddr(target))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("vmess handshake: %w", err)
	}
	return out, nil
}

// dialVLESS dials through a VLESS node. The user id lives in Password.
func dialVLESS(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	if node.PasswordValue() == "" {
		return nil, errors.New("vless: missing user id")
	}
	client, err := vless.NewClient(node.PasswordValue(), node.FlowName(), nil)
	if err != nil {
		return nil, fmt.Errorf("vless client: %w", err)
	}
	conn, err := dialTransport(node, timeout)
	if err != nil {
		return nil, err
	}
	out, err := client.DialConn(conn, M.ParseSocksaddr(target))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("vless handshake: %w", err)
	}
	return out, nil
}

// dialTrojan dials through a Trojan node: TLS, then the password hash and the
// target address, exactly as the Trojan protocol specifies.
func dialTrojan(node *file.ProxyNode, target string, timeout time.Duration) (net.Conn, error) {
	if node.PasswordValue() == "" {
		return nil, errors.New("trojan: missing password")
	}
	conn, err := dialTransport(node, timeout)
	if err != nil {
		return nil, err
	}
	req, err := trojanRequest(node.PasswordValue(), target)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if _, err = conn.Write(req); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("trojan handshake: %w", err)
	}
	return conn, nil
}

// dialTransport dials the node and wraps the connection in TLS when the node
// asks for it. Server name defaults to the node host.
func dialTransport(node *file.ProxyNode, timeout time.Duration) (net.Conn, error) {
	conn, err := dialTCP(node.Addr(), timeout)
	if err != nil {
		return nil, err
	}
	if !node.TLSEnabled() {
		return conn, nil
	}
	name := node.SNIName()
	if name == "" {
		name = node.HostValue()
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         name,
		InsecureSkipVerify: node.SkipVerifyEnabled(),
		MinVersion:         tls.VersionTLS12,
	})
	if err = tlsConn.Handshake(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("tls handshake: %w", err)
	}
	return tlsConn, nil
}
