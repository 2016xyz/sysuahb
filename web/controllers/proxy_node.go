package controllers

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/svcmgr/lib/file"
	"example.com/svcmgr/server/proxy"
)

// ProxyNodeController manages external proxy nodes (HTTP / SOCKS5) that the
// unified proxy can use as an egress together with NPS clients.
type ProxyNodeController struct {
	BaseController
}

// List renders the page (GET) or returns the filtered table rows (POST).
func (s *ProxyNodeController) List() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "proxynode"
		s.SetInfo("proxy node")
		s.display("proxy_node/list")
		return
	}
	rows := s.filteredProxyRows()
	s.AjaxTable(rows, len(rows), len(rows), map[string]interface{}{})
}

// filteredProxyRows applies the list filters and returns view projections.
// The password never leaves the server: rows carry HasPassword only.
func (s *ProxyNodeController) filteredProxyRows() []file.ProxyNodeView {
	search := strings.ToLower(s.getEscapeString("search"))
	tag := strings.ToLower(s.getEscapeString("tag"))
	status := strings.ToLower(s.getEscapeString("status"))
	protocol := strings.ToLower(s.getEscapeString("protocol"))
	latencyMin := s.GetIntNoErr("latency_min")
	latencyMax := s.GetIntNoErr("latency_max")
	failures := s.GetIntNoErr("failures")

	out := make([]file.ProxyNodeView, 0)
	for _, node := range file.GetDb().ProxyNodeList() {
		if search != "" && !strings.Contains(strings.ToLower(node.Name), search) && !strings.Contains(strings.ToLower(node.Host), search) {
			continue
		}
		if tag != "" && !node.HasTag(tag) {
			continue
		}
		if status != "" && strings.ToLower(node.StatusValue()) != status {
			continue
		}
		if protocol != "" && !strings.Contains(strings.ToLower(node.Protocols()), protocol) {
			continue
		}
		if latencyMin > 0 || latencyMax > 0 {
			_, latency, _, _, _, _ := node.HealthSnapshot()
			if latencyMin > 0 && latency < int64(latencyMin) {
				continue
			}
			if latencyMax > 0 && (latency <= 0 || latency > int64(latencyMax)) {
				continue
			}
		}
		if failures > 0 {
			_, _, _, _, n, _ := node.HealthSnapshot()
			if n < failures {
				continue
			}
		}
		out = append(out, node.View())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}

// parseProxyForm validates the add/edit form and fills a node.
// When keepPassword is true an empty password keeps the current one.
func (s *ProxyNodeController) parseProxyForm(node *file.ProxyNode, keepPassword bool) error {
	name := strings.TrimSpace(s.GetString("name"))
	host := strings.TrimSpace(s.GetString("host"))
	port := s.GetIntNoErr("port")
	username := strings.TrimSpace(s.GetString("username"))
	password := s.GetString("password")
	http := s.GetBoolNoErr("http")
	socks5 := s.GetBoolNoErr("socks5")
	tagsText := s.GetString("tags")
	enabled := s.GetBoolNoErr("enabled")

	if host == "" {
		return fmt.Errorf("host is required")
	}
	if port <= 0 || port > 65535 {
		return fmt.Errorf("port must be within 1-65535")
	}
	if !http && !socks5 {
		return fmt.Errorf("at least one of HTTP / SOCKS5 must be enabled")
	}

	normalized, err := file.NormalizeTags(file.SplitTagsText(tagsText))
	if err != nil {
		return err
	}

	// Build a detached config; the caller applies it under the node lock so a
	// concurrent health check never observes half-written fields.
	node.UpdateConfig(file.ProxyConfig{
		Name:     name,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Http:     http,
		Socks5:   socks5,
		Tags:     normalized,
		Enabled:  enabled,
	})
	return nil
}

// Add creates one node (GET renders the form).
func (s *ProxyNodeController) Add() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "proxynode"
		s.SetInfo("add proxy")
		s.display("proxy_node/add")
		return
	}
	node := file.NewProxyNode()
	if err := s.parseProxyForm(node, false); err != nil {
		s.AjaxErr(err.Error())
		return
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

// Edit updates one node; an empty password field keeps the stored one.
func (s *ProxyNodeController) Edit() {
	id := s.GetIntNoErr("id")
	if s.Ctx.Request.Method == "GET" {
		node, err := file.GetDb().GetProxyNode(id)
		if err != nil || node == nil {
			s.error()
			return
		}
		s.Data["menu"] = "proxynode"
		s.Data["p"] = node
		s.Data["Tags"] = strings.Join(node.TagList(), "\n")
		s.SetInfo("edit proxy")
		s.display("proxy_node/edit")
		return
	}
	node, err := file.GetDb().GetProxyNode(id)
	if err != nil || node == nil {
		s.AjaxErr("proxy not found")
		return
	}
	if err := s.parseProxyForm(node, true); err != nil {
		s.AjaxErr(err.Error())
		return
	}
	if err := file.GetDb().UpdateProxyNode(node); err != nil {
		s.AjaxErr(err.Error())
		return
	}
	s.AjaxOk("modified success")
}

// Del removes one node.
func (s *ProxyNodeController) Del() {
	id := s.GetIntNoErr("id")
	if err := file.GetDb().DelProxyNode(id); err != nil {
		s.AjaxErr("delete error")
		return
	}
	s.AjaxOk("delete success")
}

// ChangeStatus enables or disables one node.
func (s *ProxyNodeController) ChangeStatus() {
	id := s.GetIntNoErr("id")
	node, err := file.GetDb().GetProxyNode(id)
	if err != nil || node == nil {
		s.AjaxErr("proxy not found")
		return
	}
	enabled := s.GetBoolNoErr("enabled")
	node.Lock()
	node.Enabled = enabled
	node.Unlock()
	if !enabled {
		node.MarkDisabled()
	}
	_ = file.GetDb().UpdateProxyNode(node)
	s.AjaxOk("modified success")
}

// Check queues a health probe for one node and returns immediately.
func (s *ProxyNodeController) Check() {
	id := s.GetIntNoErr("id")
	if _, err := file.GetDb().GetProxyNode(id); err != nil {
		s.AjaxErr("proxy not found")
		return
	}
	proxy.CheckNodeNow(id)
	s.AjaxOk("check started")
}

// CheckAll queues probes for every node.
func (s *ProxyNodeController) CheckAll() {
	n := proxy.CheckAllNodes()
	s.AjaxOk("checking " + strconv.Itoa(n) + " node(s)")
}

// Import parses the batch import text and stores every valid, non-duplicate
// line. One bad line never aborts the batch: totals are reported instead.
func (s *ProxyNodeController) Import() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "proxynode"
		s.SetInfo("import proxy")
		s.display("proxy_node/import")
		return
	}

	text := s.GetString("list")
	defaultTags, err := file.NormalizeTags(file.SplitTagsText(s.GetString("tags")))
	if err != nil {
		s.AjaxErr(err.Error())
		return
	}
	autoCheck := s.GetBoolNoErr("auto_check")

	total, success, duplicate, malformed, failed := 0, 0, 0, 0, 0
	seen := make(map[string]struct{})
	existing := make(map[string]struct{})
	for _, node := range file.GetDb().ProxyNodeList() {
		if node.SupportsHttp() {
			existing[node.DuplicateKey("http")] = struct{}{}
		}
		if node.SupportsSocks5() {
			existing[node.DuplicateKey("socks5")] = struct{}{}
		}
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		total++
		parsed, proto, perr := parseImportLine(line)
		if perr != nil {
			malformed++
			continue
		}
		parsed.Tags = append(append([]string{}, parsed.Tags...), defaultTags...)
		key := parsed.DuplicateKey(proto)
		if _, ok := seen[key]; ok {
			duplicate++
			continue
		}
		if _, ok := existing[key]; ok {
			duplicate++
			continue
		}
		seen[key] = struct{}{}
		if err := file.GetDb().NewProxyNode(parsed); err != nil {
			failed++
			continue
		}
		success++
		if autoCheck {
			proxy.CheckNodeNow(parsed.Id)
		}
	}
	s.AjaxOk(fmt.Sprintf("total:%d success:%d duplicate:%d malformed:%d failed:%d", total, success, duplicate, malformed, failed))
}

// BatchOps applies one operation to the selected node ids.
// op: check | enable | disable | add_tag | remove_tag | delete
func (s *ProxyNodeController) BatchOps() {
	op := strings.ToLower(s.getEscapeString("op"))
	ids := parseIDList(s.getEscapeString("ids"))
	if len(ids) == 0 {
		s.AjaxErr("no proxy selected")
		return
	}
	tags, err := file.NormalizeTags(file.SplitTagsText(s.GetString("tags")))
	if err != nil {
		s.AjaxErr(err.Error())
		return
	}
	affected := 0
	for _, id := range ids {
		node, err := file.GetDb().GetProxyNode(id)
		if err != nil || node == nil {
			continue
		}
		switch op {
		case "check":
			proxy.CheckNodeNow(node.Id)
			affected++
		case "enable", "disable":
			node.Lock()
			node.Enabled = op == "enable"
			node.Unlock()
			if op == "disable" {
				node.MarkDisabled()
			}
			_ = file.GetDb().UpdateProxyNode(node)
			affected++
		case "add_tag", "remove_tag":
			if len(tags) == 0 {
				continue
			}
			if op == "add_tag" {
				_ = node.AddTags(tags)
			} else {
				node.RemoveTags(tags)
			}
			_ = file.GetDb().UpdateProxyNode(node)
			affected++
		case "delete":
			_ = file.GetDb().DelProxyNode(node.Id)
			affected++
		}
	}
	s.AjaxOk("affected " + strconv.Itoa(affected) + " node(s)")
}

// DeleteInvalid removes nodes that match Status=Unavailable AND
// ConsecutiveFailures>=3, with optional stricter or time-based filters.
// filter: empty | failures3 | failures5 | no_success_24h | no_success_7d
func (s *ProxyNodeController) DeleteInvalid() {
	filter := strings.ToLower(s.getEscapeString("filter"))
	if filter == "" {
		filter = "failures3"
	}
	preview := s.GetBoolNoErr("preview")
	threshold := 3
	switch filter {
	case "failures5":
		threshold = 5
	case "no_success_24h", "no_success_7d":
		threshold = 3
	default:
		threshold = 3
	}
	cutoff := time.Time{}
	if filter == "no_success_24h" {
		cutoff = time.Now().Add(-24 * time.Hour)
	} else if filter == "no_success_7d" {
		cutoff = time.Now().Add(-7 * 24 * time.Hour)
	}

	targets := make([]int, 0)
	for _, node := range file.GetDb().ProxyNodeList() {
		status, _, _, lastSuccess, failures, _ := node.HealthSnapshot()
		// Strict match on the documented rule: only a node the health checker
		// itself marked Unavailable is a candidate. An operator-disabled node
		// is skipped on purpose -- switching it off is a deliberate choice,
		// not evidence that it is broken, so it must never be swept up here.
		if !node.EnabledState() || status != file.ProxyStatusUnavailable {
			continue
		}
		if failures < threshold {
			continue
		}
		if !cutoff.IsZero() && lastSuccess.After(cutoff) {
			continue
		}
		targets = append(targets, node.Id)
	}
	if preview {
		s.Ctx.Output.JSON(map[string]interface{}{"status": 1, "msg": "ok", "count": len(targets)}, false, false)
		s.StopRun()
	}

	for _, id := range targets {
		_ = file.GetDb().DelProxyNode(id)
	}
	s.AjaxOk("deleted " + strconv.Itoa(len(targets)) + " invalid node(s)")
}

// Settings reads and writes the unified-proxy settings.
func (s *ProxyNodeController) Settings() {
	if s.Ctx.Request.Method == "GET" {
		s.Data["menu"] = "proxysettings"
		s.Data["u"] = file.GetDb().JsonDb.GetUnifiedSettings().Snapshot()
		s.SetInfo("unified settings")
		s.display("proxy_node/settings")
		return
	}
	settings := file.GetDb().JsonDb.GetUnifiedSettings()
	next := file.UnifiedSettingsCopy{
		CheckURL:          strings.TrimSpace(s.GetString("check_url")),
		CheckInterval:     s.GetIntNoErr("check_interval"),
		RetryInterval:     s.GetIntNoErr("retry_interval"),
		CheckTimeout:      s.GetIntNoErr("check_timeout"),
		FailThreshold:     s.GetIntNoErr("fail_threshold"),
		RecoverSuccess:    s.GetIntNoErr("recover_success"),
		MaxConcurrency:    s.GetIntNoErr("max_concurrency"),
		MinTTL:            s.GetIntNoErr("min_ttl"),
		MaxTTL:            s.GetIntNoErr("max_ttl"),
		AutoCheckOnAdd:    s.GetBoolNoErr("auto_check_add"),
		AutoCheckOnImport: s.GetBoolNoErr("auto_check_import"),
	}
	// Locked update: the health scheduler reads these fields concurrently.
	settings.Update(&next)
	file.GetDb().JsonDb.StoreUnifiedToJsonFile()
	s.AjaxOk("modified success")
}

// parseIDList parses "1,2,3" or "1 2 3" into ids.
func parseIDList(raw string) []int {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == ';' || r == '\n' || r == '\t'
	})
	ids := make([]int, 0, len(fields))
	for _, f := range fields {
		id, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || id <= 0 {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// parseImportLine parses one batch-import line:
//
//	http://user:pass@1.2.3.4:8080
//	socks5://user:pass@2.3.4.5:1080
//	http://3.4.5.6:8080
//	socks5://4.5.6.7:1080
//
// It returns the node, the dedupe protocol and an error for bad lines.
func parseImportLine(line string) (*file.ProxyNode, string, error) {
	u, err := url.Parse(line)
	if err != nil {
		return nil, "", err
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "socks5" && scheme != "socks" {
		return nil, "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return nil, "", fmt.Errorf("missing host")
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, "", fmt.Errorf("missing port")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, "", fmt.Errorf("invalid port")
	}
	node := file.NewProxyNode()
	node.Name = host
	node.Host = host
	node.Port = port
	node.Enabled = true
	if u.User != nil {
		node.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			node.Password = pw
		}
	}
	proto := "http"
	if scheme == "http" {
		node.Http = true
	} else {
		node.Socks5 = true
		proto = "socks5"
	}
	return node, proto, nil
}
