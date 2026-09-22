package controllers

import (
	"fmt"
	"io"
	"net/http"
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
		existing[node.DuplicateKey("")] = struct{}{}
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
		node.Tags = append(node.Tags, defaultTags...)
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

func fetchSubscription(rawURL string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(rawURL)
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
