package proxy

import (
	"sync"
	"time"
)

// StickyStore is the abstraction of the sticky routing cache.
//
// V1 uses an in-memory implementation guarded by a RWMutex; a Redis backed
// implementation can be plugged in later without touching the callers.
//
// The store only ever holds "CacheKey -> ClientID". Client pointers are never
// cached, so a stale pointer can never leak an offline client into a new
// connection.
type StickyStore interface {
	// Get returns the cached client id for key. The second result is false
	// when the entry is missing or already expired.
	Get(key string) (int, bool)
	// Set stores clientID under key with the given ttl. A non positive ttl
	// falls back to the default TTL.
	Set(key string, clientID int, ttl time.Duration)
	// Delete removes key from the store.
	Delete(key string)
	// Len returns the number of live entries.
	Len() int
	// Cleanup drops every expired entry.
	Cleanup()
}

type stickyEntry struct {
	clientID int
	expireAt time.Time
}

// memoryStickyStore is the V1 in-memory, concurrency safe StickyStore.
type memoryStickyStore struct {
	mu      sync.RWMutex
	entries map[string]stickyEntry
	// now is injectable so tests can drive expiry without sleeping.
	now func() time.Time
}

func newMemoryStickyStore() *memoryStickyStore {
	return &memoryStickyStore{
		entries: make(map[string]stickyEntry),
		now:     time.Now,
	}
}

func (s *memoryStickyStore) Get(key string) (int, bool) {
	if key == "" {
		return 0, false
	}
	s.mu.RLock()
	entry, ok := s.entries[key]
	now := s.now()
	s.mu.RUnlock()
	if !ok {
		return 0, false
	}
	if !now.Before(entry.expireAt) {
		s.Delete(key)
		return 0, false
	}
	return entry.clientID, true
}

func (s *memoryStickyStore) Set(key string, clientID int, ttl time.Duration) {
	if key == "" || clientID <= 0 {
		return
	}
	if ttl <= 0 {
		ttl = unifiedDefaultTTL
	}
	s.mu.Lock()
	s.entries[key] = stickyEntry{clientID: clientID, expireAt: s.now().Add(ttl)}
	s.mu.Unlock()
}

func (s *memoryStickyStore) Delete(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	delete(s.entries, key)
	s.mu.Unlock()
}

func (s *memoryStickyStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *memoryStickyStore) Cleanup() {
	now := s.now()
	s.mu.Lock()
	for key, entry := range s.entries {
		if !now.Before(entry.expireAt) {
			delete(s.entries, key)
		}
	}
	s.mu.Unlock()
}

// reset clears the store, it is only used by tests.
func (s *memoryStickyStore) reset() {
	s.mu.Lock()
	s.entries = make(map[string]stickyEntry)
	s.mu.Unlock()
}

// unifiedCacheCleanInterval is how often expired sticky entries are purged.
const unifiedCacheCleanInterval = time.Minute

var (
	defaultStickyStoreOnce sync.Once
	defaultStickyStore     *memoryStickyStore
	unifiedCleanerOnce     sync.Once
)

// defaultUnifiedStickyStore lazily creates the process wide sticky store.
func defaultUnifiedStickyStore() *memoryStickyStore {
	defaultStickyStoreOnce.Do(func() {
		defaultStickyStore = newMemoryStickyStore()
	})
	return defaultStickyStore
}

// startUnifiedCacheCleaner launches a one-off background goroutine that
// periodically purges expired sticky entries.
func startUnifiedCacheCleaner() {
	unifiedCleanerOnce.Do(func() {
		store := defaultUnifiedStickyStore()
		go func() {
			ticker := time.NewTicker(unifiedCacheCleanInterval)
			defer ticker.Stop()
			for range ticker.C {
				store.Cleanup()
			}
		}()
	})
}

// cleanUnifiedStickyCache purges expired entries from the default store.
func cleanUnifiedStickyCache() {
	defaultUnifiedStickyStore().Cleanup()
}
