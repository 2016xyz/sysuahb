package proxy

import (
	"sync"
	"testing"
	"time"
)

func TestMemoryStickyStoreBasics(t *testing.T) {
	s := newMemoryStickyStore()
	s.Set("k1", ClientRef(7), time.Minute)
	ref, ok := s.Get("k1")
	if !ok || ref != ClientRef(7) {
		t.Fatalf("Get = %v ok=%v", ref, ok)
	}
	s.Delete("k1")
	if _, ok := s.Get("k1"); ok {
		t.Fatalf("Delete must remove the entry")
	}
	if s.Len() != 0 {
		t.Fatalf("Len = %d", s.Len())
	}
}

func TestMemoryStickyStoreTTLExpiry(t *testing.T) {
	s := newMemoryStickyStore()
	base := time.Now()
	s.now = func() time.Time { return base }
	s.Set("k", ProxyRef(3), time.Minute)

	s.now = func() time.Time { return base.Add(59 * time.Second) }
	if _, ok := s.Get("k"); !ok {
		t.Fatalf("entry must survive until the TTL")
	}
	s.now = func() time.Time { return base.Add(61 * time.Second) }
	if _, ok := s.Get("k"); ok {
		t.Fatalf("entry must expire after the TTL")
	}
}

func TestMemoryStickyStoreConcurrent(t *testing.T) {
	s := newMemoryStickyStore()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := "k" + itoa(n%4)
			s.Set(key, ClientRef(n), time.Minute)
			_, _ = s.Get(key)
			if n%8 == 0 {
				s.Delete(key)
			}
		}(i)
	}
	wg.Wait()
	s.Cleanup()
}
