package proxy

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"example.com/svcmgr/lib/file"
)

// TestHealthCheckConcurrencyCap verifies the MaxConcurrency setting: even with
// many nodes queued at once, the number of simultaneous probes must never
// exceed the configured cap.
func TestHealthCheckConcurrencyCap(t *testing.T) {
	var inFlight int64
	var peak int64

	// A tiny HTTP "proxy" that only tracks concurrent connections.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			cur := atomic.AddInt64(&inFlight, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if cur <= old || atomic.CompareAndSwapInt64(&peak, old, cur) {
					break
				}
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				time.Sleep(80 * time.Millisecond)
				_ = atomic.AddInt64(&inFlight, -1)
			}(c)
		}
	}()
	proxyHost, proxyPort := splitAddr(t, ln.Addr().String())

	withUnifiedSettings(t, func(s *file.UnifiedSettingsCopy) {
		s.CheckURL = "http://127.0.0.1:9/generate_204"
		s.CheckTimeout = 2
		s.MaxConcurrency = 4
		s.FailThreshold = 100 // keep nodes available-looking; we only count dials
	})

	db := file.GetDb()
	for i := 0; i < 20; i++ {
		node := file.NewProxyNode()
		node.Id = 9300 + i
		node.Host = proxyHost
		node.Port = proxyPort
		node.Http = true
		node.Enabled = true
		db.JsonDb.Proxies.Store(node.Id, node)
		t.Cleanup(func() { db.JsonDb.Proxies.Delete(node.Id) })
		CheckNodeNow(node.Id)
	}

	// Wait until every queued probe has dialed and finished (bounded).
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&inFlight) == 0 && atomic.LoadInt64(&peak) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if peak > 4 {
		t.Fatalf("concurrent probes peaked at %d, cap is 4", peak)
	}
	if peak == 0 {
		t.Fatalf("no probes were dialed at all")
	}
}

// TestRecoveryRejoinsPool verifies that a node marked Unavailable leaves the
// candidate pool, and re-enters it after the configured recovery successes.
func TestRecoveryRejoinsPool(t *testing.T) {
	node := newTestProxyNode(11, true, true, "gz")
	pool := testPool(nil, []*file.ProxyNode{node})

	if len(Candidates(pool, protoHttp, []string{"gz"})) != 1 {
		t.Fatalf("healthy node must be a candidate")
	}
	node.MarkCheckFailure("boom", time.Now(), 3)
	node.MarkCheckFailure("boom", time.Now(), 3)
	node.MarkCheckFailure("boom", time.Now(), 3)
	if len(Candidates(testPool(nil, []*file.ProxyNode{node}), protoHttp, []string{"gz"})) != 0 {
		t.Fatalf("3 consecutive failures must remove the node from the pool")
	}
	// Recover: one success (default RecoverSuccess=1) brings it back.
	node.MarkCheckSuccess(5*time.Millisecond, time.Now(), 1)
	if len(Candidates(testPool(nil, []*file.ProxyNode{node}), protoHttp, []string{"gz"})) != 1 {
		t.Fatalf("a recovered node must rejoin the pool")
	}
}

// TestDisabledNodeExcludedFromPool verifies the Enabled gate of the pool.
func TestDisabledNodeExcludedFromPool(t *testing.T) {
	node := newTestProxyNode(12, true, false, "gz")
	node.MarkDisabled()
	if len(Candidates(testPool(nil, []*file.ProxyNode{node}), protoHttp, nil)) != 0 {
		t.Fatalf("a disabled node must never be a candidate")
	}
}
