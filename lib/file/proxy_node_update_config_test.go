package file

import "testing"

func TestProxyNodeUpdateConfigKeepsPassword(t *testing.T) {
	p := NewProxyNode()
	p.Id = 1
	p.Username = "u"
	p.Password = "secret"
	p.UpdateConfig(ProxyConfig{Name: "n", Host: "h", Port: 1, Username: "u2", Password: "", Http: true, Enabled: true})
	if p.Password != "secret" {
		t.Fatalf("empty password must keep the old one, got %q", p.Password)
	}
	if p.Username != "u2" {
		t.Fatalf("username must update")
	}
	p.UpdateConfig(ProxyConfig{Name: "n", Host: "h", Port: 1, Username: "u2", Password: "new", Http: true, Enabled: true})
	if p.Password != "new" {
		t.Fatalf("non-empty password must replace, got %q", p.Password)
	}
}
