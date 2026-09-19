package proxy

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/djylb/nps/lib/file"
)

// ErrNoClientForTag is returned when a tag filter leaves no usable client.
// The selector never falls back to "any client" in that case.
var ErrNoClientForTag = errors.New("unified proxy: no available client matches the requested tag")

// ClientSelector turns a RouteRequest into a concrete egress client.
//
// It owns the sticky store and enforces the invariant that a cached entry is
// only reused while it is still valid:
//
//	client exists AND client online AND client available AND still tagged
//
// Every violation invalidates the cache entry and triggers a fresh selection.
type ClientSelector struct {
	store StickyStore
}

func NewClientSelector(store StickyStore) *ClientSelector {
	if store == nil {
		store = defaultUnifiedStickyStore()
	}
	return &ClientSelector{store: store}
}

// defaultClientSelector is the process wide selector.
func defaultClientSelector() *ClientSelector {
	return &ClientSelector{store: defaultUnifiedStickyStore()}
}

// OnlineClients returns every client that is currently usable as an egress.
func (s *ClientSelector) OnlineClients() []*file.Client {
	list := make([]*file.Client, 0)
	file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
		client, ok := value.(*file.Client)
		if !ok || client == nil {
			return true
		}
		if client.Available() {
			list = append(list, client)
		}
		return true
	})
	sortClientsByID(list)
	return list
}

// Candidates filters the online clients by the requested tags.
// An empty tag list means "all online clients".
func Candidates(clients []*file.Client, tags []string) []*file.Client {
	if len(tags) == 0 {
		return clients
	}
	out := make([]*file.Client, 0, len(clients))
	for _, client := range clients {
		if clientMatchesTags(client, tags) {
			out = append(out, client)
		}
	}
	return out
}

// clientMatchesTags implements the V1 semantics: every requested tag must be
// present on the client (single tag in V1, the loop keeps V2 open).
func clientMatchesTags(client *file.Client, tags []string) bool {
	if client == nil {
		return false
	}
	for _, tag := range tags {
		if !client.HasTag(tag) {
			return false
		}
	}
	return true
}

// Select resolves the request into a client.
//
// defaultTTL is the unified proxy task cache time (DefaultCacheTTL).
func (s *ClientSelector) Select(req RouteRequest, defaultTTL time.Duration) (*file.Client, error) {
	return s.SelectFrom(req, defaultTTL, s.OnlineClients())
}

// SelectFrom is the testable core of Select: the online client set is injected.
func (s *ClientSelector) SelectFrom(req RouteRequest, defaultTTL time.Duration, online []*file.Client) (*file.Client, error) {
	if req.Mode == unifiedModeFixed {
		// A pinned client id never falls back to another client, no matter
		// what the cache says.
		s.invalidate(req.CacheKey)
		return findClientByID(online, req.ClientID)
	}

	candidates := Candidates(online, req.Tags)
	if len(candidates) == 0 {
		s.invalidate(req.CacheKey)
		if len(req.Tags) > 0 {
			return nil, fmt.Errorf("%w: %v", ErrNoClientForTag, req.Tags)
		}
		return nil, errors.New("unified proxy: no online client available")
	}

	if req.Mode == unifiedModeRandom {
		// Random mode is deliberately not cached: every new connection re-rolls.
		return candidates[rand.Intn(len(candidates))], nil
	}

	// Sticky mode: reuse the cached client only while it is still valid.
	if client := s.lookupSticky(req, candidates); client != nil {
		return client, nil
	}

	picked := candidates[rand.Intn(len(candidates))]
	s.store.Set(req.CacheKey, picked.Id, req.ResolveTTL(defaultTTL))
	return picked, nil
}

// lookupSticky returns the cached client when it is still in the candidate set.
// The candidate set already encodes "exists, online, available, tagged", so a
// miss here (offline client, removed tag, deleted client) simply re-rolls.
func (s *ClientSelector) lookupSticky(req RouteRequest, candidates []*file.Client) *file.Client {
	if req.CacheKey == "" {
		return nil
	}
	cachedID, ok := s.store.Get(req.CacheKey)
	if !ok {
		return nil
	}
	for _, client := range candidates {
		if client.Id == cachedID {
			return client
		}
	}
	// The cached client is gone / offline / untagged: drop it immediately.
	s.store.Delete(req.CacheKey)
	return nil
}

func (s *ClientSelector) invalidate(key string) {
	if key != "" {
		s.store.Delete(key)
	}
}

func findClientByID(clients []*file.Client, id int) (*file.Client, error) {
	for _, client := range clients {
		if client.Id == id {
			return client, nil
		}
	}
	return nil, fmt.Errorf("unified proxy: client %d is not online or not available", id)
}

func sortClientsByID(clients []*file.Client) {
	for i := 1; i < len(clients); i++ {
		for j := i; j > 0 && clients[j-1].Id > clients[j].Id; j-- {
			clients[j-1], clients[j] = clients[j], clients[j-1]
		}
	}
}
