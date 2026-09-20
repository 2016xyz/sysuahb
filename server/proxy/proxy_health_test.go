package proxy

import (
	"testing"

	"github.com/djylb/nps/lib/file"
)

func withUnifiedSettings(t *testing.T, mutate func(*file.UnifiedSettings)) {
	t.Helper()
	settings := file.GetDb().JsonDb.GetUnifiedSettings()
	snapshot := settings.Snapshot()
	next := snapshot
	mutate(&next)
	settings.Update(next)
	t.Cleanup(func() {
		settings.Update(snapshot)
	})
}

func TestRunProxyCheckSuccess(t *testing.T) {
	targetURL, closeTarget := startTinyHTTPServer(t, 204)
	defer closeTarget()
	proxyAddr, closeProxy := startFakeHttpProxy(t, "", "")
	defer closeProxy()

	withUnifiedSettings(t, func(s *file.UnifiedSettings) {
		s.CheckURL = targetURL
		s.FailThreshold = 3
		s.RecoverSuccess = 1
		s.CheckTimeout = 5
	})

	db := file.GetDb()
	node := file.NewProxyNode()
	node.Id = 9201
	node.Host, node.Port = splitAddr(t, proxyAddr)
	node.Http = true
	node.Enabled = true
	db.JsonDb.Proxies.Store(node.Id, node)
	t.Cleanup(func() { db.JsonDb.Proxies.Delete(node.Id) })

	runProxyCheck(node.Id)

	status, latency, _, _, failures, lastErr := node.HealthSnapshot()
	if status != file.ProxyStatusAvailable {
		t.Fatalf("status=%d latency=%d failures=%d err=%s", status, latency, failures, lastErr)
	}
	if latency <= 0 {
		t.Fatalf("latency must be recorded, got %d", latency)
	}
}

func TestRunProxyCheckFailureThresholdAndRecovery(t *testing.T) {
	withUnifiedSettings(t, func(s *file.UnifiedSettings) {
		s.CheckURL = "http://127.0.0.1:9/generate_204"
		s.FailThreshold = 3
		s.RecoverSuccess = 1
		s.CheckTimeout = 1
	})

	db := file.GetDb()
	node := file.NewProxyNode()
	node.Id = 9202
	node.Host = "127.0.0.1"
	node.Port = 9
	node.Http = true
	node.Enabled = true
	db.JsonDb.Proxies.Store(node.Id, node)
	t.Cleanup(func() { db.JsonDb.Proxies.Delete(node.Id) })

	runProxyCheck(node.Id)
	runProxyCheck(node.Id)
	if node.StatusValue() == "Unavailable" {
		t.Fatalf("must not flip before the threshold")
	}
	runProxyCheck(node.Id)
	if node.StatusValue() != "Unavailable" {
		t.Fatalf("three failures must flip to Unavailable, got %s", node.StatusValue())
	}
}
