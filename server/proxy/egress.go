package proxy

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"example.com/svcmgr/lib/file"
)

// ---------------------------------------------------------------------------
// EgressRef: which upstream serves a unified proxy connection.
// ---------------------------------------------------------------------------

// EgressType distinguishes the two egress flavours of the unified pool.
type EgressType int

const (
	// EgressTypeClient is an NPS client reached through the bridge/NPC.
	EgressTypeClient EgressType = 1
	// EgressTypeProxy is an external HTTP/SOCKS5 upstream proxy.
	EgressTypeProxy EgressType = 2
)

func (t EgressType) String() string {
	switch t {
	case EgressTypeClient:
		return "client"
	case EgressTypeProxy:
		return "proxy"
	default:
		return "unknown"
	}
}

// EgressRef is the value the sticky cache stores: it only references the
// egress, it never holds a pointer, so a stale object can never leak into a
// new connection.
type EgressRef struct {
	Type EgressType
	ID   int
}

// ClientRef builds a client egress reference.
func ClientRef(id int) EgressRef {
	return EgressRef{Type: EgressTypeClient, ID: id}
}

// ProxyRef builds an external proxy egress reference.
func ProxyRef(id int) EgressRef {
	return EgressRef{Type: EgressTypeProxy, ID: id}
}

// Valid reports whether the reference can be resolved at all.
func (r EgressRef) Valid() bool {
	return (r.Type == EgressTypeClient || r.Type == EgressTypeProxy) && r.ID > 0
}

func (r EgressRef) String() string {
	return fmt.Sprintf("%s:%d", r.Type, r.ID)
}

// ---------------------------------------------------------------------------
// Protocol capability: what the upstream must speak for this connection.
// ---------------------------------------------------------------------------

// egressProtocol is the protocol the current unified proxy connection needs.
// An NPS client supports both; an external proxy node has to be configured
// for the requested one.
type egressProtocol int

const (
	protoHttp egressProtocol = iota
	protoSocks5
)

func (p egressProtocol) String() string {
	if p == protoSocks5 {
		return "socks5"
	}
	return "http"
}

func (p egressProtocol) Supports(n *file.ProxyNode) bool {
	if n == nil {
		return false
	}
	// A tunnel node terminates the protocol itself, so it can carry both the
	// HTTP and the SOCKS5 entry of the unified proxy.
	if n.IsTunnel() {
		return true
	}
	if p == protoSocks5 {
		return n.SupportsSocks5()
	}
	return n.SupportsHttp()
}

// ---------------------------------------------------------------------------
// unifiedEgress: one selectable item of the unified pool.
// ---------------------------------------------------------------------------

// unifiedEgress is either a connected NPS client or a healthy external proxy.
// Exactly one of client/node is set; the other is nil.
type unifiedEgress struct {
	ref    EgressRef
	sortID int
	client *file.Client
	node   *file.ProxyNode
}

// Client returns the backing NPS client, or nil for an external proxy.
func (e unifiedEgress) Client() *file.Client { return e.client }

// Node returns the backing external proxy node, or nil for an NPS client.
func (e unifiedEgress) Node() *file.ProxyNode { return e.node }

// IsClient reports whether the egress is an NPS client.
func (e unifiedEgress) IsClient() bool { return e.ref.Type == EgressTypeClient }

// ---------------------------------------------------------------------------
// EgressSelector: the single selection pipeline used by HTTP and SOCKS5.
// ---------------------------------------------------------------------------

// ErrNoEgressForTag is returned when the tag filter leaves no usable egress.
// The selector never falls back to the whole pool in that case.
var ErrNoEgressForTag = errors.New("unified proxy: no available egress matches the requested tag")

// ErrNoEgress is returned when the pool itself is empty.
var ErrNoEgress = errors.New("unified proxy: no online client or proxy node available")

// EgressSelector turns a RouteRequest into a concrete EgressRef.
//
// It owns the sticky store and enforces the invariant that a cached entry is
// only reused while the egress is still part of the fresh candidate pool:
//
//	client/proxy exists AND enabled AND available AND still tagged AND
//	supports the protocol of this connection
//
// Every violation invalidates the cache entry and re-rolls immediately.
type EgressSelector struct {
	store StickyStore
}

func NewEgressSelector(store StickyStore) *EgressSelector {
	if store == nil {
		store = defaultUnifiedStickyStore()
	}
	return &EgressSelector{store: store}
}

// defaultEgressSelector is the process wide selector.
func defaultEgressSelector() *EgressSelector {
	return &EgressSelector{store: defaultUnifiedStickyStore()}
}

// PoolFromDb builds the current egress candidate pool from the live state:
// connected clients plus available (health-verified) proxy nodes.
func PoolFromDb() []unifiedEgress {
	out := make([]unifiedEgress, 0)
	file.GetDb().JsonDb.Clients.Range(func(key, value interface{}) bool {
		client, ok := value.(*file.Client)
		if !ok || client == nil || !client.Available() {
			return true
		}
		out = append(out, unifiedEgress{ref: ClientRef(client.Id), sortID: client.Id, client: client})
		return true
	})
	file.GetDb().JsonDb.Proxies.Range(func(key, value interface{}) bool {
		node, ok := value.(*file.ProxyNode)
		if !ok || node == nil || !node.Available() {
			return true
		}
		out = append(out, unifiedEgress{ref: ProxyRef(node.Id), sortID: node.Id, node: node})
		return true
	})
	sortEgressPool(out)
	return out
}

// PoolFromClientsAndProxies is the testable pool builder: it applies the exact
// same availability rules to an injected data set.
func PoolFromClientsAndProxies(clients []*file.Client, nodes []*file.ProxyNode) []unifiedEgress {
	out := make([]unifiedEgress, 0, len(clients)+len(nodes))
	for _, client := range clients {
		if client == nil || !client.Available() {
			continue
		}
		out = append(out, unifiedEgress{ref: ClientRef(client.Id), sortID: client.Id, client: client})
	}
	for _, node := range nodes {
		if node == nil || !node.Available() {
			continue
		}
		out = append(out, unifiedEgress{ref: ProxyRef(node.Id), sortID: node.Id, node: node})
	}
	sortEgressPool(out)
	return out
}

// Candidates filters the pool by liveness, protocol support and requested
// tags. An empty tag list means "every egress that speaks the protocol".
//
// Liveness is re-checked here even though PoolFromDb already filters: a stale
// injected pool (tests, or a node that died while the caller was building the
// list) must never leak an offline egress into a new connection.
func Candidates(pool []unifiedEgress, protocol egressProtocol, tags []string) []unifiedEgress {
	out := make([]unifiedEgress, 0, len(pool))
	for _, eg := range pool {
		if !egressLive(eg) {
			continue
		}
		if !egressSupportsProtocol(eg, protocol) {
			continue
		}
		if !egressMatchesTags(eg, tags) {
			continue
		}
		out = append(out, eg)
	}
	return out
}

// egressLive reports whether the egress backing object is still enabled,
// available and usable right now.
func egressLive(eg unifiedEgress) bool {
	if eg.IsClient() {
		return eg.client != nil && eg.client.Available()
	}
	return eg.node != nil && eg.node.Available()
}

// egressSupportsProtocol: NPS clients carry both protocols; an external node
// must be configured for the one requested by the current connection.
func egressSupportsProtocol(eg unifiedEgress, protocol egressProtocol) bool {
	if eg.IsClient() {
		return eg.client != nil
	}
	return protocol.Supports(eg.node)
}

// egressMatchesTags implements the V1 semantics: every requested tag must be
// present (single tag in V1, loop keeps V2 open).
func egressMatchesTags(eg unifiedEgress, tags []string) bool {
	if len(tags) == 0 {
		return true
	}
	for _, tag := range tags {
		if eg.IsClient() {
			if eg.client == nil || !eg.client.HasTag(tag) {
				return false
			}
			continue
		}
		if eg.node == nil || !eg.node.HasTag(tag) {
			return false
		}
	}
	return true
}

// Select resolves the request against the live pool.
func (s *EgressSelector) Select(req RouteRequest, defaultTTL time.Duration, protocol egressProtocol) (EgressRef, error) {
	return s.SelectFrom(req, defaultTTL, protocol, PoolFromDb())
}

// SelectFrom is the testable core of Select: the pool is injected.
func (s *EgressSelector) SelectFrom(req RouteRequest, defaultTTL time.Duration, protocol egressProtocol, pool []unifiedEgress) (EgressRef, error) {
	if req.Mode == unifiedModeFixed {
		// A pinned client id never falls back to another egress, no matter
		// what the cache says.
		s.invalidate(req.CacheKey)
		return findClientRef(pool, req.ClientID)
	}

	candidates := Candidates(pool, protocol, req.Tags)
	if len(candidates) == 0 {
		s.invalidate(req.CacheKey)
		if len(req.Tags) > 0 {
			return EgressRef{}, fmt.Errorf("%w: %v", ErrNoEgressForTag, req.Tags)
		}
		return EgressRef{}, ErrNoEgress
	}

	if req.Mode == unifiedModeRandom {
		// Random mode is deliberately not cached: every new connection re-rolls.
		return candidates[rand.Intn(len(candidates))].ref, nil
	}

	// Sticky mode: reuse the cached egress only while it is still valid.
	if ref, ok := s.lookupSticky(req, candidates); ok {
		return ref, nil
	}

	picked := candidates[rand.Intn(len(candidates))].ref
	s.store.Set(req.CacheKey, picked, req.ResolveTTL(defaultTTL))
	return picked, nil
}

// lookupSticky returns the cached ref when it is still in the candidate set.
// The candidate set already encodes "exists, enabled, available, tagged and
// protocol-capable", so a miss here (offline egress, removed tag, disabled or
// deleted node/proxy) simply re-rolls.
func (s *EgressSelector) lookupSticky(req RouteRequest, candidates []unifiedEgress) (EgressRef, bool) {
	if req.CacheKey == "" {
		return EgressRef{}, false
	}
	cached, ok := s.store.Get(req.CacheKey)
	if !ok {
		return EgressRef{}, false
	}
	for _, eg := range candidates {
		if eg.ref == cached {
			return cached, true
		}
	}
	// The cached egress is gone / offline / untagged / protocol-mismatched:
	// drop it immediately.
	s.store.Delete(req.CacheKey)
	return EgressRef{}, false
}

// Invalidate drops a sticky entry (used when a proxy node changes).
func (s *EgressSelector) Invalidate(key string) {
	s.invalidate(key)
}

func (s *EgressSelector) invalidate(key string) {
	if key != "" {
		s.store.Delete(key)
	}
}

func findClientRef(pool []unifiedEgress, id int) (EgressRef, error) {
	for _, eg := range pool {
		if eg.ref.Type == EgressTypeClient && eg.ref.ID == id {
			return eg.ref, nil
		}
	}
	return EgressRef{}, fmt.Errorf("unified proxy: client %d is not online or not available", id)
}

// sortEgressPool keeps the pool deterministic (clients and proxies by id).
func sortEgressPool(pool []unifiedEgress) {
	sort.Slice(pool, func(i, j int) bool {
		if pool[i].ref.Type != pool[j].ref.Type {
			return pool[i].ref.Type < pool[j].ref.Type
		}
		return pool[i].ref.ID < pool[j].ref.ID
	})
}

// ResolveEgress returns the live object behind a reference. It is used after
// selection to hand the connection to the right dial path.
func ResolveEgress(ref EgressRef) (unifiedEgress, error) {
	switch ref.Type {
	case EgressTypeClient:
		client, err := file.GetDb().GetClient(ref.ID)
		if err != nil {
			return unifiedEgress{}, err
		}
		if !client.Available() {
			return unifiedEgress{}, fmt.Errorf("unified proxy: client %d is not available", ref.ID)
		}
		return unifiedEgress{ref: ref, client: client}, nil
	case EgressTypeProxy:
		node, err := file.GetDb().GetProxyNode(ref.ID)
		if err != nil {
			return unifiedEgress{}, err
		}
		if !node.Available() {
			return unifiedEgress{}, fmt.Errorf("unified proxy: proxy node %d is not available", ref.ID)
		}
		return unifiedEgress{ref: ref, node: node}, nil
	default:
		return unifiedEgress{}, fmt.Errorf("unified proxy: unknown egress type %d", ref.Type)
	}
}
