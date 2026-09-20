package proxy

import (
	"testing"
	"time"

	"github.com/djylb/nps/lib/file"
)

func newTestClient(id int, tags ...string) *file.Client {
	c := &file.Client{Id: id, Status: true, IsConnect: true, Flow: new(file.Flow), Cnf: new(file.Config)}
	c.Tags = tags
	return c
}

func newTestProxyNode(id int, http, socks5 bool, tags ...string) *file.ProxyNode {
	p := file.NewProxyNode()
	p.Id = id
	p.Host = "127.0.0.1"
	p.Port = 1080
	p.Http = http
	p.Socks5 = socks5
	p.Tags = tags
	p.Enabled = true
	p.MarkCheckSuccess(3*time.Millisecond, time.Now(), 1)
	return p
}

func testPool(cs []*file.Client, ps []*file.ProxyNode) []unifiedEgress {
	return PoolFromClientsAndProxies(cs, ps)
}

// --- 混合出口池 -------------------------------------------------------------

func TestEgressPoolMixesClientsAndProxies(t *testing.T) {
	pool := testPool(
		[]*file.Client{newTestClient(1, "gz")},
		[]*file.ProxyNode{newTestProxyNode(11, true, false, "gz")},
	)
	if got := len(Candidates(pool, protoHttp, nil)); got != 2 {
		t.Fatalf("mixed pool must contain both egresses, got %d", got)
	}
}

func TestEgressTagPoolIsolation(t *testing.T) {
	gzClient := newTestClient(1, "gz")
	jpClient := newTestClient(2, "jp")
	gzProxy := newTestProxyNode(11, true, true, "gz")
	jpProxy := newTestProxyNode(12, true, true, "jp")
	pool := testPool([]*file.Client{gzClient, jpClient}, []*file.ProxyNode{gzProxy, jpProxy})

	sel := NewEgressSelector(newMemoryStickyStore())
	for i := 0; i < 40; i++ {
		req, _ := ParseRoute("gz.auto")
		ref, err := sel.SelectFrom(req, time.Minute, protoHttp, pool)
		if err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}
		if ref == ClientRef(jpClient.Id) || ref == ProxyRef(jpProxy.Id) {
			t.Fatalf("gz.auto escaped its tag pool: %s", ref)
		}
	}
}

func TestEgressUnknownTagFails(t *testing.T) {
	pool := testPool([]*file.Client{newTestClient(1, "gz")}, nil)
	sel := NewEgressSelector(newMemoryStickyStore())
	req, _ := ParseRoute("kr.auto")
	if _, err := sel.SelectFrom(req, time.Minute, protoHttp, pool); err == nil {
		t.Fatalf("unknown tag must fail instead of falling back to the whole pool")
	}
}

func TestEgressFixedClientOfflineFails(t *testing.T) {
	pool := testPool([]*file.Client{newTestClient(1), newTestClient(2)}, nil)
	sel := NewEgressSelector(newMemoryStickyStore())

	req, _ := ParseRoute("12")
	if _, err := sel.SelectFrom(req, time.Minute, protoHttp, pool); err == nil {
		t.Fatalf("pinned client 12 is offline, selection must fail")
	}
	req, _ = ParseRoute("2")
	ref, err := sel.SelectFrom(req, time.Minute, protoHttp, pool)
	if err != nil || ref != ClientRef(2) {
		t.Fatalf("pinned client 2 => %v, %v", ref, err)
	}
}

func TestEgressEmptyPoolFails(t *testing.T) {
	sel := NewEgressSelector(newMemoryStickyStore())
	req, _ := ParseRoute("auto")
	if _, err := sel.SelectFrom(req, time.Minute, protoHttp, nil); err == nil {
		t.Fatalf("an empty pool must fail")
	}
}

// --- 协议匹配 ----------------------------------------------------------------

func TestHttpSelectionSkipsSocks5OnlyProxy(t *testing.T) {
	socksOnly := newTestProxyNode(11, false, true, "gz")
	httpOnly := newTestProxyNode(12, true, false, "gz")
	pool := testPool(nil, []*file.ProxyNode{socksOnly, httpOnly})

	sel := NewEgressSelector(newMemoryStickyStore())
	req, _ := ParseRoute("gz.auto")
	for i := 0; i < 40; i++ {
		ref, err := sel.SelectFrom(req, time.Minute, protoHttp, pool)
		if err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}
		if ref == ProxyRef(socksOnly.Id) {
			t.Fatalf("an HTTP request must never pick a SOCKS5-only proxy")
		}
	}
}

func TestSocks5SelectionSkipsHttpOnlyProxy(t *testing.T) {
	socksOnly := newTestProxyNode(11, false, true, "gz")
	httpOnly := newTestProxyNode(12, true, false, "gz")
	pool := testPool(nil, []*file.ProxyNode{socksOnly, httpOnly})

	sel := NewEgressSelector(newMemoryStickyStore())
	req, _ := ParseRoute("gz.auto")
	for i := 0; i < 40; i++ {
		ref, err := sel.SelectFrom(req, time.Minute, protoSocks5, pool)
		if err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}
		if ref == ProxyRef(httpOnly.Id) {
			t.Fatalf("a SOCKS5 request must never pick an HTTP-only proxy")
		}
	}
}

func TestEgressPoolMustMatchProtocol(t *testing.T) {
	pool := testPool(nil, []*file.ProxyNode{newTestProxyNode(11, true, false, "gz")})

	sel := NewEgressSelector(newMemoryStickyStore())
	req, _ := ParseRoute("gz.auto")
	if _, err := sel.SelectFrom(req, time.Minute, protoSocks5, pool); err == nil {
		t.Fatalf("no SOCKS5-capable egress: selection must fail, not fall back")
	}
}

// --- Sticky：缓存 Client 或 Proxy，并逐项重校验 --------------------------------

func TestStickyCachesProxyRef(t *testing.T) {
	store := newMemoryStickyStore()
	sel := NewEgressSelector(store)
	proxy := newTestProxyNode(11, true, true, "gz")
	pool := testPool(nil, []*file.ProxyNode{proxy})

	req, _ := ParseRoute("abc.gz-auto")
	req.CacheKey = CacheKeyFor(7, "abc.gz-auto")

	ref, err := sel.SelectFrom(req, time.Hour, protoHttp, pool)
	if err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}
	if ref != ProxyRef(proxy.Id) {
		t.Fatalf("only one candidate: got %s", ref)
	}
	if cached, ok := store.Get(req.CacheKey); !ok || cached != ref {
		t.Fatalf("sticky must cache the proxy ref, got %v ok=%v", cached, ok)
	}
}

// TestStickyProxyChecksFailed verifies each invalidation rule for proxy refs.
func TestStickyProxyChecksFailed(t *testing.T) {
	// unavailable: a failing proxy is not in the pool at all
	{
		store := newMemoryStickyStore()
		sel := NewEgressSelector(store)
		proxy := newTestProxyNode(11, true, true, "gz")
		pool := testPool(nil, []*file.ProxyNode{proxy})

		req, _ := ParseRoute("abc.gz-auto")
		req.CacheKey = CacheKeyFor(7, "abc.gz-auto")
		if _, err := sel.SelectFrom(req, time.Hour, protoHttp, pool); err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}

		// Proxy becomes unhealthy -> it leaves the pool -> sticky must re-roll.
		proxy.MarkCheckFailure("boom", time.Now(), 1)
		if _, err := sel.SelectFrom(req, time.Hour, protoHttp, pool); err == nil {
			t.Fatalf("no candidate after the proxy went unhealthy: selection must fail")
		}
		if _, ok := store.Get(req.CacheKey); ok {
			t.Fatalf("the stale sticky entry must be dropped")
		}
	}
	// disabled / deleted / tag removed -> same treatment
	{
		store := newMemoryStickyStore()
		sel := NewEgressSelector(store)
		proxy := newTestProxyNode(11, true, true, "gz")
		pool := testPool(nil, []*file.ProxyNode{proxy})

		req, _ := ParseRoute("abc.gz-auto")
		req.CacheKey = CacheKeyFor(7, "abc.gz-auto")
		if _, err := sel.SelectFrom(req, time.Hour, protoHttp, pool); err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}

		proxy.RemoveTags([]string{"gz"})
		dropped := testPool(nil, []*file.ProxyNode{proxy})
		if _, err := sel.SelectFrom(req, time.Hour, protoHttp, dropped); err == nil {
			t.Fatalf("the tag is gone: gz.auto must fail")
		}

		proxy.SetTags([]string{"gz"})
		proxy.Lock()
		proxy.Enabled = false
		proxy.Unlock()
		if _, err := sel.SelectFrom(req, time.Hour, protoHttp, pool); err == nil {
			t.Fatalf("a disabled proxy must not be selected")
		}
	}
}

func TestFixedProxyLikeClientIDRejectedWhenMissing(t *testing.T) {
	// username=12 resolves to client id 12 only; without that client the
	// selection must fail even if a proxy with the same id exists.
	pool := testPool(nil, []*file.ProxyNode{newTestProxyNode(12, true, true)})
	sel := NewEgressSelector(newMemoryStickyStore())
	req, _ := ParseRoute("12")
	if _, err := sel.SelectFrom(req, time.Minute, protoHttp, pool); err == nil {
		t.Fatalf("pinned id 12 without an online client must fail")
	}
}
