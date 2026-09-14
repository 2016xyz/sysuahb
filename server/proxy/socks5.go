package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/conn"
	"github.com/djylb/nps/lib/file"
	"github.com/djylb/nps/lib/logs"
	"github.com/djylb/nps/lib/transport"
)

func readLenPrefixedString(r io.Reader) (string, error) {
	var n [1]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return "", err
	}
	if n[0] == 0 {
		return "", errors.New("empty field")
	}
	b := make([]byte, int(n[0]))
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return string(b), nil
}

const (
	ipV4            = 1
	domainName      = 3
	ipV6            = 4
	connectMethod   = 1
	bindMethod      = 2
	associateMethod = 3
	// The maximum packet size of any udp Associate packet, based on ethernet's max size,
	// minus the IP and UDP headers. IPv4 has a 20 byte header, UDP adds another 4 bytes.
	// This is a total overhead of 24 bytes. Ethernet's max packet size is 1500 bytes,
	// 1500 - 24 = 1476.
	//maxUDPPacketSize = 1476
)

const (
	succeeded uint8 = iota
	serverFailure
	notAllowed
	networkUnreachable
	hostUnreachable
	connectionRefused
	ttlExpired
	commandNotSupported
	addrTypeNotSupported
)

const (
	UserPassAuth    = uint8(2)
	userAuthVersion = uint8(1)
	authSuccess     = uint8(0)
	authFailure     = uint8(1)
)

// Handle the SOCKS5 request after method selection.
// Expected header:
// +----+-----+-------+------+----------+----------+
// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
// +----+-----+-------+------+----------+----------+
func (s *TunnelModeServer) handleSocks5Request(c net.Conn, client *file.Client) {
	var header [3]byte
	if _, err := io.ReadFull(c, header[:]); err != nil {
		logs.Warn("illegal request (head) %v", err)
		_ = c.Close()
		return
	}
	// Strict check: VER==5, RSV==0
	if header[0] != 5 || header[2] != 0 {
		logs.Warn("illegal request ver/rsv: ver=%d rsv=%d", header[0], header[2])
		s.sendReply(c, commandNotSupported)
		_ = c.Close()
		return
	}

	switch header[1] {
	case connectMethod:
		s.handleConnect(c, client)
	case bindMethod:
		s.handleBind(c)
	case associateMethod:
		if client == nil {
			// unified proxy: UDP ASSOCIATE is not supported yet
			s.sendReply(c, commandNotSupported)
			_ = c.Close()
			return
		}
		s.handleUDP(c)
	default:
		s.sendReply(c, commandNotSupported)
		_ = c.Close()
	}
}

// Send a standard SOCKS5 reply using c.LocalAddr as BND.ADDR/BND.PORT.
func (s *TunnelModeServer) sendReply(c net.Conn, rep uint8) {
	localAddr := c.LocalAddr().String()
	localHost, localPort, _ := net.SplitHostPort(localAddr)
	ip := net.ParseIP(localHost)

	var atype byte
	var addrBytes []byte
	if v4 := ip.To4(); v4 != nil {
		atype = ipV4
		addrBytes = v4
	} else if v6 := ip.To16(); v6 != nil {
		atype = ipV6
		addrBytes = v6
	} else {
		atype = ipV4
		addrBytes = net.IPv4(127, 0, 0, 1).To4()
	}

	nPort, _ := strconv.Atoi(localPort)

	// VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
	reply := make([]byte, 6+len(addrBytes))
	reply[0], reply[1], reply[2], reply[3] = 5, rep, 0, atype
	copy(reply[4:], addrBytes)
	binary.BigEndian.PutUint16(reply[4+len(addrBytes):], uint16(nPort))
	_, _ = c.Write(reply)
}

// CONNECT command handler: parse target and bridge TCP through DealClient.
func (s *TunnelModeServer) handleConnect(c net.Conn, client *file.Client) {
	var addrType [1]byte
	if _, err := io.ReadFull(c, addrType[:]); err != nil {
		s.sendReply(c, addrTypeNotSupported)
		return
	}
	var host string
	switch addrType[0] {
	case ipV4:
		ipv4 := make(net.IP, net.IPv4len)
		if _, err := io.ReadFull(c, ipv4); err != nil {
			s.sendReply(c, addrTypeNotSupported)
			return
		}
		host = ipv4.String()
	case ipV6:
		ipv6 := make(net.IP, net.IPv6len)
		if _, err := io.ReadFull(c, ipv6); err != nil {
			s.sendReply(c, addrTypeNotSupported)
			return
		}
		host = ipv6.String()
	case domainName:
		domain, err := readLenPrefixedString(c)
		if err != nil {
			s.sendReply(c, addrTypeNotSupported)
			return
		}
		host = domain
	default:
		s.sendReply(c, addrTypeNotSupported)
		return
	}

	var port uint16
	if err := binary.Read(c, binary.BigEndian, &port); err != nil {
		s.sendReply(c, addrTypeNotSupported)
		return
	}

	addr := net.JoinHostPort(host, strconv.Itoa(int(port)))
	if s.Task != nil && s.Task.Mode == "mixProxy" && s.Task.DestAclMode != file.AclOff {
		if !s.Task.AllowsDestination(addr) {
			logs.Warn("mixProxy dest acl deny: client=%d task=%d dest=%s", s.Task.Client.Id, s.Task.Id, common.ExtractHost(addr))
			s.sendReply(c, notAllowed)
			_ = c.Close()
			return
		}
	}

	_ = s.DealClient(conn.NewConn(c), client, addr, nil, common.CONN_TCP, func() {
		s.sendReply(c, succeeded)
	}, []*file.Flow{s.Task.Flow, client.Flow}, 0, s.Task.Target.LocalProxy, s.Task)
}

// BIND is not supported.
func (s *TunnelModeServer) handleBind(c net.Conn) {
	s.sendReply(c, commandNotSupported)
	_ = c.Close()
}

// Compose UDP associate reply. Prefer the actual UDP bound IP, then TCP local IP,
func (s *TunnelModeServer) sendUdpReply(writeConn net.Conn, replyUDP *net.UDPConn, rep uint8, clientIP net.IP) {
	wantV6 := clientIP != nil && clientIP.To4() == nil

	var ipToUse net.IP
	var tcpLocalIP net.IP
	var udpLocalIP net.IP

	if ta, ok := writeConn.LocalAddr().(*net.TCPAddr); ok && ta != nil && ta.IP != nil {
		tcpLocalIP = ta.IP
	}
	if ua, ok := replyUDP.LocalAddr().(*net.UDPAddr); ok && ua != nil && ua.IP != nil {
		udpLocalIP = ua.IP
	}

	// 1) Prefer UDP bound IP if it's specific and matches family.
	if udpLocalIP != nil && !udpLocalIP.IsUnspecified() && !common.IsZeroIP(udpLocalIP) {
		if (udpLocalIP.To4() == nil) == wantV6 {
			ipToUse = udpLocalIP
		}
	}
	// 2) Fallback to TCP local IP if family matches.
	if ipToUse == nil && tcpLocalIP != nil && !tcpLocalIP.IsUnspecified() && !common.IsZeroIP(tcpLocalIP) {
		if (tcpLocalIP.To4() == nil) == wantV6 {
			ipToUse = tcpLocalIP
		}
	}
	// 3) Final fallback to loopback
	if ipToUse == nil {
		if wantV6 {
			ipToUse = net.IPv6loopback
		} else {
			ipToUse = net.IPv4(127, 0, 0, 1)
		}
	}

	var atype byte
	var addrBytes []byte
	if v4 := ipToUse.To4(); v4 != nil {
		atype = ipV4
		addrBytes = v4
	} else {
		atype = ipV6
		addrBytes = ipToUse.To16()
	}

	la := replyUDP.LocalAddr().(*net.UDPAddr)

	logs.Debug("send udp reply: chosen=%v atype=%d bndPort=%d wantV6=%v tcpLocal=%v udpLocal=%v clientIP=%v",
		ipToUse, atype, la.Port, wantV6, writeConn.LocalAddr(), replyUDP.LocalAddr(), clientIP)

	// VER, REP, RSV, ATYP, BND.ADDR, BND.PORT
	reply := make([]byte, 6+len(addrBytes))
	reply[0], reply[1], reply[2], reply[3] = 5, rep, 0, atype
	copy(reply[4:], addrBytes)
	binary.BigEndian.PutUint16(reply[4+len(addrBytes):], uint16(la.Port))
	_, _ = writeConn.Write(reply)
}

func (s *TunnelModeServer) handleUDP(c net.Conn) {
	if tcpConn, ok := c.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(15 * time.Second)
		_ = transport.SetTcpKeepAliveParams(tcpConn, 15, 15, 3)
	}
	if s.Task != nil && s.Task.Mode == "mixProxy" && s.Task.DestAclMode != file.AclOff {
		// SOCKS5 UDP frames are tunneled as-is and we do not reliably inspect per-datagram destinations.
		// Reject UDP associate whenever destination ACL is enabled to avoid bypassing whitelist/blacklist.
		logs.Warn("mixProxy dest acl active, reject socks5 udp associate: client=%d task=%d", s.Task.Client.Id, s.Task.Id)
		s.sendReply(c, notAllowed)
		_ = c.Close()
		return
	}
	defer func() { _ = c.Close() }()
	var addrType [1]byte
	if _, err := io.ReadFull(c, addrType[:]); err != nil {
		s.sendReply(c, addrTypeNotSupported)
		return
	}
	var host string
	switch addrType[0] {
	case ipV4:
		ipv4 := make(net.IP, net.IPv4len)
		if _, err := io.ReadFull(c, ipv4); err != nil {
			s.sendReply(c, addrTypeNotSupported)
			return
		}
		host = ipv4.String()
	case ipV6:
		ipv6 := make(net.IP, net.IPv6len)
		if _, err := io.ReadFull(c, ipv6); err != nil {
			s.sendReply(c, addrTypeNotSupported)
			return
		}
		host = ipv6.String()
	case domainName:
		domain, err := readLenPrefixedString(c)
		if err != nil {
			s.sendReply(c, addrTypeNotSupported)
			return
		}
		host = domain
	default:
		s.sendReply(c, addrTypeNotSupported)
		return
	}
	var port uint16
	if err := binary.Read(c, binary.BigEndian, &port); err != nil {
		s.sendReply(c, addrTypeNotSupported)
		return
	}
	logs.Trace("ASSOCIATE %s:%d", host, port)

	// Bind a UDP socket. Try to match client's address family where possible.
	var clientIP net.IP
	if ta, ok := c.RemoteAddr().(*net.TCPAddr); ok && ta != nil {
		clientIP = ta.IP
	}
	network, localAddr := common.BuildUdpBindAddr(s.Task.ServerIp, clientIP)
	logs.Trace("listen local reply udp port (network=%s, localAddr=%v)", network, localAddr)
	reply, err := net.ListenUDP(network, localAddr)
	if err != nil {
		s.sendReply(c, addrTypeNotSupported)
		logs.Error("listen local reply udp port error: %v (network=%s, localAddr=%v)", err, network, localAddr)
		return
	}
	defer func() { _ = reply.Close() }()
	// Reply BND.ADDR/PORT to client.
	s.sendUdpReply(c, reply, succeeded, clientIP)

	// Create a tunnel link to npc; pass SOCKS5 UDP frames as-is.
	link := conn.NewLink("udp5", "", s.Task.Client.Cnf.Crypt, s.Task.Client.Cnf.Compress, c.RemoteAddr().String(), s.AllowLocalProxy && s.Task.Target.LocalProxy)
	link.Option.Timeout = 180 * time.Second

	target, err := s.Bridge.SendLinkInfo(s.Task.Client.Id, link, s.Task)
	if err != nil {
		logs.Warn("get connection from client Id %d error: %v", s.Task.Client.Id, err)
		return
	}
	defer func() { _ = target.Close() }()
	timeoutConn := conn.NewTimeoutConn(target, link.Option.Timeout)
	defer func() { _ = timeoutConn.Close() }()
	flowConn := conn.NewFlowConn(timeoutConn, s.Task.Flow, s.Task.Client.Flow)
	framed := conn.WrapFramed(flowConn)

	// First-UDP IP locking: set on first datagram we receive, then only accept same IP.
	var firstUDPIP net.IP
	var clientAddr atomic.Pointer[net.UDPAddr]

	// Local UDP -> tunnel
	go func() {
		b := common.BufPool.Get()
		defer common.BufPool.Put(b)

		for {
			n, lAddr, err := reply.ReadFromUDP(b)
			if err != nil {
				logs.Debug("read data from %v err %v", reply.LocalAddr(), err)
				_ = c.Close()
				_ = flowConn.Close()
				return
			}

			// Lock to the first UDP source IP (allow port changes).
			normIP := common.NormalizeIP(lAddr.IP)
			if firstUDPIP == nil {
				firstUDPIP = normIP
				logs.Debug("lock UDP source ip to %v", firstUDPIP)
			}
			if !normIP.Equal(firstUDPIP) {
				logs.Debug("ignore udp from unexpected ip: %v (locked to %v)", lAddr.IP, firstUDPIP)
				continue
			}
			// Update the return address to the latest (port may change).
			clientAddr.Store(lAddr)

			// SOCKS5 UDP: FRAG must be 0 (we don't support fragmentation).
			if n >= 3 && b[2] != 0 {
				logs.Warn("socks5 udp frag not supported, drop (frag=%d)", b[2])
				continue
			}
			// Size guard vs framed link.
			if n > conn.MaxFramePayload {
				logs.Debug("udp datagram too large: %d > %d (drop)", n, conn.MaxFramePayload)
				continue
			}
			if _, err := framed.Write(b[:n]); err != nil {
				logs.Debug("write udp frame to tunnel error %v", err)
				_ = c.Close()
				_ = flowConn.Close()
				return
			}
		}
	}()

	// Tunnel -> local UDP
	go func() {
		b := common.BufPool.Get()
		defer common.BufPool.Put(b)

		for {
			n, err := framed.Read(b)
			if err != nil || n <= 0 || n > len(b) {
				logs.Debug("read udp frame from tunnel error %v", err)
				_ = c.Close()
				_ = flowConn.Close()
				return
			}
			if addr := clientAddr.Load(); addr != nil {
				if _, err := reply.WriteTo(b[:n], addr); err != nil {
					logs.Warn("write data to user %v", err)
					_ = c.Close()
					_ = flowConn.Close()
					return
				}
			} else {
				// Haven't seen a valid client UDP address yet; drop.
				logs.Debug("no client udp addr yet, drop %d bytes", n)
			}
		}
	}()

	// Keep TCP control connection alive until client closes it.
	b := common.BufPool.Get()
	defer common.BufPool.Put(b)
	for {
		if _, err := c.Read(b); err != nil {
			_ = flowConn.Close()
			return
		}
	}
}

func (s *TunnelModeServer) SocksAuth(c net.Conn) error {
	var header [2]byte
	if _, err := io.ReadFull(c, header[:]); err != nil {
		return err
	}
	if header[0] != userAuthVersion {
		return errors.New("auth method not supported")
	}
	userLen := int(header[1])
	user := make([]byte, userLen)
	if _, err := io.ReadFull(c, user); err != nil {
		return err
	}
	if _, err := io.ReadFull(c, header[:1]); err != nil {
		return errors.New("failed to read password length")
	}
	passLen := int(header[0])
	pass := make([]byte, passLen)
	if _, err := io.ReadFull(c, pass); err != nil {
		return err
	}

	if common.CheckAuthWithAccountMap(string(user), string(pass), s.Task.Client.Cnf.U, s.Task.Client.Cnf.P, file.GetAccountMap(s.Task.MultiAccount), file.GetAccountMap(s.Task.UserAuth)) {
		if _, err := c.Write([]byte{userAuthVersion, authSuccess}); err != nil {
			return err
		}
		return nil
	} else {
		if _, err := c.Write([]byte{userAuthVersion, authFailure}); err != nil {
			return err
		}
		return errors.New("auth failed")
	}
}

func ProcessMix(c *conn.Conn, s *TunnelModeServer) error {
	switch s.Task.Mode {
	case "socks5":
		s.Task.Mode = "mixProxy"
		s.Task.HttpProxy = false
		s.Task.Socks5Proxy = true
	case "httpProxy":
		s.Task.Mode = "mixProxy"
		s.Task.HttpProxy = true
		s.Task.Socks5Proxy = false
	}

	var buf [2]byte
	if _, err := io.ReadFull(c, buf[:]); err != nil {
		logs.Warn("negotiation err %v", err)
		_ = c.Close()
		return err
	}

	if version := buf[0]; version != 5 {
		method := string(buf[:])
		if isHttpMethodPrefix(method) {
			if !s.Task.HttpProxy {
				logs.Warn("http proxy is disable, client %d request from: %v", s.Task.Client.Id, c.RemoteAddr())
				_ = c.Close()
				return errors.New("http proxy is disabled")
			}
			if err := ProcessHttp(c.SetRb(buf[:]), s); err != nil {
				logs.Warn("http proxy error: %v", err)
				_ = c.Close()
				return err
			}
			_ = c.Close()
			return nil
		}
		logs.Trace("Socks5 Buf: %s", buf[:])
		logs.Warn("only support socks5 and http, request from: %v", c.RemoteAddr())
		_ = c.Close()
		return errors.New("unknown protocol")
	}

	if !s.Task.Socks5Proxy {
		logs.Warn("socks5 proxy is disable, client %d request from: %v", s.Task.Client.Id, c.RemoteAddr())
		_ = c.Close()
		return errors.New("socks5 proxy is disabled")
	}

	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(c, methods); err != nil {
		logs.Warn("wrong method")
		_ = c.Close()
		return errors.New("wrong method")
	}
	supports := func(m byte) bool {
		for _, x := range methods {
			if x == m {
				return true
			}
		}
		return false
	}
	needAuth := (s.Task.Client.Cnf.U != "" && s.Task.Client.Cnf.P != "") ||
		(s.Task.MultiAccount != nil && len(s.Task.MultiAccount.AccountMap) > 0) ||
		(s.Task.UserAuth != nil && len(s.Task.UserAuth.AccountMap) > 0)

	if needAuth {
		if !supports(UserPassAuth) {
			_, _ = c.Write([]byte{5, 0xFF})
			_ = c.Close()
			return errors.New("no acceptable authentication method")
		}
		_, _ = c.Write([]byte{5, UserPassAuth})
		if err := s.SocksAuth(c); err != nil {
			_ = c.Close()
			logs.Warn("Validation failed: %v", err)
			return err
		}
	} else {
		if !supports(0x00) {
			_, _ = c.Write([]byte{5, 0xFF})
			_ = c.Close()
			return errors.New("no acceptable method (no-auth not offered)")
		}
		_, _ = c.Write([]byte{5, 0x00})
	}
	s.handleSocks5Request(c, s.Task.Client)
	return nil
}

func isHttpMethodPrefix(prefix string) bool {
	switch prefix {
	case "GE", "PO", "HE", "PU ", "DE", "OP", "CO", "TR", "PA", "PR", "MK", "MO", "LO", "UN", "RE", "AC", "SE", "LI":
		return true
	}
	return false
}

// ProcessUnified unified proxy: exit traffic is routed to online clients dynamically.
// The proxy username controls client selection:
//   - "auto"          : fully random online client per connection
//   - pure digits     : fixed client by Id (must be online)
//   - anything else   : sticky random client cached for Task.CacheTime minutes (default 10)
func ProcessUnified(c *conn.Conn, s *TunnelModeServer) error {
	var buf [2]byte
	if _, err := io.ReadFull(c, buf[:]); err != nil {
		logs.Warn("negotiation err %v", err)
		_ = c.Close()
		return err
	}

	if version := buf[0]; version != 5 {
		if isHttpMethodPrefix(string(buf[:])) {
			if err := ProcessUnifiedHttp(c.SetRb(buf[:]), s); err != nil {
				logs.Warn("unified http proxy error: %v", err)
				_ = c.Close()
				return err
			}
			_ = c.Close()
			return nil
		}
		logs.Warn("only support socks5 and http, request from: %v", c.RemoteAddr())
		_ = c.Close()
		return errors.New("unknown protocol")
	}

	// socks5: username is the routing key, so username/password auth is always required
	nMethods := int(buf[1])
	methods := make([]byte, nMethods)
	if _, err := io.ReadFull(c, methods); err != nil {
		logs.Warn("wrong method")
		_ = c.Close()
		return errors.New("wrong method")
	}
	supports := func(m byte) bool {
		for _, x := range methods {
			if x == m {
				return true
			}
		}
		return false
	}
	if !supports(UserPassAuth) {
		_, _ = c.Write([]byte{5, 0xFF})
		_ = c.Close()
		return errors.New("no acceptable authentication method")
	}
	_, _ = c.Write([]byte{5, UserPassAuth})
	username, err := s.unifiedSocksAuth(c)
	if err != nil {
		_ = c.Close()
		logs.Warn("unified proxy validation failed: %v", err)
		return err
	}
	client, err := s.pickUnifiedClient(username)
	if err != nil {
		logs.Warn("unified proxy pick client failed: %v", err)
		_ = c.Close()
		return err
	}
	if s.Bridge.IsServer() {
		if err := s.CheckFlowAndConnNum(client); err != nil {
			logs.Warn("unified proxy client Id %d, task Id %d, error %v", client.Id, s.Task.Id, err)
			_ = c.Close()
			return err
		}
		defer client.CutConn()
	}
	s.handleSocks5Request(c, client)
	return nil
}

// ProcessUnifiedHttp http branch of unified proxy
func ProcessUnifiedHttp(c *conn.Conn, s *TunnelModeServer) error {
	_, addr, rb, r, err := c.GetHost()
	if err != nil {
		_ = c.Close()
		logs.Info("%v", err)
		return err
	}
	user, pass, _ := parseProxyBasicAuth(r)
	if s.Task.Password != "" && pass != s.Task.Password {
		_, _ = c.Write([]byte(common.ProxyAuthRequiredBytes))
		_ = c.Close()
		return errors.New("401 Unauthorized")
	}
	if user == "" {
		user = "auto"
	}
	client, err := s.pickUnifiedClient(user)
	if err != nil {
		logs.Warn("unified proxy pick client failed: %v", err)
		_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
		_ = c.Close()
		return err
	}
	if s.Bridge.IsServer() {
		if err := s.CheckFlowAndConnNum(client); err != nil {
			logs.Warn("unified proxy client Id %d, task Id %d, error %v", client.Id, s.Task.Id, err)
			_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
			_ = c.Close()
			return err
		}
		defer client.CutConn()
	}
	return s.handleHttpProxy(c, client, addr, rb, r)
}

// parseProxyBasicAuth extract credentials from Proxy-Authorization header.
// r.BasicAuth() reads the Authorization header, which proxies must not use.
func parseProxyBasicAuth(r *http.Request) (user, pass string, ok bool) {
	h := r.Header.Get("Proxy-Authorization")
	if h == "" {
		return "", "", false
	}
	const prefix = "Basic "
	if !strings.HasPrefix(h, prefix) && !strings.HasPrefix(strings.ToLower(h), strings.ToLower(prefix)) {
		return "", "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(h[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	if i := bytes.IndexByte(decoded, ':'); i >= 0 {
		return string(decoded[:i]), string(decoded[i+1:]), true
	}
	return string(decoded), "", true
}

// unifiedCheckAuth unified proxy auth: empty password accepts anyone
func unifiedCheckAuth(task *file.Tunnel, user, pass string) bool {
	if task.Password == "" || pass == task.Password {
		return true
	}
	return false
}

// unifiedSocksAuth socks5 username/password auth for unified proxy, returns the username
func (s *TunnelModeServer) unifiedSocksAuth(c net.Conn) (string, error) {
	var header [2]byte
	if _, err := io.ReadFull(c, header[:]); err != nil {
		return "", err
	}
	if header[0] != userAuthVersion {
		return "", errors.New("auth method not supported")
	}
	userLen := int(header[1])
	user := make([]byte, userLen)
	if _, err := io.ReadFull(c, user); err != nil {
		return "", err
	}
	if _, err := io.ReadFull(c, header[:1]); err != nil {
		return "", errors.New("failed to read password length")
	}
	passLen := int(header[0])
	pass := make([]byte, passLen)
	if _, err := io.ReadFull(c, pass); err != nil {
		return "", err
	}
	if !unifiedCheckAuth(s.Task, string(user), string(pass)) {
		if _, err := c.Write([]byte{userAuthVersion, authFailure}); err != nil {
			return "", err
		}
		return "", errors.New("auth failed")
	}
	if _, err := c.Write([]byte{userAuthVersion, authSuccess}); err != nil {
		return "", err
	}
	return string(user), nil
}

// unifiedOnlineClients returns all online clients available for unified proxy egress
func unifiedOnlineClients() []*file.Client {
	list := make([]*file.Client, 0)
	file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
		c := value.(*file.Client)
		if c.Id > 0 && c.Status && c.IsConnect {
			list = append(list, c)
		}
		return true
	})
	sort.Slice(list, func(i, j int) bool { return list[i].Id < list[j].Id })
	return list
}

type unifiedCacheEntry struct {
	clientId int
	expireAt time.Time
}

var unifiedStickyCache sync.Map // key: taskId:username -> unifiedCacheEntry

// pickUnifiedClient choose the exit client according to the username routing rules
func (s *TunnelModeServer) pickUnifiedClient(username string) (*file.Client, error) {
	candidates := unifiedOnlineClients()
	if len(candidates) == 0 {
		return nil, errors.New("unified proxy: no online clients available")
	}

	if username == "auto" {
		return candidates[rand.Intn(len(candidates))], nil
	}
	if id, err := strconv.Atoi(username); err == nil {
		// pure digits: fixed client by Id
		for _, c := range candidates {
			if c.Id == id {
				return c, nil
			}
		}
		return nil, fmt.Errorf("unified proxy: client %d is not online", id)
	}

	// sticky: random client cached for CacheTime minutes
	cacheTime := s.Task.CacheTime
	if cacheTime <= 0 {
		cacheTime = 10
	}
	key := fmt.Sprintf("%d:%s", s.Task.Id, username)
	now := time.Now()
	if v, ok := unifiedStickyCache.Load(key); ok {
		e := v.(unifiedCacheEntry)
		if now.Before(e.expireAt) {
			for _, c := range candidates {
				if c.Id == e.clientId {
					return c, nil
				}
			}
		}
		unifiedStickyCache.Delete(key)
	}
	picked := candidates[rand.Intn(len(candidates))]
	unifiedStickyCache.Store(key, unifiedCacheEntry{
		clientId: picked.Id,
		expireAt: now.Add(time.Duration(cacheTime) * time.Minute),
	})
	return picked, nil
}
