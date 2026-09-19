package proxy

import (
	"encoding/base64"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/djylb/nps/lib/common"
	"github.com/djylb/nps/lib/conn"
	"github.com/djylb/nps/lib/file"
)

// ---------------------------------------------------------------------------
// RouteParser
// ---------------------------------------------------------------------------

func TestParseRoute(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     RouteRequest
		wantOK   bool
	}{
		// --- auto ---------------------------------------------------------
		{name: "auto is fully random", username: "auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeRandom, TTL: unifiedDefaultTTL}},
		{name: "auto with ttl is invalid", username: "auto-30m", wantOK: false},
		{name: "uppercase AUTO invalid", username: "AUTO", wantOK: false},

		// --- fixed client id ----------------------------------------------
		{name: "fixed client 12", username: "12", wantOK: true,
			want: RouteRequest{Mode: unifiedModeFixed, ClientID: 12, TTL: unifiedDefaultTTL}},
		{name: "fixed client 1", username: "1", wantOK: true,
			want: RouteRequest{Mode: unifiedModeFixed, ClientID: 1, TTL: unifiedDefaultTTL}},
		{name: "client 0 invalid", username: "0", wantOK: false},
		{name: "negative client invalid", username: "-5", wantOK: false},

		// --- tag random ----------------------------------------------------
		{name: "gz.auto is tag random", username: "gz.auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeRandom, Tags: []string{"gz"}, TTL: unifiedDefaultTTL}},
		{name: "jp.auto is tag random", username: "jp.auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeRandom, Tags: []string{"jp"}, TTL: unifiedDefaultTTL}},
		{name: "tag random must not carry a ttl", username: "gz.auto-10m", wantOK: false},
		{name: "dot requires the auto keyword", username: "gz.hk", wantOK: false},

		// --- sticky random -------------------------------------------------
		{name: "abc-auto sticky default ttl", username: "abc-auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", TTL: unifiedDefaultTTL}},
		{name: "abc-auto-10m", username: "abc-auto-10m", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", TTL: 10 * time.Minute}},
		{name: "abc-auto-30m", username: "abc-auto-30m", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", TTL: 30 * time.Minute}},
		{name: "crawler.jp-auto", username: "crawler.jp-auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "crawler", Tags: []string{"jp"}, TTL: unifiedDefaultTTL}},
		{name: "crawler.jp-auto-2h", username: "crawler.jp-auto-2h", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "crawler", Tags: []string{"jp"}, TTL: 2 * time.Hour}},
		{name: "user001.hk-auto-1d", username: "user001.hk-auto-1d", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "user001", Tags: []string{"hk"}, TTL: 24 * time.Hour}},
		{name: "abc.gz-auto", username: "abc.gz-auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", Tags: []string{"gz"}, TTL: unifiedDefaultTTL}},
		{name: "abc.gz-auto-30m", username: "abc.gz-auto-30m", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", Tags: []string{"gz"}, TTL: 30 * time.Minute}},

		// --- invalid -------------------------------------------------------
		{name: "empty", username: "", wantOK: false},
		{name: "blank", username: "   ", wantOK: false},
		{name: "plain word", username: "foo", wantOK: false},
		{name: "auto without key", username: "-auto", wantOK: false},
		{name: "empty ttl after marker", username: "abc-auto-", wantOK: false},
		{name: "auto with empty ttl", username: "auto-", wantOK: false},
		{name: "bare auto dashes", username: "-auto-", wantOK: false},
		{name: "dot without tag", username: ".auto", wantOK: false},
		{name: "empty sticky key before tag", username: "abc.-auto", wantOK: false},
		{name: "separator only key", username: "--auto", wantOK: false},
		{name: "separator only tag", username: "-.auto", wantOK: false},
		{name: "zero ttl", username: "abc-auto-0m", wantOK: false},
		{name: "ttl unit only", username: "abc-auto-m", wantOK: false},
		{name: "unknown unit", username: "abc-auto-5x", wantOK: false},
		{name: "uppercase unit", username: "abc-auto-30M", wantOK: false},
		{name: "trailing garbage", username: "abc-auto-30m-x", wantOK: false},
		{name: "ttl above 24h", username: "abc-auto-25h", wantOK: false},
		{name: "ttl 2d above 24h", username: "abc-auto-2d", wantOK: false},
		{name: "dot in tag", username: "abc.gz.hk-auto", wantOK: false},
		{name: "space in key", username: "a bc-auto", wantOK: false},
		{name: "uppercase tag", username: "abc.GZ-auto", wantOK: false},
		{name: "dash only key", username: "--auto", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRoute(tt.username)
			if !tt.wantOK {
				if err == nil {
					t.Fatalf("ParseRoute(%q) error = nil, want error (got %+v)", tt.username, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRoute(%q) unexpected error = %v", tt.username, err)
			}
			if got.Mode != tt.want.Mode {
				t.Fatalf("ParseRoute(%q).Mode = %v, want %v", tt.username, got.Mode, tt.want.Mode)
			}
			if got.ClientID != tt.want.ClientID {
				t.Fatalf("ParseRoute(%q).ClientID = %d, want %d", tt.username, got.ClientID, tt.want.ClientID)
			}
			if got.StickyKey != tt.want.StickyKey {
				t.Fatalf("ParseRoute(%q).StickyKey = %q, want %q", tt.username, got.StickyKey, tt.want.StickyKey)
			}
			if len(got.Tags) != len(tt.want.Tags) {
				t.Fatalf("ParseRoute(%q).Tags = %v, want %v", tt.username, got.Tags, tt.want.Tags)
			}
			for i := range tt.want.Tags {
				if got.Tags[i] != tt.want.Tags[i] {
					t.Fatalf("ParseRoute(%q).Tags = %v, want %v", tt.username, got.Tags, tt.want.Tags)
				}
			}
			if got.TTL != tt.want.TTL {
				t.Fatalf("ParseRoute(%q).TTL = %v, want %v", tt.username, got.TTL, tt.want.TTL)
			}
		})
	}
}

func TestParseUnifiedTTLBounds(t *testing.T) {
	if ttl, err := parseUnifiedTTL("1m"); err != nil || ttl != time.Minute {
		t.Fatalf("1m => %v, %v", ttl, err)
	}
	if ttl, err := parseUnifiedTTL("24h"); err != nil || ttl != 24*time.Hour {
		t.Fatalf("24h => %v, %v", ttl, err)
	}
	if ttl, err := parseUnifiedTTL("1d"); err != nil || ttl != 24*time.Hour {
		t.Fatalf("1d => %v, %v", ttl, err)
	}
	if _, err := parseUnifiedTTL("1441m"); err == nil {
		t.Fatalf("1441m must exceed the 24h ceiling")
	}
	if _, err := parseUnifiedTTL("0m"); err == nil {
		t.Fatalf("0m must be rejected")
	}
}

// ---------------------------------------------------------------------------
// ClientSelector / StickyStore
// ---------------------------------------------------------------------------

func newTestClient(id int, tags ...string) *file.Client {
	c := &file.Client{Id: id, Status: true, IsConnect: true, Flow: new(file.Flow), Cnf: new(file.Config)}
	c.Tags = tags
	return c
}

func selectorWith(store StickyStore) *ClientSelector {
	if store == nil {
		store = newMemoryStickyStore()
	}
	return NewClientSelector(store)
}

func TestSelectorTagPoolIsolation(t *testing.T) {
	gz := newTestClient(1, "gz", "telecom")
	jp := newTestClient(2, "jp")
	hk := newTestClient(3, "hk")
	online := []*file.Client{gz, jp, hk}

	sel := selectorWith(nil)
	for i := 0; i < 50; i++ {
		req, err := ParseRoute("gz.auto")
		if err != nil {
			t.Fatalf("ParseRoute: %v", err)
		}
		got, err := sel.SelectFrom(req, time.Minute, online)
		if err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}
		if got.Id != gz.Id {
			t.Fatalf("gz.auto picked client %d, want only gz (1)", got.Id)
		}
	}
}

func TestSelectorUnknownTagFails(t *testing.T) {
	online := []*file.Client{newTestClient(1, "gz"), newTestClient(2, "jp")}
	sel := selectorWith(nil)

	req, err := ParseRoute("kr.auto")
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	if _, err := sel.SelectFrom(req, time.Minute, online); err == nil {
		t.Fatalf("unknown tag must fail instead of falling back to every client")
	}
}

func TestSelectorFixedClientOfflineFails(t *testing.T) {
	online := []*file.Client{newTestClient(1), newTestClient(2)}
	sel := selectorWith(nil)

	req, _ := ParseRoute("12")
	if _, err := sel.SelectFrom(req, time.Minute, online); err == nil {
		t.Fatalf("pinned client 12 is offline, selection must fail")
	}

	req, _ = ParseRoute("2")
	got, err := sel.SelectFrom(req, time.Minute, online)
	if err != nil || got.Id != 2 {
		t.Fatalf("pinned client 2 => %v, %v", got, err)
	}
}

func TestSelectorNoOnlineClients(t *testing.T) {
	sel := selectorWith(nil)
	req, _ := ParseRoute("auto")
	if _, err := sel.SelectFrom(req, time.Minute, nil); err == nil {
		t.Fatalf("no online client must fail")
	}
}

func TestStickyReusesSameClientAndRespectsTTL(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	now := time.Now()
	store.now = func() time.Time { return now }

	online := []*file.Client{newTestClient(1), newTestClient(2), newTestClient(3)}
	req, _ := ParseRoute("abc-auto")
	req.CacheKey = CacheKeyFor(7, "abc-auto")

	first, err := sel.SelectFrom(req, 10*time.Minute, online)
	if err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, err := sel.SelectFrom(req, 10*time.Minute, online)
		if err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}
		if got.Id != first.Id {
			t.Fatalf("sticky connection %d picked %d, want %d", i, got.Id, first.Id)
		}
	}

	// TTL expiry => a new connection may re-roll.
	now = now.Add(11 * time.Minute)
	if _, ok := store.Get(req.CacheKey); ok {
		t.Fatalf("entry must be expired after the ttl")
	}
	if _, err := sel.SelectFrom(req, 10*time.Minute, online); err != nil {
		t.Fatalf("SelectFrom after expiry: %v", err)
	}
	if _, ok := store.Get(req.CacheKey); !ok {
		t.Fatalf("a fresh entry must be stored after expiry")
	}
}

func TestStickyTTLFromUsername(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	now := time.Now()
	store.now = func() time.Time { return now }

	online := []*file.Client{newTestClient(1)}
	req, err := ParseRoute("abc-auto-30m")
	if err != nil {
		t.Fatalf("ParseRoute: %v", err)
	}
	req.CacheKey = CacheKeyFor(1, "abc-auto-30m")
	if _, err := sel.SelectFrom(req, 10*time.Minute, online); err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}

	now = now.Add(29 * time.Minute)
	if _, ok := store.Get(req.CacheKey); !ok {
		t.Fatalf("entry must still be alive before 30m")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := store.Get(req.CacheKey); ok {
		t.Fatalf("entry must be gone after 30m")
	}
}

func TestStickyCacheIsolatedByUnifiedProxyID(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	online := []*file.Client{newTestClient(1), newTestClient(2)}

	reqA, _ := ParseRoute("abc-auto")
	reqA.CacheKey = CacheKeyFor(1, "abc-auto")
	reqB, _ := ParseRoute("abc-auto")
	reqB.CacheKey = CacheKeyFor(2, "abc-auto")

	if reqA.CacheKey == reqB.CacheKey {
		t.Fatalf("cache keys must differ across unified proxy instances")
	}
	if _, err := sel.SelectFrom(reqA, time.Minute, online); err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}
	if _, ok := store.Get(reqB.CacheKey); ok {
		t.Fatalf("a second unified proxy must not see the first one's sticky entry")
	}
}

func TestStickyInvalidatedWhenClientGoesOffline(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	online := []*file.Client{newTestClient(1), newTestClient(2), newTestClient(3)}

	req, _ := ParseRoute("abc-auto")
	req.CacheKey = CacheKeyFor(9, "abc-auto")
	first, err := sel.SelectFrom(req, time.Hour, online)
	if err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}

	// The cached client disappears from the online set.
	remaining := make([]*file.Client, 0, len(online))
	for _, c := range online {
		if c.Id != first.Id {
			remaining = append(remaining, c)
		}
	}
	got, err := sel.SelectFrom(req, time.Hour, remaining)
	if err != nil {
		t.Fatalf("SelectFrom after the client went offline: %v", err)
	}
	if got.Id == first.Id {
		t.Fatalf("an offline client must never be reused")
	}
	if id, ok := store.Get(req.CacheKey); !ok || id != got.Id {
		t.Fatalf("cache must be refreshed to %d, got %d ok=%v", got.Id, id, ok)
	}
}

func TestStickyInvalidatedWhenTagRemoved(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)

	gz1 := newTestClient(1, "gz")
	gz2 := newTestClient(2, "gz")
	online := []*file.Client{gz1, gz2}

	req, _ := ParseRoute("abc.gz-auto")
	req.CacheKey = CacheKeyFor(3, "abc.gz-auto")
	first, err := sel.SelectFrom(req, time.Hour, online)
	if err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}

	// The tag is removed from the client that owns the sticky entry.
	if err := first.SetTags(nil); err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	got, err := sel.SelectFrom(req, time.Hour, online)
	if err != nil {
		t.Fatalf("SelectFrom after tag removal: %v", err)
	}
	if got.Id == first.Id {
		t.Fatalf("a client that lost the tag must not be reused")
	}
	if id, ok := store.Get(req.CacheKey); !ok || id != got.Id {
		t.Fatalf("cache must be refreshed to %d, got %d ok=%v", got.Id, id, ok)
	}
}

func TestStickyFailsWhenEveryTaggedClientIsGone(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	online := []*file.Client{newTestClient(1, "jp")}

	req, _ := ParseRoute("abc.gz-auto")
	req.CacheKey = CacheKeyFor(4, "abc.gz-auto")
	if _, err := sel.SelectFrom(req, time.Hour, online); err == nil {
		t.Fatalf("no gz client must fail, never fall back to jp")
	}
}

func TestRandomModeIsNeverCached(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	online := []*file.Client{newTestClient(1), newTestClient(2)}

	req, _ := ParseRoute("auto")
	req.CacheKey = CacheKeyFor(5, "auto")
	for i := 0; i < 20; i++ {
		if _, err := sel.SelectFrom(req, time.Minute, online); err != nil {
			t.Fatalf("SelectFrom: %v", err)
		}
	}
	if store.Len() != 0 {
		t.Fatalf("random mode must not write the sticky cache, len=%d", store.Len())
	}

	tagReq, _ := ParseRoute("gz.auto")
	tagReq.CacheKey = CacheKeyFor(5, "gz.auto")
	if _, err := sel.SelectFrom(tagReq, time.Minute, []*file.Client{newTestClient(1, "gz")}); err != nil {
		t.Fatalf("SelectFrom: %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("tag random mode must not write the sticky cache, len=%d", store.Len())
	}
}

func TestStickyStoreConcurrentAccess(t *testing.T) {
	store := newMemoryStickyStore()
	sel := selectorWith(store)
	online := []*file.Client{newTestClient(1, "gz"), newTestClient(2, "gz"), newTestClient(3)}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, err := ParseRoute("abc.gz-auto-1m")
			if err != nil {
				t.Errorf("ParseRoute: %v", err)
				return
			}
			req.CacheKey = CacheKeyFor(i%4, "abc.gz-auto-1m")
			if _, err := sel.SelectFrom(req, time.Minute, online); err != nil {
				t.Errorf("SelectFrom: %v", err)
			}
		}(i)
	}
	wg.Wait()
}

func TestMemoryStickyStoreGetSetDelete(t *testing.T) {
	store := newMemoryStickyStore()
	if _, ok := store.Get("k"); ok {
		t.Fatalf("missing key must not hit")
	}
	store.Set("k", 5, time.Minute)
	if id, ok := store.Get("k"); !ok || id != 5 {
		t.Fatalf("Get = %d, %v", id, ok)
	}
	store.Delete("k")
	if _, ok := store.Get("k"); ok {
		t.Fatalf("deleted key must not hit")
	}
	store.Set("", 1, time.Minute)
	store.Set("bad", 0, time.Minute)
	if store.Len() != 0 {
		t.Fatalf("invalid writes must be ignored, len=%d", store.Len())
	}
}

func TestCleanUnifiedStickyCache(t *testing.T) {
	store := defaultUnifiedStickyStore()
	store.reset()
	defer store.reset()
	defer func() { store.now = time.Now }()

	now := time.Now()
	store.now = func() time.Time { return now }
	store.Set("alive", 1, time.Hour)
	store.Set("dead", 2, time.Minute)

	now = now.Add(2 * time.Minute)
	cleanUnifiedStickyCache()

	if _, ok := store.Get("alive"); !ok {
		t.Fatalf("a live entry must survive the cleaner")
	}
	if _, ok := store.Get("dead"); ok {
		t.Fatalf("an expired entry must be purged")
	}
}

// ---------------------------------------------------------------------------
// end to end: HTTP CONNECT and SOCKS5 must behave identically
// ---------------------------------------------------------------------------

// fakeBridge captures the client chosen by the unified proxy and hands back a
// live pipe so the data path can be exercised.
type fakeBridge struct {
	mu        sync.Mutex
	chosenIDs []int
	target    net.Conn
}

func (b *fakeBridge) IsServer() bool { return true }

func (b *fakeBridge) SendLinkInfo(clientId int, link *conn.Link, t *file.Tunnel) (net.Conn, error) {
	b.mu.Lock()
	b.chosenIDs = append(b.chosenIDs, clientId)
	b.mu.Unlock()
	return b.target, nil
}

func (b *fakeBridge) CliProcess(c *conn.Conn, tunnelType string) {}

func (b *fakeBridge) picked() []int {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]int, len(b.chosenIDs))
	copy(out, b.chosenIDs)
	return out
}

// waitPicked polls until at least n clients have been recorded by the bridge.
// The HTTP CONNECT path writes "200 Connection established" before the egress
// client is handed to DealClient, so the assertion must not race the pick.
func (b *fakeBridge) waitPicked(t *testing.T, n int) []int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		ids := b.picked()
		if len(ids) >= n {
			return ids
		}
		if time.Now().After(deadline) {
			t.Fatalf("bridge never picked %d client(s), got %v", n, ids)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newUnifiedE2EServer(t *testing.T, password string) (*TunnelModeServer, *fakeBridge) {
	t.Helper()

	db := file.GetDb()
	clients := []*file.Client{
		newTestClient(9001, "gz"),
		newTestClient(9002, "jp"),
	}
	for _, c := range clients {
		c.Cnf = &file.Config{Crypt: false, Compress: false}
		c.Rate = nil
		db.JsonDb.Clients.Store(c.Id, c)
	}
	t.Cleanup(func() {
		for _, c := range clients {
			db.JsonDb.Clients.Delete(c.Id)
		}
	})

	bridge := &fakeBridge{}
	task := &file.Tunnel{
		Id:        4242,
		Mode:      "unifiedProxy",
		Password:  password,
		CacheTime: 10,
		Flow:      new(file.Flow),
		Target:    &file.Target{},
	}
	s := NewTunnelModeServer(ProcessUnified, bridge, task, false)
	s.stickyStore = newMemoryStickyStore()
	return s, bridge
}

func TestProcessUnifiedHttpRequiresPassword(t *testing.T) {
	s, _ := newUnifiedE2EServer(t, "s3cret")

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("auto:wrong")) + "\r\n\r\n"
	if _, err := clientSide.Write([]byte(req)); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, len(common.ProxyAuthRequiredBytes))
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("read 407: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 407") {
		t.Fatalf("expected a 407 challenge, got %q", string(buf))
	}
	<-done
}

func TestProcessUnifiedHttpRejectsEmptyPasswordTask(t *testing.T) {
	s, _ := newUnifiedE2EServer(t, "")

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("auto:")) + "\r\n\r\n"
	if _, err := clientSide.Write([]byte(req)); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, len(common.ProxyAuthRequiredBytes))
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("read 407: %v", err)
	}
	<-done
}

func TestProcessUnifiedHttpConnectPicksTaggedClient(t *testing.T) {
	s, bridge := newUnifiedE2EServer(t, "s3cret")

	backendServer, backendClient := net.Pipe()
	defer func() { _ = backendClient.Close() }()
	bridge.target = backendServer

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnifiedHttp(conn.NewConn(serverSide), s)
	}()

	req := "CONNECT example.com:443 HTTP/1.1\r\nHost: example.com:443\r\nProxy-Authorization: Basic " +
		base64.StdEncoding.EncodeToString([]byte("gz.auto:s3cret")) + "\r\n\r\n"
	if _, err := clientSide.Write([]byte(req)); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 39)
	if _, err := io.ReadFull(clientSide, buf); err != nil {
		t.Fatalf("read 200: %v", err)
	}
	if !strings.HasPrefix(string(buf), "HTTP/1.1 200") {
		t.Fatalf("expected 200 Connection established, got %q", string(buf))
	}

	ids := bridge.waitPicked(t, 1)
	if ids[0] != 9001 {
		t.Fatalf("gz.auto must pick client 9001, picked %v", ids)
	}
}

func TestProcessUnifiedSocks5PicksTaggedClient(t *testing.T) {
	s, bridge := newUnifiedE2EServer(t, "s3cret")

	backendServer, backendClient := net.Pipe()
	defer func() { _ = backendClient.Close() }()
	bridge.target = backendServer

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnified(conn.NewConn(serverSide), s)
	}()

	// greeting: VER=5, NMETHODS=1, USERPASS
	if _, err := clientSide.Write([]byte{5, 1, 2}); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read method selection: %v", err)
	}
	if reply[0] != 5 || reply[1] != 2 {
		t.Fatalf("method selection = %v, want [5 2]", reply)
	}

	// username/password: gz.auto / s3cret
	auth := []byte{1, 7}
	auth = append(auth, []byte("gz.auto")...)
	auth = append(auth, 6)
	auth = append(auth, []byte("s3cret")...)
	if _, err := clientSide.Write(auth); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read auth reply: %v", err)
	}
	if reply[0] != 1 || reply[1] != 0 {
		t.Fatalf("auth reply = %v, want [1 0]", reply)
	}

	// CONNECT example.com:443 (domain address type)
	req := []byte{5, 1, 0, 3, 11}
	req = append(req, []byte("example.com")...)
	req = append(req, 0x01, 0xBB)
	if _, err := clientSide.Write(req); err != nil {
		t.Fatalf("write connect: %v", err)
	}

	head := make([]byte, 10)
	if _, err := io.ReadFull(clientSide, head); err != nil {
		t.Fatalf("read connect reply: %v", err)
	}
	if head[0] != 5 || head[1] != 0 {
		t.Fatalf("connect reply = %v, want rep 0", head)
	}

	ids := bridge.picked()
	if len(ids) != 1 || ids[0] != 9001 {
		t.Fatalf("gz.auto must pick client 9001, picked %v", ids)
	}
}

func TestProcessUnifiedSocks5RejectsEmptyPassword(t *testing.T) {
	s, _ := newUnifiedE2EServer(t, "")

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnified(conn.NewConn(serverSide), s)
	}()

	if _, err := clientSide.Write([]byte{5, 1, 2}); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read method selection: %v", err)
	}
	if reply[1] != 2 {
		t.Fatalf("userpass auth must be offered")
	}

	auth := []byte{1, 7}
	auth = append(auth, []byte("gz.auto")...)
	auth = append(auth, 1)
	auth = append(auth, []byte("x")...)
	if _, err := clientSide.Write(auth); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read auth reply: %v", err)
	}
	if reply[1] == 0 {
		t.Fatalf("an empty task password must never authenticate")
	}
}

func TestProcessUnifiedSocks5RejectsInvalidUsername(t *testing.T) {
	s, bridge := newUnifiedE2EServer(t, "s3cret")

	serverSide, clientSide := net.Pipe()
	defer func() { _ = clientSide.Close() }()

	go func() {
		_ = ProcessUnified(conn.NewConn(serverSide), s)
	}()

	if _, err := clientSide.Write([]byte{5, 1, 2}); err != nil {
		t.Fatalf("write greeting: %v", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read method selection: %v", err)
	}

	user := "kr.auto"
	auth := []byte{1, byte(len(user))}
	auth = append(auth, []byte(user)...)
	auth = append(auth, 6)
	auth = append(auth, []byte("s3cret")...)
	if _, err := clientSide.Write(auth); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if _, err := io.ReadFull(clientSide, reply); err != nil {
		t.Fatalf("read auth reply: %v", err)
	}
	if reply[1] != 0 {
		t.Fatalf("credentials are valid, auth must succeed before routing")
	}

	// CONNECT must fail because no client carries the "kr" tag.
	req := []byte{5, 1, 0, 3, 11}
	req = append(req, []byte("example.com")...)
	req = append(req, 0x01, 0xBB)
	_, _ = clientSide.Write(req)

	if ids := bridge.picked(); len(ids) != 0 {
		t.Fatalf("an unknown tag must not reach any client, picked %v", ids)
	}
}
