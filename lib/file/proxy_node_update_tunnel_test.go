package file

import "testing"

// Saving the plain HTTP/SOCKS5 edit form must never wipe a tunnel node's
// scheme. ProxyConfig used to carry the tunnel fields and UpdateConfig copied
// them unconditionally, so the empty form values cleared Scheme and left an
// SS/VMess/Trojan node with no protocol at all.
func TestUpdateConfigPreservesTunnelFields(t *testing.T) {
	node := NewProxyNode()
	node.Id = 7101
	node.Scheme = SchemeVMess
	node.Method = "aes-128-gcm"
	node.Host = "1.2.3.4"
	node.Port = 443
	node.Password = "uuid-1111"
	node.Network = "ws"
	node.Path = "/ray"
	node.HostHeader = "cdn.example.com"
	node.TLS = true
	node.SNI = "cdn.example.com"
	node.Link = "vmess://eyJhZGQiOiIxLjIuMy40In0="

	// Exactly what parseProxyForm produces from the 代理节点 form: no tunnel
	// fields are known there, so they are all zero values.
	node.UpdateConfig(ProxyConfig{
		Name:     "renamed",
		Host:     "1.2.3.4",
		Port:     443,
		Username: "",
		Password: "",
		Http:     false,
		Socks5:   false,
		Tags:     []string{"jp"},
		Enabled:  true,
	})

	if got := node.SchemeName(); got != SchemeVMess {
		t.Fatalf("scheme after saving the plain form = %q, want %q", got, SchemeVMess)
	}
	if !node.IsTunnel() {
		t.Fatal("a tunnel node stopped being a tunnel node after a plain-form save")
	}
	if got := node.MethodName(); got != "aes-128-gcm" {
		t.Fatalf("method = %q, want aes-128-gcm", got)
	}
	if got := node.PasswordValue(); got != "uuid-1111" {
		t.Fatalf("password = %q, want uuid-1111 (blank form field must keep it)", got)
	}
	if !node.TLSEnabled() || node.SNIName() != "cdn.example.com" {
		t.Fatal("TLS settings were cleared by the plain-form save")
	}
	if node.Path != "/ray" || node.HostHeader != "cdn.example.com" {
		t.Fatalf("transport settings were cleared: path=%q host=%q", node.Path, node.HostHeader)
	}
	if node.Link == "" {
		t.Fatal("share link was cleared")
	}
	// The plain fields the form does own must still be applied.
	if node.Name != "renamed" || node.Scheme == "" {
		t.Fatalf("plain fields not applied: name=%q", node.Name)
	}
	if !node.HasTag("jp") {
		t.Fatal("tags from the form were not applied")
	}
}

// The plain fields must still be editable: the fix must not turn UpdateConfig
// into a no-op.
func TestUpdateConfigStillUpdatesPlainFields(t *testing.T) {
	node := NewProxyNode()
	node.Id = 7102
	node.Host = "10.0.0.1"
	node.Port = 8080
	node.Http = true

	node.UpdateConfig(ProxyConfig{
		Name: "gz-01", Host: "9.9.9.9", Port: 1080,
		Username: "u", Password: "p", Socks5: true, Enabled: true,
	})

	if node.HostValue() != "9.9.9.9" || node.Addr() != "9.9.9.9:1080" {
		t.Fatalf("host/port not updated: %s", node.Addr())
	}
	user, pass := node.Credentials()
	if user != "u" || pass != "p" {
		t.Fatalf("credentials not updated: %q/%q", user, pass)
	}
	if node.SupportsHttp() || !node.SupportsSocks5() {
		t.Fatal("protocol flags not updated")
	}
}
