package proxy

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/djylb/nps/lib/file"
)

func TestParseUnifiedUsername(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		wantRoute unifiedRoute
		wantOK    bool
	}{
		{name: "auto means random", username: "auto", wantRoute: unifiedRoute{mode: unifiedModeRandom}, wantOK: true},
		{name: "pure digits means fixed client", username: "3", wantRoute: unifiedRoute{mode: unifiedModeFixed, clientId: 3}, wantOK: true},
		{name: "zero parses as fixed client zero", username: "0", wantRoute: unifiedRoute{mode: unifiedModeFixed, clientId: 0}, wantOK: true},
		{name: "prefix auto means sticky default ttl", username: "abc-auto", wantRoute: unifiedRoute{mode: unifiedModeSticky}, wantOK: true},
		{name: "minutes ttl", username: "abc-auto-30m", wantRoute: unifiedRoute{mode: unifiedModeSticky, ttlMinutes: 30}, wantOK: true},
		{name: "hours ttl", username: "abc-auto-2h", wantRoute: unifiedRoute{mode: unifiedModeSticky, ttlMinutes: 120}, wantOK: true},
		{name: "days ttl", username: "abc-auto-1d", wantRoute: unifiedRoute{mode: unifiedModeSticky, ttlMinutes: 1440}, wantOK: true},
		{name: "multi segment prefix", username: "user1-auto-5m", wantRoute: unifiedRoute{mode: unifiedModeSticky, ttlMinutes: 5}, wantOK: true},
		{name: "longer prefix with auto", username: "a-b-auto", wantRoute: unifiedRoute{mode: unifiedModeSticky}, wantOK: true},
		{name: "digit prefix with auto is sticky", username: "1-auto", wantRoute: unifiedRoute{mode: unifiedModeSticky}, wantOK: true},
		{name: "auto suffix after auto", username: "auto-auto", wantRoute: unifiedRoute{mode: unifiedModeSticky}, wantOK: true},
		{name: "empty segment before auto", username: "-auto", wantRoute: unifiedRoute{mode: unifiedModeSticky}, wantOK: true},
		{name: "ttl upper boundary accepted", username: "abc-auto-5256000m", wantRoute: unifiedRoute{mode: unifiedModeSticky, ttlMinutes: 5256000}, wantOK: true},
		{name: "ttl without prefix segment invalid", username: "auto-30m", wantOK: false},
		{name: "zero ttl invalid", username: "abc-auto-0m", wantOK: false},
		{name: "ttl above limit invalid", username: "abc-auto-5256001m", wantOK: false},
		{name: "unit without number invalid", username: "abc-auto-m", wantOK: false},
		{name: "unknown unit invalid", username: "abc-auto-5x", wantOK: false},
		{name: "uppercase unit invalid", username: "abc-auto-30M", wantOK: false},
		{name: "unknown suffix invalid", username: "abc-auto-x", wantOK: false},
		{name: "plain word invalid", username: "foo", wantOK: false},
		{name: "empty username invalid", username: "", wantOK: false},
		{name: "uppercase auto invalid", username: "AUTO", wantOK: false},
		{name: "negative number parses as unreachable fixed client", username: "-5", wantRoute: unifiedRoute{mode: unifiedModeFixed, clientId: -5}, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, ok := parseUnifiedUsername(tt.username)
			if ok != tt.wantOK {
				t.Fatalf("parseUnifiedUsername(%q) ok = %v, want %v", tt.username, ok, tt.wantOK)
			}
			if route != tt.wantRoute {
				t.Fatalf("parseUnifiedUsername(%q) = %+v, want %+v", tt.username, route, tt.wantRoute)
			}
		})
	}
}

func newUnifiedTestServer(taskId, cacheTime int) *TunnelModeServer {
	return &TunnelModeServer{BaseServer: &BaseServer{Task: &file.Tunnel{Id: taskId, CacheTime: cacheTime}}}
}

func unifiedTestKey(taskId int, username string) string {
	return fmt.Sprintf("%d:%s", taskId, username)
}

func TestCleanUnifiedStickyCache(t *testing.T) {
	expiredKey := "999:clean-expired"
	validKey := "999:clean-valid"
	unifiedStickyCache.Store(expiredKey, unifiedCacheEntry{clientId: 1, expireAt: time.Now().Add(-time.Second)})
	unifiedStickyCache.Store(validKey, unifiedCacheEntry{clientId: 2, expireAt: time.Now().Add(time.Hour)})
	defer func() {
		unifiedStickyCache.Delete(expiredKey)
		unifiedStickyCache.Delete(validKey)
	}()

	cleanUnifiedStickyCache()

	if _, ok := unifiedStickyCache.Load(expiredKey); ok {
		t.Fatalf("expired entry %q should have been removed", expiredKey)
	}
	if _, ok := unifiedStickyCache.Load(validKey); !ok {
		t.Fatalf("valid entry %q should have been kept", validKey)
	}
}

func TestPickUnifiedClientRandomMode(t *testing.T) {
	s := newUnifiedTestServer(1, 5)
	c1 := &file.Client{Id: 1}
	c2 := &file.Client{Id: 2}
	candidates := []*file.Client{c1, c2}

	got, err := s.pickUnifiedClientFrom("auto", candidates)
	if err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if got != c1 && got != c2 {
		t.Fatalf("picked client Id = %d, want one of candidates", got.Id)
	}
	if _, ok := unifiedStickyCache.Load(unifiedTestKey(1, "auto")); ok {
		t.Fatalf("random mode should not write sticky cache")
	}
}

func TestPickUnifiedClientFixedMode(t *testing.T) {
	s := newUnifiedTestServer(2, 5)
	c1 := &file.Client{Id: 1}
	c2 := &file.Client{Id: 2}
	candidates := []*file.Client{c1, c2}

	got, err := s.pickUnifiedClientFrom("2", candidates)
	if err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if got != c2 {
		t.Fatalf("picked client Id = %d, want fixed client 2", got.Id)
	}

	if _, err := s.pickUnifiedClientFrom("9", candidates); err == nil {
		t.Fatalf("pickUnifiedClientFrom() for offline client should fail")
	} else if !strings.Contains(err.Error(), "not online") {
		t.Fatalf("error = %v, want not online", err)
	}
}

func TestPickUnifiedClientErrors(t *testing.T) {
	s := newUnifiedTestServer(3, 5)
	candidates := []*file.Client{{Id: 1}}

	if _, err := s.pickUnifiedClientFrom("foo", candidates); err == nil || !strings.Contains(err.Error(), "invalid username") {
		t.Fatalf("error = %v, want invalid username", err)
	}
	if _, err := s.pickUnifiedClientFrom("auto", nil); err == nil || !strings.Contains(err.Error(), "no online clients") {
		t.Fatalf("error = %v, want no online clients", err)
	}
}

func TestPickUnifiedClientStickyStoresAndReuses(t *testing.T) {
	s := newUnifiedTestServer(1, 5)
	c1 := &file.Client{Id: 1}
	c2 := &file.Client{Id: 2}
	candidates := []*file.Client{c1, c2}
	key := unifiedTestKey(1, "u-auto")
	unifiedStickyCache.Delete(key)
	defer unifiedStickyCache.Delete(key)

	first, err := s.pickUnifiedClientFrom("u-auto", candidates)
	if err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if first != c1 && first != c2 {
		t.Fatalf("picked client Id = %d, want one of candidates", first.Id)
	}

	v, ok := unifiedStickyCache.Load(key)
	if !ok {
		t.Fatalf("sticky entry for %q not stored", key)
	}
	e := v.(unifiedCacheEntry)
	if e.clientId != first.Id {
		t.Fatalf("cached clientId = %d, want %d", e.clientId, first.Id)
	}
	if e.expireAt.Before(time.Now().Add(4*time.Minute)) || e.expireAt.After(time.Now().Add(6*time.Minute)) {
		t.Fatalf("expireAt = %v, want around now+5m", e.expireAt)
	}

	second, err := s.pickUnifiedClientFrom("u-auto", candidates)
	if err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if second != first {
		t.Fatalf("second pick = client %d, want cached client %d", second.Id, first.Id)
	}
}

func TestPickUnifiedClientStickyUsernameTTLWins(t *testing.T) {
	s := newUnifiedTestServer(2, 5)
	candidates := []*file.Client{{Id: 1}}
	key := unifiedTestKey(2, "u2-auto-30m")
	unifiedStickyCache.Delete(key)
	defer unifiedStickyCache.Delete(key)

	if _, err := s.pickUnifiedClientFrom("u2-auto-30m", candidates); err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}

	v, ok := unifiedStickyCache.Load(key)
	if !ok {
		t.Fatalf("sticky entry for %q not stored", key)
	}
	e := v.(unifiedCacheEntry)
	if e.expireAt.Before(time.Now().Add(29*time.Minute)) || e.expireAt.After(time.Now().Add(31*time.Minute)) {
		t.Fatalf("expireAt = %v, want around now+30m", e.expireAt)
	}
}

func TestPickUnifiedClientStickyDefaultTTLOfTenMinutes(t *testing.T) {
	s := newUnifiedTestServer(3, 0)
	candidates := []*file.Client{{Id: 1}}
	key := unifiedTestKey(3, "u3-auto")
	unifiedStickyCache.Delete(key)
	defer unifiedStickyCache.Delete(key)

	if _, err := s.pickUnifiedClientFrom("u3-auto", candidates); err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}

	v, ok := unifiedStickyCache.Load(key)
	if !ok {
		t.Fatalf("sticky entry for %q not stored", key)
	}
	e := v.(unifiedCacheEntry)
	if e.expireAt.Before(time.Now().Add(9*time.Minute)) || e.expireAt.After(time.Now().Add(11*time.Minute)) {
		t.Fatalf("expireAt = %v, want around now+10m", e.expireAt)
	}
}

func TestPickUnifiedClientStickyCachedClientOfflineRepicks(t *testing.T) {
	s := newUnifiedTestServer(4, 5)
	online := &file.Client{Id: 2}
	key := unifiedTestKey(4, "u4-auto")
	unifiedStickyCache.Store(key, unifiedCacheEntry{clientId: 99, expireAt: time.Now().Add(time.Hour)})
	defer unifiedStickyCache.Delete(key)

	got, err := s.pickUnifiedClientFrom("u4-auto", []*file.Client{online})
	if err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if got != online {
		t.Fatalf("picked client Id = %d, want repicked online client 2", got.Id)
	}

	v, ok := unifiedStickyCache.Load(key)
	if !ok {
		t.Fatalf("sticky entry for %q not refreshed", key)
	}
	e := v.(unifiedCacheEntry)
	if e.clientId != 2 {
		t.Fatalf("cached clientId = %d, want 2", e.clientId)
	}
}

func TestPickUnifiedClientStickyExpiredEntryRefreshed(t *testing.T) {
	s := newUnifiedTestServer(5, 5)
	c1 := &file.Client{Id: 1}
	key := unifiedTestKey(5, "u5-auto")
	unifiedStickyCache.Store(key, unifiedCacheEntry{clientId: 1, expireAt: time.Now().Add(-time.Minute)})
	defer unifiedStickyCache.Delete(key)

	got, err := s.pickUnifiedClientFrom("u5-auto", []*file.Client{c1})
	if err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if got != c1 {
		t.Fatalf("picked client Id = %d, want 1", got.Id)
	}

	v, ok := unifiedStickyCache.Load(key)
	if !ok {
		t.Fatalf("sticky entry for %q not stored after expiry", key)
	}
	e := v.(unifiedCacheEntry)
	if !e.expireAt.After(time.Now()) {
		t.Fatalf("expireAt = %v, want refreshed to the future", e.expireAt)
	}
}

func TestPickUnifiedClientStickyIsolatedByTaskId(t *testing.T) {
	s1 := newUnifiedTestServer(10, 5)
	s2 := newUnifiedTestServer(20, 5)
	candidates := []*file.Client{{Id: 1}}
	key1 := unifiedTestKey(10, "u6-auto")
	key2 := unifiedTestKey(20, "u6-auto")
	unifiedStickyCache.Delete(key1)
	unifiedStickyCache.Delete(key2)
	defer func() {
		unifiedStickyCache.Delete(key1)
		unifiedStickyCache.Delete(key2)
	}()

	if _, err := s1.pickUnifiedClientFrom("u6-auto", candidates); err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}
	if _, err := s2.pickUnifiedClientFrom("u6-auto", candidates); err != nil {
		t.Fatalf("pickUnifiedClientFrom() unexpected error = %v", err)
	}

	if _, ok := unifiedStickyCache.Load(key1); !ok {
		t.Fatalf("sticky entry for %q not stored", key1)
	}
	if _, ok := unifiedStickyCache.Load(key2); !ok {
		t.Fatalf("sticky entry for %q not stored", key2)
	}
}
