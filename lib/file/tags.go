package file

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// tagPattern is the only allowed shape of a client tag.
// "." is explicitly rejected so that the unified proxy username
// "abc.gz-auto" stays unambiguous (sticky key / tag separator).
var tagPattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

// tagAlnumPattern guards against tags made of separators only, such as "-",
// "__" or "--": those match tagPattern but can never be addressed from a
// unified proxy username, so they are rejected instead of being stored.
var tagAlnumPattern = regexp.MustCompile(`[a-z0-9]`)

// ErrInvalidTag is returned when a tag does not match [a-z0-9_-]+.
var ErrInvalidTag = errors.New("invalid tag, only [a-z0-9_-]+ with at least one alphanumeric character is allowed")

// NormalizeTag trims and lowercases a single tag.
func NormalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}

// IsValidTag reports whether the (already normalized) tag is legal.
func IsValidTag(tag string) bool {
	return tagPattern.MatchString(tag) && tagAlnumPattern.MatchString(tag)
}

// NormalizeTags trims, lowercases, validates and de-duplicates tags.
// The order of first appearance is preserved. An empty input yields nil.
// Any illegal tag makes the whole call fail, no guessing / no fallback.
func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		tag := NormalizeTag(raw)
		if tag == "" {
			continue
		}
		if !IsValidTag(tag) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidTag, raw)
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// SplitTagsText parses a raw textarea / comma separated tag list.
// Both "\n", "," and ";" are accepted as separators.
func SplitTagsText(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	return fields
}

// HasTag reports whether the client currently owns the given tag.
// The comparison is done on the normalized form so that callers may pass
// either an already normalized tag or a raw one.
func (s *Client) HasTag(tag string) bool {
	if s == nil {
		return false
	}
	want := NormalizeTag(tag)
	if want == "" {
		return false
	}
	s.RLock()
	defer s.RUnlock()
	for _, t := range s.Tags {
		if NormalizeTag(t) == want {
			return true
		}
	}
	return false
}

// TagList returns a copy of the client tags (normalized, de-duplicated, sorted).
func (s *Client) TagList() []string {
	if s == nil {
		return nil
	}
	s.RLock()
	defer s.RUnlock()
	if len(s.Tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.Tags))
	seen := make(map[string]struct{}, len(s.Tags))
	for _, t := range s.Tags {
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

// SetTags replaces the client tags with the normalized form of tags.
func (s *Client) SetTags(tags []string) error {
	normalized, err := NormalizeTags(tags)
	if err != nil {
		return err
	}
	s.Lock()
	s.Tags = normalized
	s.Unlock()
	return nil
}

// Available reports whether the client can be used as an egress for the
// unified proxy: it must be connected and its status must allow connections.
func (s *Client) Available() bool {
	if s == nil {
		return false
	}
	s.RLock()
	defer s.RUnlock()
	return s.Id > 0 && s.Status && s.IsConnect
}
