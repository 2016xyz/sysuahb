package controllers

import (
	"net"
	"strings"
	"testing"
)

// The Clash-subscription importer fetches a URL supplied by the operator.
// Without validation that is server-side request forgery: the server would
// fetch loopback services or the cloud metadata endpoint on the caller's
// behalf. These tests pin the address filter that blocks it.
func TestIsPublicIPRejectsInternalRanges(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"127.1.2.3",       // loopback, not just .0.1
		"10.0.0.1",        // RFC1918
		"172.16.0.1",      // RFC1918
		"172.31.255.254",  // RFC1918 upper bound
		"192.168.1.1",     // RFC1918
		"169.254.169.254", // link-local: cloud metadata
		"169.254.0.1",     // link-local
		"0.0.0.0",         // unspecified
		"100.64.0.1",      // carrier-grade NAT
		"100.127.255.1",   // CGNAT upper bound
		"192.0.0.1",       // IETF protocol assignments
		"224.0.0.1",       // multicast
		"::1",             // IPv6 loopback
		"fc00::1",         // IPv6 unique local
		"fe80::1",         // IPv6 link-local
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if isPublicIP(ip) {
			t.Errorf("isPublicIP(%s) = true, want false", raw)
		}
	}
}

func TestIsPublicIPAcceptsGlobalUnicast(t *testing.T) {
	allowed := []string{"8.8.8.8", "1.1.1.1", "9.9.9.9", "2001:4860:4860::8888"}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if !isPublicIP(ip) {
			t.Errorf("isPublicIP(%s) = false, want true", raw)
		}
	}
}

func TestResolvePublicIPsRejectsLiteralInternalAddress(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "10.1.1.1"} {
		if _, err := resolvePublicIPs(raw); err == nil {
			t.Errorf("resolvePublicIPs(%s) returned no error, want rejection", raw)
		}
	}
	ips, err := resolvePublicIPs("8.8.8.8")
	if err != nil {
		t.Fatalf("resolvePublicIPs(8.8.8.8): %v", err)
	}
	if len(ips) != 1 {
		t.Fatalf("resolvePublicIPs(8.8.8.8) returned %d addresses, want 1", len(ips))
	}
}

// fetchSubscription must refuse anything that is not a public http/https URL.
func TestFetchSubscriptionRejectsDangerousURLs(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"file:///etc/passwd", "scheme"},
		{"gopher://127.0.0.1:6379/_info", "scheme"},
		{"ftp://example.com/x", "scheme"},
		{"http://127.0.0.1:8080/x", "public"},
		{"http://169.254.169.254/latest/meta-data/", "public"},
		{"http://[::1]:80/x", "public"},
		{"http://10.0.0.5/x", "public"},
		{"", "scheme"},
	}
	for _, tc := range cases {
		_, err := fetchSubscription(tc.url)
		if err == nil {
			t.Errorf("fetchSubscription(%q) succeeded, want an error", tc.url)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), tc.want) {
			t.Errorf("fetchSubscription(%q) error = %q, want it to mention %q", tc.url, err, tc.want)
		}
	}
}
