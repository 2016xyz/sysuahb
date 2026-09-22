package file

import (
	"encoding/base64"
	"testing"
)

func TestParseShareLinks(t *testing.T) {
	cases := []struct {
		line   string
		scheme string
		host   string
		port   int
		secret string
	}{
		{"ss://YWVzLTI1Ni1nY206cGFzcw@1.2.3.4:8388#tokyo", SchemeSS, "1.2.3.4", 8388, "pass"},
		{"ss://aes-256-gcm:pass@5.6.7.8:8388", SchemeSS, "5.6.7.8", 8388, "pass"},
		{"vmess://eyJwcyI6ImpwIiwicG9ydCI6IjQ0MyIsImlkIjoiYWFhYSIsImFpZCI6IjAiLCJuZXQiOiJ0Y3AiLCJ0bHMiOiJ0bHMiLCJhZGQiOiJleGFtcGxlLmNvbSJ9", SchemeVMess, "example.com", 443, "aaaa"},
		{"vless://bbbb@9.9.9.9:443?security=tls&sni=ex.com#name", SchemeVLESS, "9.9.9.9", 443, "bbbb"},
		{"trojan://secret@8.8.8.8:443?sni=h.com#t", SchemeTrojan, "8.8.8.8", 443, "secret"},
		{"tuic://uuid:pw@7.7.7.7:443?sni=h.com", SchemeTUIC, "7.7.7.7", 443, "pw"},
		{"hysteria2://pw@6.6.6.6:443?sni=h.com", SchemeHY2, "6.6.6.6", 443, "pw"},
		{"naive+https://user:pw@5.5.5.5:443#n", SchemeNaive, "5.5.5.5", 443, "pw"},
		{"socks5://u:p@4.4.4.4:1080", SchemeSOCKS5, "4.4.4.4", 1080, "p"},
		{"http://3.3.3.3:8080", SchemeHTTP, "3.3.3.3", 8080, ""},
	}
	for _, c := range cases {
		node, proto, err := ParseShareLink(c.line)
		if err != nil {
			t.Errorf("%s: %v", c.scheme, err)
			continue
		}
		if proto != c.scheme || node.Host != c.host || node.Port != c.port {
			t.Errorf("%s: got %s %s:%d", c.scheme, proto, node.Host, node.Port)
		}
		if c.secret != "" && node.Password != c.secret && node.Username != c.secret {
			t.Errorf("%s: secret not captured (pw=%q user=%q)", c.scheme, node.Password, node.Username)
		}
		if !node.IsTunnel() && (c.scheme != SchemeHTTP && c.scheme != SchemeSOCKS5) {
			t.Errorf("%s: expected tunnel", c.scheme)
		}
	}
}

func TestParseSSMethod(t *testing.T) {
	node, _, err := ParseShareLink("ss://aes-256-gcm:pass@1.2.3.4:8388")
	if err != nil {
		t.Fatal(err)
	}
	if node.Method != "aes-256-gcm" {
		t.Fatalf("method = %q", node.Method)
	}
}

func TestParseVMessTLS(t *testing.T) {
	node, _, err := ParseShareLink("vmess://eyJwcyI6ImpwIiwicG9ydCI6IjQ0MyIsImlkIjoiYWFhYSIsImFpZCI6IjIiLCJuZXQiOiJ3cyIsInRscyI6InRscyIsImFkZCI6ImV4YW1wbGUuY29tIiwic25pIjoicy5jb20iLCJwYXRoIjoiL3dzIn0=")
	if err != nil {
		t.Fatal(err)
	}
	if !node.TLS || node.SNI != "s.com" || node.Path != "/ws" || node.Network != "ws" || node.AlterId != 2 {
		t.Fatalf("vmess fields: tls=%v sni=%s path=%s net=%s aid=%d", node.TLS, node.SNI, node.Path, node.Network, node.AlterId)
	}
}

func TestParseSSR(t *testing.T) {
	// host:port:protocol:method:obfs:base64(password)
	raw := "1.2.3.4:1234:origin:aes-256-cfb:plain:cGFzcw==/?remarks=anA="
	node, proto, err := ParseShareLink("ssr://" + encodeB64(raw))
	if err != nil {
		t.Fatal(err)
	}
	if proto != SchemeSSR || node.Host != "1.2.3.4" || node.Port != 1234 || node.Method != "aes-256-cfb" || node.Password != "pass" || node.Name != "jp" {
		t.Fatalf("ssr: %+v", node)
	}
}

func TestParseRejectsBad(t *testing.T) {
	for _, line := range []string{"", "not a link", "ftp://1.2.3.4:21", "ss://%%%"} {
		if _, _, err := ParseShareLink(line); err == nil {
			t.Errorf("expected error for %q", line)
		}
	}
}

func encodeB64(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}
