package file

import (
	"sync"
	"testing"
	"time"
)

// A node that is switched off while a probe is in flight must stay Disabled.
// Before the fix, MarkCheckSuccess resurrected it (Status -> Available) and
// MarkCheckFailure stamped it Unavailable, so the list showed a healthy
// verdict for a node the operator had explicitly disabled.
func TestDisabledNodeKeepsDisabledStatus(t *testing.T) {
	node := NewProxyNode()
	node.Id = 7001
	node.Enabled = false
	node.MarkDisabled()

	// Assert the raw stored Status, not StatusValue(): StatusValue is derived
	// from Enabled and would mask a polluted Status field.
	node.MarkCheckSuccess(10*time.Millisecond, time.Now(), 1)
	status, _, _, _, failures, lastErr := node.HealthSnapshot()
	if status != ProxyStatusDisabled {
		t.Fatalf("after a successful probe the raw status is %d (%s), want Disabled",
			status, ProxyStatusName(status))
	}
	if failures != 0 || lastErr != "" {
		t.Fatalf("a probe for a disabled node must not record health data: failures=%d lastErr=%q", failures, lastErr)
	}

	node.MarkCheckFailure("boom", time.Now(), 1)
	if status, _, _, _, _, _ := node.HealthSnapshot(); status != ProxyStatusDisabled {
		t.Fatalf("after a failed probe the raw status is %d (%s), want Disabled",
			status, ProxyStatusName(status))
	}
	if got := node.StatusValue(); got != "Disabled" {
		t.Fatalf("effective status is %q, want Disabled", got)
	}
	if node.Available() {
		t.Fatal("a disabled node must never be Available")
	}
}

// Re-enabling a node must let probes decide the verdict again.
func TestEnabledNodeTransitionsNormally(t *testing.T) {
	node := NewProxyNode()
	node.Id = 7002
	node.Enabled = true

	node.MarkCheckFailure("fail", time.Now(), 3)
	if got := node.StatusValue(); got != "Unknown" {
		t.Fatalf("one failure below threshold reported %q, want Unknown", got)
	}
	node.MarkCheckFailure("fail", time.Now(), 3)
	node.MarkCheckFailure("fail", time.Now(), 3)
	if got := node.StatusValue(); got != "Unavailable" {
		t.Fatalf("three failures reported %q, want Unavailable", got)
	}
	if node.Available() {
		t.Fatal("an Unavailable node must not enter the pool")
	}
	node.MarkCheckSuccess(5*time.Millisecond, time.Now(), 1)
	if got := node.StatusValue(); got != "Available" {
		t.Fatalf("recovery reported %q, want Available", got)
	}
	if !node.Available() {
		t.Fatal("a recovered, enabled node must be Available")
	}
}

// concurrent AddTags must not lose tags. The old implementation read the tag
// slice, released the lock and wrote back, so two racing calls dropped one
// another's tag (classic lost update).
func TestConcurrentAddTagsKeepsEveryTag(t *testing.T) {
	node := NewProxyNode()
	node.Id = 7003
	node.Enabled = true

	const n = 50
	tags := make([]string, n)
	for i := range tags {
		tags[i] = string(rune('a'+i%26)) + "-" + itoa(i)
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(tag string) {
			defer wg.Done()
			if err := node.AddTags([]string{tag}); err != nil {
				t.Errorf("AddTags(%q): %v", tag, err)
			}
		}(tags[i])
	}
	wg.Wait()

	got := make(map[string]bool, n)
	for _, tag := range node.TagList() {
		got[tag] = true
	}
	if len(got) != n {
		t.Fatalf("lost tags: kept %d of %d (%v)", len(got), n, node.TagList())
	}
	for _, tag := range tags {
		if !got[NormalizeTag(tag)] {
			t.Fatalf("tag %q was dropped by a concurrent AddTags", tag)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
