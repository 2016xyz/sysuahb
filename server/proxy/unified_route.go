package proxy

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// unifiedMode is the routing mode encoded in a unified proxy username.
type unifiedMode int

const (
	// unifiedModeRandom is "auto": pick a random available client for every new connection.
	unifiedModeRandom unifiedMode = iota
	// unifiedModeFixed is "<clientId>": pin to one client, never fall back.
	unifiedModeFixed
	// unifiedModeSticky is "[stickyKey.]tag-auto[-ttl]": sticky random selection.
	unifiedModeSticky
)

func (m unifiedMode) String() string {
	switch m {
	case unifiedModeRandom:
		return "random"
	case unifiedModeFixed:
		return "fixed"
	case unifiedModeSticky:
		return "sticky"
	default:
		return "unknown"
	}
}

// TTL bounds for the sticky username suffix.
const (
	unifiedMinTTL     = time.Minute
	unifiedMaxTTL     = 24 * time.Hour
	unifiedDefaultTTL = 10 * time.Minute
)

// RouteRequest is the single source of truth produced by RouteParser and
// consumed by both the HTTP and the SOCKS5 unified proxy entry points.
type RouteRequest struct {
	Mode      unifiedMode
	Raw       string
	ClientID  int
	StickyKey string
	Tags      []string
	TTL       time.Duration
	CacheKey  string
}

// Sticky reports whether the request is served from the sticky cache.
func (r RouteRequest) Sticky() bool {
	return r.Mode == unifiedModeSticky
}

var unifiedTTLRegexp = regexp.MustCompile(`^([1-9][0-9]*)([mhd])$`)

// unifiedTagRegexp mirrors file.IsValidTag so the parser stays self-contained.
var unifiedTagRegexp = regexp.MustCompile(`^[a-z0-9_-]+$`)

// unifiedAlnumRegexp guards against names made of separators only, e.g. "-",
// "__" or "--", which would otherwise be accepted as a sticky key or tag.
var unifiedAlnumRegexp = regexp.MustCompile(`[a-z0-9]`)

// isValidNameSegment reports whether s is a legal tag / sticky key segment:
// it must match [a-z0-9_-]+ and contain at least one alphanumeric character.
func isValidNameSegment(s string) bool {
	return unifiedTagRegexp.MatchString(s) && unifiedAlnumRegexp.MatchString(s)
}

// unifiedStickySuffixRegexp matches "<stickyKey>.auto" or "<stickyKey>.auto-<ttl>".
var unifiedStickySuffixRegexp = regexp.MustCompile(`^([a-z0-9_-]+)\.auto(?:-([0-9]+[mhd]))?$`)

// unifiedAutoSuffixRegexp matches "auto" or "auto-<ttl>" (no sticky key, no tag).
var unifiedAutoSuffixRegexp = regexp.MustCompile(`^auto(?:-([0-9]+[mhd]))?$`)

// ParseRoute parses a unified proxy username into a RouteRequest.
//
// Supported grammar (V1, exactly one tag):
//
//	auto                              random over all available clients, per connection
//	<clientId>                        pinned client, fails when it is not available
//	<tag>.auto                        random over clients tagged <tag>, per connection
//	<stickyKey>-auto                  sticky random over all available clients
//	<stickyKey>-auto-<ttl>            same with an explicit TTL
//	<stickyKey>.<tag>-auto            sticky random over clients tagged <tag>
//	<stickyKey>.<tag>-auto-<ttl>      same with an explicit TTL
//
// ttl = <number><unit>, unit in m (minute) / h (hour) / d (day), 1m .. 24h.
// Anything else is rejected, never guessed and never silently degraded.
func ParseRoute(username string) (RouteRequest, error) {
	raw := username
	username = strings.TrimSpace(username)
	if username == "" {
		return RouteRequest{}, errors.New("unified proxy: empty username")
	}

	// 1) "auto": fully random, no tag, no sticky.
	if username == "auto" {
		return RouteRequest{
			Mode: unifiedModeRandom,
			Raw:  raw,
			TTL:  unifiedDefaultTTL,
		}, nil
	}

	// 2) "<clientId>": pinned client. Anything non numeric is not a client id.
	if isAllDigits(username) {
		id, err := strconv.Atoi(username)
		if err != nil {
			return RouteRequest{}, fmt.Errorf("unified proxy: invalid client id %q", username)
		}
		if id <= 0 {
			return RouteRequest{}, fmt.Errorf("unified proxy: invalid client id %q", username)
		}
		return RouteRequest{
			Mode:     unifiedModeFixed,
			Raw:      raw,
			ClientID: id,
			TTL:      unifiedDefaultTTL,
		}, nil
	}

	// 3) "<tag>.auto[-ttl]": tag only, random per connection (NOT sticky).
	if m := unifiedStickySuffixRegexp.FindStringSubmatch(username); m != nil {
		tag := m[1]
		if !isValidNameSegment(tag) {
			return RouteRequest{}, fmt.Errorf("unified proxy: invalid tag %q", tag)
		}
		if m[2] != "" {
			return RouteRequest{}, fmt.Errorf("unified proxy: random tag route %q must not carry a ttl, use <stickyKey>.%s-auto-<ttl>", username, tag)
		}
		return RouteRequest{
			Mode: unifiedModeRandom,
			Raw:  raw,
			Tags: []string{tag},
			TTL:  unifiedDefaultTTL,
		}, nil
	}

	// 4) "<stickyKey>[-<tag>]-auto[-<ttl>]": sticky.
	return parseStickyRoute(raw, username)
}

// parseStickyRoute handles the sticky forms:
//
//	<stickyKey>-auto[-<ttl>]
//	<stickyKey>.<tag>-auto[-<ttl>]
func parseStickyRoute(raw, username string) (RouteRequest, error) {
	key, tag, ttlText, err := splitStickyUsername(username)
	if err != nil {
		return RouteRequest{}, err
	}
	ttl := unifiedDefaultTTL
	if ttlText != "" {
		if ttl, err = parseUnifiedTTL(ttlText); err != nil {
			return RouteRequest{}, err
		}
	}
	req := RouteRequest{
		Mode:      unifiedModeSticky,
		Raw:       raw,
		StickyKey: key,
		TTL:       ttl,
	}
	if tag != "" {
		req.Tags = []string{tag}
	}
	return req, nil
}

// splitStickyUsername splits "<stickyKey>[.<tag>]-auto[-<ttl>]" into its parts.
func splitStickyUsername(username string) (key, tag, ttlText string, err error) {
	body := username

	// strip the "-auto" marker, keep the optional ttl
	if !strings.HasSuffix(body, "-auto") {
		idx := strings.LastIndex(body, "-auto-")
		if idx < 0 {
			return "", "", "", fmt.Errorf("unified proxy: invalid username %q", username)
		}
		ttlText = body[idx+len("-auto-"):]
		body = body[:idx+len("-auto")]
	}
	body = strings.TrimSuffix(body, "-auto")

	if body == "" {
		return "", "", "", fmt.Errorf("unified proxy: invalid username %q: missing sticky key", username)
	}

	// "<stickyKey>.<tag>"
	if dot := strings.Index(body, "."); dot >= 0 {
		key = body[:dot]
		tag = body[dot+1:]
		if tag == "" || strings.Contains(tag, ".") {
			return "", "", "", fmt.Errorf("unified proxy: invalid username %q", username)
		}
	} else {
		key = body
	}

	if !isValidNameSegment(key) {
		return "", "", "", fmt.Errorf("unified proxy: invalid sticky key %q", key)
	}
	if tag != "" && !isValidNameSegment(tag) {
		return "", "", "", fmt.Errorf("unified proxy: invalid tag %q", tag)
	}
	if ttlText != "" {
		// ttlText was already validated by the regexp on the full username, but
		// guard against trailing garbage such as "abc-auto-30m-x".
		if !unifiedTTLRegexp.MatchString(ttlText) {
			return "", "", "", fmt.Errorf("unified proxy: invalid ttl %q", ttlText)
		}
	}
	return key, tag, ttlText, nil
}

// parseUnifiedTTL parses "<number><m|h|d>" into a duration, enforcing 1m..24h.
func parseUnifiedTTL(text string) (time.Duration, error) {
	m := unifiedTTLRegexp.FindStringSubmatch(text)
	if m == nil {
		return 0, fmt.Errorf("unified proxy: invalid ttl %q, expected <number>[m|h|d]", text)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("unified proxy: invalid ttl %q", text)
	}
	var unit time.Duration
	switch m[2] {
	case "m":
		unit = time.Minute
	case "h":
		unit = time.Hour
	case "d":
		unit = 24 * time.Hour
	default:
		return 0, fmt.Errorf("unified proxy: invalid ttl unit %q", m[2])
	}
	ttl := time.Duration(n) * unit
	if ttl < unifiedMinTTL {
		return 0, fmt.Errorf("unified proxy: ttl %q is below the minimum of 1m", text)
	}
	if ttl > unifiedMaxTTL {
		return 0, fmt.Errorf("unified proxy: ttl %q is above the maximum of 24h", text)
	}
	return ttl, nil
}

// ResolveTTL returns the effective TTL: the explicit username TTL wins,
// otherwise the unified proxy task default, otherwise 10m.
func (r RouteRequest) ResolveTTL(defaultTTL time.Duration) time.Duration {
	if r.TTL > 0 {
		return r.TTL
	}
	if defaultTTL > 0 {
		if defaultTTL < unifiedMinTTL {
			return unifiedMinTTL
		}
		if defaultTTL > unifiedMaxTTL {
			return unifiedMaxTTL
		}
		return defaultTTL
	}
	return unifiedDefaultTTL
}

// CacheKeyFor builds the sticky cache key. The unified proxy task id is part
// of the key so that two unified proxy instances never share entries.
func CacheKeyFor(unifiedProxyID int, username string) string {
	return strconv.Itoa(unifiedProxyID) + ":" + strings.TrimSpace(username)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
