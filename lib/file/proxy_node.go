package file

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Proxy node health status values.
// Disabled is derived from Enabled=false and never stored.
const (
	ProxyStatusUnknown     = 0
	ProxyStatusAvailable   = 1
	ProxyStatusUnavailable = 2
	ProxyStatusDisabled    = 3
)

// ProxyStatusName maps a status value to its API/UI name.
func ProxyStatusName(status int) string {
	switch status {
	case ProxyStatusUnavailable:
		return "Unavailable"
	case ProxyStatusDisabled:
		return "Disabled"
	case ProxyStatusAvailable:
		return "Available"
	default:
		return "Unknown"
	}
}

// ProxyNode is an external upstream proxy (HTTP and/or SOCKS5) that can be
// used as an egress of the unified proxy, next to the NPS clients.
//
// Only the configuration part of the struct is persisted; the health state is
// runtime-only (LastSuccessTime is the single exception, so that "not
// successful for X hours" filters survive a restart).
type ProxyNode struct {
	Id       int      `json:"Id"`
	Name     string   `json:"Name"`
	Host     string   `json:"Host"`
	Port     int      `json:"Port"`
	Username string   `json:"Username"`
	Password string   `json:"Password"`
	Http     bool     `json:"Http"`
	Socks5   bool     `json:"Socks5"`
	Tags     []string `json:"Tags"`
	Enabled  bool     `json:"Enabled"`

	// Tunnel protocol configuration. Scheme is empty for the original plain
	// HTTP/SOCKS5 nodes, which are described by the Http/Socks5 flags above.
	// For every other scheme the node is dialed with its own protocol and the
	// fields below carry what that protocol needs.
	Scheme     string `json:"Scheme,omitempty"`
	Method     string `json:"Method,omitempty"`  // ss/ssr cipher, vmess security
	Flow       string `json:"Flow,omitempty"`    // vless flow
	AlterId    int    `json:"AlterId,omitempty"` // vmess alter id
	TLS        bool   `json:"TLS,omitempty"`
	SNI        string `json:"SNI,omitempty"`
	SkipVerify bool   `json:"SkipVerify,omitempty"`
	Network    string `json:"Network,omitempty"` // tcp / ws / grpc / quic
	Path       string `json:"Path,omitempty"`
	HostHeader string `json:"HostHeader,omitempty"`
	ALPN       string `json:"ALPN,omitempty"`
	Link       string `json:"Link,omitempty"` // original share link, for re-editing

	// LastSuccessTime is persisted: it backs the "not successful for 24h/7d"
	// batch-delete filters across restarts.
	LastSuccessTime time.Time `json:"LastSuccessTime,omitempty"`

	// Runtime health state, never persisted.
	Status               int       `json:"-"`
	Latency              int64     `json:"-"`
	LastCheckTime        time.Time `json:"-"`
	ConsecutiveFailures  int       `json:"-"`
	ConsecutiveSuccesses int       `json:"-"`
	LastError            string    `json:"-"`

	sync.RWMutex `json:"-"`
}

// NewProxyNode returns a disabled-by-default node; callers fill the config.
func NewProxyNode() *ProxyNode {
	return &ProxyNode{
		Enabled: true,
		Status:  ProxyStatusUnknown,
	}
}

// Addr returns "host:port".
func (p *ProxyNode) Addr() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

// Protocols returns the configured protocol support list, e.g. "HTTP+SOCKS5"
// for a plain node, or the tunnel scheme label for a tunnel node.
func (p *ProxyNode) Protocols() string {
	p.RLock()
	defer p.RUnlock()
	if IsTunnelScheme(p.Scheme) {
		return SchemeLabel(p.Scheme)
	}
	switch {
	case p.Http && p.Socks5:
		return "HTTP+SOCKS5"
	case p.Http:
		return "HTTP"
	case p.Socks5:
		return "SOCKS5"
	default:
		return ""
	}
}

// SupportsHttp / SupportsSocks5 read the protocol flags under the lock.
func (p *ProxyNode) SupportsHttp() bool {
	p.RLock()
	defer p.RUnlock()
	return p.Http
}

func (p *ProxyNode) SupportsSocks5() bool {
	p.RLock()
	defer p.RUnlock()
	return p.Socks5
}

// IsTunnel reports whether the node uses a tunnel protocol instead of the
// plain HTTP/SOCKS5 flags.
func (p *ProxyNode) IsTunnel() bool {
	if p == nil {
		return false
	}
	p.RLock()
	defer p.RUnlock()
	return IsTunnelScheme(p.Scheme)
}

// SchemeName returns the node's scheme, "" for a plain node.
func (p *ProxyNode) SchemeName() string {
	if p == nil {
		return ""
	}
	p.RLock()
	defer p.RUnlock()
	return p.Scheme
}

// MethodName / PasswordValue / FlowName read the tunnel fields under the lock
// so the dialer never touches them while a config update holds the write lock.
func (p *ProxyNode) MethodName() string {
	p.RLock()
	defer p.RUnlock()
	return p.Method
}

func (p *ProxyNode) PasswordValue() string {
	p.RLock()
	defer p.RUnlock()
	return p.Password
}

func (p *ProxyNode) FlowName() string {
	p.RLock()
	defer p.RUnlock()
	return p.Flow
}

func (p *ProxyNode) AlterIdValue() int {
	p.RLock()
	defer p.RUnlock()
	return p.AlterId
}

func (p *ProxyNode) TLSEnabled() bool {
	p.RLock()
	defer p.RUnlock()
	return p.TLS
}

func (p *ProxyNode) SNIName() string {
	p.RLock()
	defer p.RUnlock()
	return p.SNI
}

func (p *ProxyNode) SkipVerifyEnabled() bool {
	p.RLock()
	defer p.RUnlock()
	return p.SkipVerify
}

// EnabledState reads Enabled under the lock.
func (p *ProxyNode) EnabledState() bool {
	p.RLock()
	defer p.RUnlock()
	return p.Enabled
}

// Available reports whether the node may enter the unified proxy egress pool:
// enabled by configuration AND healthy according to the last check.
func (p *ProxyNode) Available() bool {
	if p == nil {
		return false
	}
	p.RLock()
	defer p.RUnlock()
	return p.Id > 0 && p.Enabled && p.Status == ProxyStatusAvailable
}

// HasTag mirrors Client.HasTag so both egress kinds share the tag semantics.
func (p *ProxyNode) HasTag(tag string) bool {
	if p == nil {
		return false
	}
	want := NormalizeTag(tag)
	if want == "" {
		return false
	}
	p.RLock()
	defer p.RUnlock()
	for _, t := range p.Tags {
		if NormalizeTag(t) == want {
			return true
		}
	}
	return false
}

// TagList returns a normalized, de-duplicated, sorted copy of the tags.
func (p *ProxyNode) TagList() []string {
	if p == nil {
		return nil
	}
	p.RLock()
	defer p.RUnlock()
	if len(p.Tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.Tags))
	seen := make(map[string]struct{}, len(p.Tags))
	for _, t := range p.Tags {
		n := NormalizeTag(t)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// SetTags replaces the tags with their normalized form.
func (p *ProxyNode) SetTags(tags []string) error {
	normalized, err := NormalizeTags(tags)
	if err != nil {
		return err
	}
	p.Lock()
	p.Tags = normalized
	p.Unlock()
	return nil
}

// AddTags appends tags (normalized, de-duplicated) to the node.
func (p *ProxyNode) AddTags(tags []string) error {
	normalized, err := NormalizeTags(tags)
	if err != nil {
		return err
	}
	if len(normalized) == 0 {
		return nil
	}
	p.Lock()
	merged := append(append([]string{}, p.Tags...), normalized...)
	p.Unlock()
	return p.SetTags(merged)
}

// RemoveTags deletes tags (case-insensitive) from the node.
func (p *ProxyNode) RemoveTags(tags []string) {
	if p == nil || len(tags) == 0 {
		return
	}
	drop := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		if n := NormalizeTag(t); n != "" {
			drop[n] = struct{}{}
		}
	}
	p.Lock()
	defer p.Unlock()
	if len(p.Tags) == 0 {
		return
	}
	kept := make([]string, 0, len(p.Tags))
	for _, t := range p.Tags {
		if _, ok := drop[NormalizeTag(t)]; ok {
			continue
		}
		kept = append(kept, t)
	}
	p.Tags = kept
}

// HealthSnapshot returns the runtime health fields in one locked read.
func (p *ProxyNode) HealthSnapshot() (status int, latency int64, lastCheck time.Time,
	lastSuccess time.Time, failures int, lastError string) {
	p.RLock()
	defer p.RUnlock()
	return p.Status, p.Latency, p.LastCheckTime, p.LastSuccessTime, p.ConsecutiveFailures, p.LastError
}

// StatusValue computes the effective status name, taking Enabled into account.
func (p *ProxyNode) StatusValue() string {
	p.RLock()
	defer p.RUnlock()
	if !p.Enabled {
		return "Disabled"
	}
	return ProxyStatusName(p.Status)
}

// MarkCheckSuccess records a successful health check.
//
// recoverThreshold is the number of consecutive successes required to leave a
// failing state ("Unavailable"); the default is 1, i.e. one success recovers.
// A node in Unknown (never checked) always becomes Available on first success.
func (p *ProxyNode) MarkCheckSuccess(latency time.Duration, now time.Time, recoverThreshold int) {
	if recoverThreshold <= 0 {
		recoverThreshold = 1
	}
	p.Lock()
	p.ConsecutiveFailures = 0
	p.ConsecutiveSuccesses++
	p.LastCheckTime = now
	p.LastSuccessTime = now
	p.Latency = latency.Milliseconds()
	p.LastError = ""
	if p.Status != ProxyStatusAvailable && p.ConsecutiveSuccesses >= recoverThreshold {
		p.Status = ProxyStatusAvailable
	}
	p.Unlock()
}

// MarkCheckFailure records a failed health check and flips the node to
// Unavailable once the given failure threshold is reached.
func (p *ProxyNode) MarkCheckFailure(msg string, now time.Time, threshold int) {
	if threshold <= 0 {
		threshold = 1
	}
	p.Lock()
	p.LastCheckTime = now
	p.ConsecutiveFailures++
	p.ConsecutiveSuccesses = 0
	p.LastError = msg
	if p.ConsecutiveFailures >= threshold {
		p.Status = ProxyStatusUnavailable
	}
	p.Unlock()
}

// MarkDisabled forces the Disabled state (used when Enabled is turned off).
func (p *ProxyNode) MarkDisabled() {
	p.Lock()
	p.Status = ProxyStatusDisabled
	p.Latency = 0
	p.LastError = ""
	p.ConsecutiveFailures = 0
	p.ConsecutiveSuccesses = 0
	p.Unlock()
}

// ResetStatusToUnknown clears the health verdict so the next check decides
// again (used when the node is re-enabled or freshly loaded).
func (p *ProxyNode) ResetStatusToUnknown() {
	p.Lock()
	p.Status = ProxyStatusUnknown
	p.Latency = 0
	p.LastError = ""
	p.ConsecutiveFailures = 0
	p.ConsecutiveSuccesses = 0
	p.Unlock()
}

// ProxyConfig is the caller-editable subset of ProxyNode used by the web
// form. Applying it goes through ProxyNode.UpdateConfig so every write to a
// shared node is lock-guarded.
type ProxyConfig struct {
	Name     string
	Host     string
	Port     int
	Username string
	Password string
	Http     bool
	Socks5   bool
	Tags     []string
	Enabled  bool

	Scheme     string
	Method     string
	Flow       string
	AlterId    int
	TLS        bool
	SNI        string
	SkipVerify bool
	Network    string
	Path       string
	HostHeader string
	ALPN       string
	Link       string
}

// UpdateConfig replaces the configuration fields under the write lock. The
// runtime health state (Status/Latency/...) is left untouched except for the
// enabled/disabled transition, which must reset the health verdict.
func (p *ProxyNode) UpdateConfig(c ProxyConfig) {
	p.Lock()
	p.Name = c.Name
	p.Host = c.Host
	p.Port = c.Port
	p.Username = c.Username
	if c.Password != "" {
		p.Password = c.Password
	}
	p.Http = c.Http
	p.Socks5 = c.Socks5
	p.Tags = c.Tags
	p.Scheme = c.Scheme
	p.Method = c.Method
	p.Flow = c.Flow
	p.AlterId = c.AlterId
	p.TLS = c.TLS
	p.SNI = c.SNI
	p.SkipVerify = c.SkipVerify
	p.Network = c.Network
	p.Path = c.Path
	p.HostHeader = c.HostHeader
	p.ALPN = c.ALPN
	if c.Link != "" {
		p.Link = c.Link
	}
	enabledChanged := p.Enabled != c.Enabled
	p.Enabled = c.Enabled
	p.Unlock()
	if enabledChanged {
		if c.Enabled {
			p.ResetStatusToUnknown()
		} else {
			p.MarkDisabled()
		}
	}
}

// String implements a log-safe representation: the password never appears.
func (p *ProxyNode) String() string {
	if p == nil {
		return "proxy<nil>"
	}
	p.RLock()
	defer p.RUnlock()
	return fmt.Sprintf("proxy#%d[%s %s %s]", p.Id, p.Name, p.Addr(), p.ProtocolsLocked())
}

// ProtocolsLocked is Protocols for callers that already hold the lock.
func (p *ProxyNode) ProtocolsLocked() string {
	if IsTunnelScheme(p.Scheme) {
		return SchemeLabel(p.Scheme)
	}
	switch {
	case p.Http && p.Socks5:
		return "HTTP+SOCKS5"
	case p.Http:
		return "HTTP"
	case p.Socks5:
		return "SOCKS5"
	default:
		return ""
	}
}

// ProxyNodeView is the API/UI projection of a ProxyNode. The password is
// replaced by HasPassword so it can never leak through the list API.
type ProxyNodeView struct {
	Id              int      `json:"Id"`
	Name            string   `json:"Name"`
	Host            string   `json:"Host"`
	Port            int      `json:"Port"`
	Username        string   `json:"Username"`
	HasPassword     bool     `json:"HasPassword"`
	Http            bool     `json:"Http"`
	Socks5          bool     `json:"Socks5"`
	Protocols       string   `json:"Protocols"`
	Scheme          string   `json:"Scheme"`
	Method          string   `json:"Method"`
	Tags            []string `json:"Tags"`
	Enabled         bool     `json:"Enabled"`
	Status          string   `json:"Status"`
	Latency         int64    `json:"Latency"`
	LastCheckTime   int64    `json:"LastCheckTime"`
	LastSuccessTime int64    `json:"LastSuccessTime"`
	Failures        int      `json:"Failures"`
	LastError       string   `json:"LastError"`
}

// View projects the node into its password-free API form.
func (p *ProxyNode) View() ProxyNodeView {
	p.RLock()
	defer p.RUnlock()
	status := ProxyStatusName(p.Status)
	if !p.Enabled {
		status = "Disabled"
	}
	var lastCheck, lastSuccess int64
	if !p.LastCheckTime.IsZero() {
		lastCheck = p.LastCheckTime.UnixMilli()
	}
	if !p.LastSuccessTime.IsZero() {
		lastSuccess = p.LastSuccessTime.UnixMilli()
	}
	return ProxyNodeView{
		Id:              p.Id,
		Name:            p.Name,
		Host:            p.Host,
		Port:            p.Port,
		Username:        p.Username,
		HasPassword:     p.Password != "",
		Http:            p.Http,
		Socks5:          p.Socks5,
		Protocols:       p.ProtocolsLocked(),
		Scheme:          p.Scheme,
		Method:          p.Method,
		Tags:            p.TagListLocked(),
		Enabled:         p.Enabled,
		Status:          status,
		Latency:         p.Latency,
		LastCheckTime:   lastCheck,
		LastSuccessTime: lastSuccess,
		Failures:        p.ConsecutiveFailures,
		LastError:       p.LastError,
	}
}

// TagListLocked is TagList for callers that already hold the lock.
func (p *ProxyNode) TagListLocked() []string {
	if len(p.Tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(p.Tags))
	seen := make(map[string]struct{}, len(p.Tags))
	for _, t := range p.Tags {
		n := NormalizeTag(t)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// DuplicateKey builds the import de-duplication key:
// protocol + host + port + username (per-protocol, so an HTTP node and a
// SOCKS5 node on the same host:port stay distinct).
func (p *ProxyNode) DuplicateKey(protocol string) string {
	p.RLock()
	defer p.RUnlock()
	key := protocol
	if IsTunnelScheme(p.Scheme) {
		key = p.Scheme
	}
	return duplicateKey(key, p.Host, p.Port, p.Username)
}

func duplicateKey(protocol, host string, port int, username string) string {
	return fmt.Sprintf("%s|%s|%d|%s", strings.ToLower(strings.TrimSpace(protocol)),
		strings.ToLower(strings.TrimSpace(host)), port, strings.TrimSpace(username))
}
