package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/svcmgr/lib/file"
	"example.com/svcmgr/server/proxy"
)

// OmniProxyController is the "全能代理" page. It reuses the proxy-node store,
// egress selector and health checker, and only adds the share-link and
// Clash-subscription import on top.
type OmniProxyController struct {
	BaseController
}

func (s *OmniProxyController) List() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "omniproxy"
		s.SetInfo("omni proxy")
		s.display("omni_proxy/list")
		return
	}
	rows := make([]file.ProxyNodeView, 0)
	for _, node := range file.GetDb().ProxyNodeList() {
		if node.IsTunnel() {
			rows = append(rows, node.View())
		}
	}
	s.AjaxTable(rows, len(rows), len(rows), map[string]interface{}{})
}

func (s *OmniProxyController) Add() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "omniproxy"
		s.SetInfo("add omni proxy")
		s.display("omni_proxy/add")
		return
	}
	link := strings.TrimSpace(s.GetString("link"))
	node, _, err := file.ParseShareLink(link)
	if err != nil {
		s.AjaxErr(err.Error())
		return
	}
	if name := strings.TrimSpace(s.GetString("name")); name != "" {
		node.Name = name
	}
	if tags, err := file.NormalizeTags(file.SplitTagsText(s.GetString("tags"))); err == nil {
		node.Tags = tags
	}
	if err := file.GetDb().NewProxyNode(node); err != nil {
		s.AjaxErr(err.Error())
		return
	}
	if file.GetDb().JsonDb.GetUnifiedSettings().AutoCheckOnAdd {
		proxy.CheckNodeNow(node.Id)
	}
	s.AjaxOk("add success")
}

// Import accepts share links one per line, plus a Clash subscription URL.
func (s *OmniProxyController) Import() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "omniproxy"
		s.SetInfo("import omni proxy")
		s.display("omni_proxy/import")
		return
	}
	text := s.GetString("list")
	sub := strings.TrimSpace(s.GetString("subscription"))
	if sub != "" {
		body, err := fetchSubscription(sub)
		if err != nil {
			s.AjaxErr("subscription: " + err.Error())
			return
		}
		text += "\n" + body
	}
	defaultTags, err := file.NormalizeTags(file.SplitTagsText(s.GetString("tags")))
	if err != nil {
		s.AjaxErr(err.Error())
		return
	}
	autoCheck := s.GetBoolNoErr("auto_check")

	total, success, duplicate, malformed, failed := 0, 0, 0, 0, 0
	existing := map[string]struct{}{}
	for _, node := range file.GetDb().ProxyNodeList() {
		// A plain node occupies one key per protocol it speaks, so the set
		// must be built from DuplicateKeys. Using DuplicateKey("") produced
		// keys with an empty protocol that never matched the "http"/"socks5"
		// keys of incoming links, so re-importing an existing proxy created a
		// duplicate instead of being skipped.
		for _, key := range node.DuplicateKeys() {
			existing[key] = struct{}{}
		}
	}
	seen := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		total++
		node, proto, perr := file.ParseShareLink(line)
		if perr != nil {
			malformed++
			continue
		}
		merged, terr := file.NormalizeTags(append(append([]string{}, node.Tags...), defaultTags...))
		if terr != nil {
			malformed++
			continue
		}
		node.Tags = merged
		key := node.DuplicateKey(proto)
		if _, ok := seen[key]; ok {
			duplicate++
			continue
		}
		if _, ok := existing[key]; ok {
			duplicate++
			continue
		}
		seen[key] = struct{}{}
		if err := file.GetDb().NewProxyNode(node); err != nil {
			failed++
			continue
		}
		success++
		if autoCheck {
			proxy.CheckNodeNow(node.Id)
		}
	}
	s.AjaxOk(fmt.Sprintf("total:%d success:%d duplicate:%d malformed:%d failed:%d", total, success, duplicate, malformed, failed))
}

// fetchSubscription downloads a Clash subscription.
//
// The URL comes from an authenticated admin, but it is still server-side
// request forgery surface: without validation the server would happily fetch
// http://127.0.0.1:... or the cloud metadata endpoint on the admin's behalf.
// Only http/https is accepted and every resolved address must be public;
// redirects are re-validated on each hop.
func fetchSubscription(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid subscription url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("subscription url scheme %q is not allowed", u.Scheme)
	}
	if u.Hostname() == "" {
		return "", errors.New("subscription url has no host")
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			// The dialer resolves the name itself, rejects every non-public
			// answer and then connects to the validated IP literal. Dialling
			// by IP instead of by name closes the DNS-rebinding window that a
			// separate "check then connect by name" would leave open.
			DialContext:         safeDialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return assertPublicHost(req.URL.Hostname())
		},
	}
	if err = assertPublicHost(u.Hostname()); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	text := string(body)
	if decoded, err := file.DecodeBase64Loose(text); err == nil && strings.Contains(decoded, "://") {
		return decoded, nil
	}
	return text, nil
}

// resolvePublicIPs resolves host and returns its addresses, rejecting the
// whole host when any answer is non-public. A literal IP is validated as is.
func resolvePublicIPs(host string) ([]net.IP, error) {
	if host == "" {
		return nil, errors.New("empty host")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("subscription host %s is not a public address", host)
		}
		return []net.IP{ip}, nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve %s: no address", host)
	}
	public := make([]net.IP, 0, len(addrs))
	for _, ip := range addrs {
		if !isPublicIP(ip) {
			return nil, fmt.Errorf("subscription host %s resolves to a non-public address", host)
		}
		public = append(public, ip)
	}
	return public, nil
}

// safeDialContext resolves the target, validates every answer and connects to
// the validated IP literal. Dialling the IP rather than the name means a DNS
// answer that changes between the check and the connect (rebinding) cannot
// redirect the request to an internal address.
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := resolvePublicIPs(host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var lastErr error
	for _, ip := range ips {
		conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	return nil, fmt.Errorf("dial %s: %w", host, lastErr)
}

// assertPublicHost is the cheap pre-flight check for the URL and every
// redirect hop; safeDialContext enforces the same rule at connect time.
func assertPublicHost(host string) error {
	_, err := resolvePublicIPs(host)
	return err
}

// isPublicIP reports whether ip is a global unicast address that is safe to
// let the server connect to.
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		// 100.64.0.0/10 carrier-grade NAT and 192.0.0.0/24 IETF protocol
		// assignments are not reachable for a subscription fetch.
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return false
		}
	}
	return true
}
