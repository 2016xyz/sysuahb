package file

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseShareLink turns one proxy share link into a ProxyNode.
//
// Supported:
//
//	ss://method:password@host:port
//	ss://BASE64(method:password)@host:port#name
//	ssr://BASE64(host:port:protocol:method:obfs:BASE64(password)/?remarks=)
//	vmess://BASE64(json)
//	vless://uuid@host:port?security=tls&sni=...#name
//	trojan://password@host:port?sni=...#name
//	tuic://uuid:password@host:port?sni=...#name
//	hysteria2://password@host:port?sni=...#name   (hy2:// also accepted)
//	naive+https://user:pass@host:port#name
//	http://user:pass@host:port
//	socks5://user:pass@host:port
//
// The returned string is the dedupe key protocol.
func ParseShareLink(line string) (*ProxyNode, string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, "", fmt.Errorf("empty line")
	}
	scheme, rest, ok := splitScheme(line)
	if !ok {
		return nil, "", fmt.Errorf("missing scheme")
	}
	switch scheme {
	case "ss":
		return parseSS(rest)
	case "ssr":
		return parseSSR(rest)
	case "vmess":
		return parseVMess(rest)
	case "vless":
		return parseVLESS(rest)
	case "trojan":
		return parseTrojan(rest)
	case "tuic":
		return parseUserPassScheme(SchemeTUIC, rest)
	case "hysteria2", "hy2":
		return parseUserPassScheme(SchemeHY2, rest)
	case "naive+https", "naive":
		return parseNaive(rest)
	case "http", "https":
		return parsePlain(SchemeHTTP, line)
	case "socks5", "socks":
		return parsePlain(SchemeSOCKS5, line)
	default:
		return nil, "", fmt.Errorf("unsupported scheme %q", scheme)
	}
}

func splitScheme(line string) (string, string, bool) {
	i := strings.Index(line, "://")
	if i <= 0 {
		return "", "", false
	}
	return strings.ToLower(line[:i]), line[i+3:], true
}

// parseSS handles both the SIP002 form and the legacy base64 form.
func parseSS(rest string) (*ProxyNode, string, error) {
	name := ""
	if i := strings.Index(rest, "#"); i >= 0 {
		name, _ = url.QueryUnescape(rest[i+1:])
		rest = rest[:i]
	}
	rest = strings.SplitN(rest, "?", 2)[0]

	var userinfo, hostport string
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		userinfo = rest[:at]
		hostport = rest[at+1:]
		if decoded, err := decodeB64(userinfo); err == nil && strings.Contains(decoded, ":") {
			userinfo = decoded
		}
	} else {
		decoded, err := decodeB64(rest)
		if err != nil {
			return nil, "", fmt.Errorf("ss: %w", err)
		}
		at = strings.LastIndex(decoded, "@")
		if at < 0 {
			return nil, "", fmt.Errorf("ss: malformed")
		}
		userinfo = decoded[:at]
		hostport = decoded[at+1:]
	}
	method, password, ok := strings.Cut(userinfo, ":")
	if !ok || method == "" {
		return nil, "", fmt.Errorf("ss: missing method")
	}
	host, port, err := splitHostPort(hostport, 0)
	if err != nil {
		return nil, "", fmt.Errorf("ss: %w", err)
	}
	node := NewProxyNode()
	node.Scheme = SchemeSS
	node.Method = method
	node.Password = password
	node.Host = host
	node.Port = port
	node.Name = firstNonEmpty(name, host)
	return node, SchemeSS, nil
}

func parseSSR(rest string) (*ProxyNode, string, error) {
	decoded, err := decodeB64(strings.SplitN(rest, "#", 2)[0])
	if err != nil {
		return nil, "", fmt.Errorf("ssr: %w", err)
	}
	main, query, _ := strings.Cut(decoded, "/?")
	parts := strings.Split(main, ":")
	if len(parts) < 6 {
		return nil, "", fmt.Errorf("ssr: malformed")
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port <= 0 || port > 65535 {
		return nil, "", fmt.Errorf("ssr: bad port")
	}
	password, err := decodeB64(parts[5])
	if err != nil {
		password = parts[5]
	}
	node := NewProxyNode()
	node.Scheme = SchemeSSR
	node.Host = parts[0]
	node.Port = port
	node.Method = parts[3]
	node.Password = password
	node.Name = parts[0]
	if q, err := url.ParseQuery(query); err == nil {
		if r := q.Get("remarks"); r != "" {
			if name, err := decodeB64(r); err == nil {
				node.Name = name
			}
		}
	}
	return node, SchemeSSR, nil
}

func parseVMess(rest string) (*ProxyNode, string, error) {
	decoded, err := decodeB64(strings.SplitN(rest, "#", 2)[0])
	if err != nil {
		return nil, "", fmt.Errorf("vmess: %w", err)
	}
	var v struct {
		Ps   string      `json:"ps"`
		Add  string      `json:"add"`
		Port interface{} `json:"port"`
		ID   string      `json:"id"`
		Aid  interface{} `json:"aid"`
		Net  string      `json:"net"`
		TLS  string      `json:"tls"`
		Sni  string      `json:"sni"`
		Host string      `json:"host"`
		Path string      `json:"path"`
		Scy  string      `json:"scy"`
	}
	if err = json.Unmarshal([]byte(decoded), &v); err != nil {
		return nil, "", fmt.Errorf("vmess: %w", err)
	}
	port := toInt(v.Port)
	if v.Add == "" || port == 0 || v.ID == "" {
		return nil, "", fmt.Errorf("vmess: incomplete")
	}
	node := NewProxyNode()
	node.Scheme = SchemeVMess
	node.Host = v.Add
	node.Port = port
	node.Password = v.ID
	node.AlterId = toInt(v.Aid)
	node.Method = v.Scy
	node.Network = v.Net
	node.TLS = strings.EqualFold(v.TLS, "tls")
	node.SNI = v.Sni
	node.HostHeader = v.Host
	node.Path = v.Path
	node.Name = firstNonEmpty(v.Ps, v.Add)
	return node, SchemeVMess, nil
}

func parseVLESS(rest string) (*ProxyNode, string, error) {
	return parseStdURL(SchemeVLESS, "vless://"+rest, func(node *ProxyNode, u *url.URL) {
		node.Password = u.User.Username()
		q := u.Query()
		node.Flow = q.Get("flow")
		node.Network = q.Get("type")
		node.SNI = firstNonEmpty(q.Get("sni"), q.Get("peer"))
		node.Path = q.Get("path")
		node.HostHeader = q.Get("host")
		node.ALPN = q.Get("alpn")
		node.TLS = q.Get("security") == "tls" || q.Get("security") == "reality"
		node.SkipVerify = q.Get("allowInsecure") == "1"
	})
}

func parseTrojan(rest string) (*ProxyNode, string, error) {
	return parseStdURL(SchemeTrojan, "trojan://"+rest, func(node *ProxyNode, u *url.URL) {
		node.Password = u.User.Username()
		q := u.Query()
		node.SNI = firstNonEmpty(q.Get("sni"), q.Get("peer"))
		node.Network = q.Get("type")
		node.Path = q.Get("path")
		node.HostHeader = q.Get("host")
		node.ALPN = q.Get("alpn")
		node.TLS = true
		node.SkipVerify = q.Get("allowInsecure") == "1"
	})
}

func parseUserPassScheme(scheme, rest string) (*ProxyNode, string, error) {
	return parseStdURL(scheme, scheme+"://"+rest, func(node *ProxyNode, u *url.URL) {
		node.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			node.Password = pw
		} else {
			node.Password = node.Username
			node.Username = ""
		}
		q := u.Query()
		node.SNI = q.Get("sni")
		node.ALPN = q.Get("alpn")
		node.TLS = true
		node.SkipVerify = q.Get("insecure") == "1" || q.Get("allowInsecure") == "1"
	})
}

func parseNaive(rest string) (*ProxyNode, string, error) {
	return parseStdURL(SchemeNaive, "https://"+rest, func(node *ProxyNode, u *url.URL) {
		node.Username = u.User.Username()
		node.Password, _ = u.User.Password()
		node.TLS = true
	})
}

func parsePlain(scheme, line string) (*ProxyNode, string, error) {
	return parseStdURL(scheme, line, func(node *ProxyNode, u *url.URL) {
		if u.User != nil {
			node.Username = u.User.Username()
			node.Password, _ = u.User.Password()
		}
		if scheme == SchemeHTTP {
			node.Http = true
		} else {
			node.Socks5 = true
		}
	})
}

// parseStdURL handles the schemes that are plain URLs.
func parseStdURL(scheme, raw string, fill func(*ProxyNode, *url.URL)) (*ProxyNode, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", scheme, err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, "", fmt.Errorf("%s: missing host", scheme)
	}
	portStr := u.Port()
	port := 0
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return nil, "", fmt.Errorf("%s: bad port", scheme)
		}
	}
	if port == 0 {
		port = defaultPort(scheme)
	}
	node := NewProxyNode()
	node.Scheme = scheme
	node.Host = host
	node.Port = port
	if u.User != nil {
		fill(node, u)
	} else {
		fill(node, u)
	}
	name, _ := url.QueryUnescape(u.Fragment)
	node.Name = firstNonEmpty(name, host)
	node.Link = raw
	return node, scheme, nil
}

func defaultPort(scheme string) int {
	switch scheme {
	case SchemeHTTP:
		return 80
	case SchemeTrojan, SchemeVLESS, SchemeNaive:
		return 443
	case SchemeHY2:
		return 443
	default:
		return 443
	}
}

func splitHostPort(hostport string, defPort int) (string, int, error) {
	host, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		if defPort > 0 {
			return hostport, defPort, nil
		}
		return "", 0, fmt.Errorf("bad address %q", hostport)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("bad port %q", portStr)
	}
	return host, port, nil
}

// DecodeBase64Loose decodes standard or URL-safe base64, with or without
// padding. Exported for the subscription importer.
func DecodeBase64Loose(s string) (string, error) {
	return decodeB64(s)
}

func decodeB64(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty")
	}
	if m := len(s) % 4; m != 0 {
		s += strings.Repeat("=", 4-m)
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), nil
		}
	}
	return "", fmt.Errorf("not base64")
}

func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
