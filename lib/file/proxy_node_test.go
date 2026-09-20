package file

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestProxyNodeStateMachine(t *testing.T) {
	p := NewProxyNode()
	p.Id = 1
	p.Enabled = true
	if p.Available() {
		t.Fatalf("a fresh node starts Unknown: it must not be selectable")
	}
	if p.StatusValue() != "Unknown" {
		t.Fatalf("StatusValue = %q", p.StatusValue())
	}

	p.MarkCheckFailure("dial timeout", time.Now(), 3)
	if p.Available() {
		t.Fatalf("first failure must not flip a 3-strike node")
	}
	p.MarkCheckFailure("dial timeout", time.Now(), 3)
	p.MarkCheckFailure("dial timeout", time.Now(), 3)
	if p.StatusValue() != "Unavailable" {
		t.Fatalf("three failures must flip to Unavailable, got %q", p.StatusValue())
	}
	if p.Available() {
		t.Fatalf("unavailable node must not be selectable")
	}

	p.MarkCheckSuccess(time.Millisecond*82, time.Now(), 1)
	if !p.Available() {
		t.Fatalf("one success must recover (threshold 1)")
	}
	if p.StatusValue() != "Available" {
		t.Fatalf("StatusValue = %q", p.StatusValue())
	}
	_, latency, _, _, failures, _ := p.HealthSnapshot()
	if latency != 82 || failures != 0 {
		t.Fatalf("latency=%d failures=%d", latency, failures)
	}
}

func TestProxyNodeTags(t *testing.T) {
	p := NewProxyNode()
	if err := p.SetTags([]string{" GZ ", "gz", "Telecom", "bad.tag"}); err == nil {
		t.Fatalf("a dot is not a valid tag char: SetTags must reject")
	}
}

func TestProxyNodeTagsValidPath(t *testing.T) {
	p := NewProxyNode()
	if err := p.SetTags([]string{" GZ ", "gz", "Telecom", "gz"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	tags := p.TagList()
	if len(tags) != 2 || tags[0] != "gz" || tags[1] != "telecom" {
		t.Fatalf("tags = %v, want [gz telecom]", tags)
	}
	if !p.HasTag("GZ") {
		t.Fatalf("HasTag must be case-insensitive")
	}
}

func TestProxyNodeViewHidesPassword(t *testing.T) {
	p := NewProxyNode()
	p.Id = 3
	p.Host = "1.2.3.4"
	p.Port = 8080
	p.Username = "u"
	p.Password = "topsecret"
	p.Http = true

	v := p.View()
	if !v.HasPassword {
		t.Fatalf("HasPassword must be true when a password is set")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "topsecret") {
		t.Fatalf("the view leaked the password: %s", raw)
	}
}

func TestProxyNodeDuplicateKey(t *testing.T) {
	p := NewProxyNode()
	p.Host = "1.2.3.4"
	p.Port = 8080
	p.Username = "user"
	k1 := p.DuplicateKey("http")
	p2 := NewProxyNode()
	p2.Host = "1.2.3.4"
	p2.Port = 8080
	p2.Username = "user"
	k2 := p2.DuplicateKey("HTTP ")
	if k1 != k2 {
		t.Fatalf("protocol/host must be case-insensitive: %q vs %q", k1, k2)
	}
	p3 := NewProxyNode()
	p3.Host = "1.2.3.4"
	p3.Port = 8080
	p3.Username = "user"
	if p3.DuplicateKey("socks5") == k1 {
		t.Fatalf("protocol must be part of the duplicate key")
	}
	p4 := NewProxyNode()
	p4.Host = "1.2.3.4"
	p4.Port = 8080
	p4.Username = "User"
	if p4.DuplicateKey("http") == k1 {
		t.Fatalf("username stays case-sensitive: User and user are distinct credentials")
	}
}

func TestProxyNodeJSONPersistShape(t *testing.T) {
	p := NewProxyNode()
	p.Id = 9
	p.Name = "node"
	p.Host = "h"
	p.Port = 1
	p.Password = "pw"
	p.Enabled = true
	p.MarkCheckSuccess(time.Millisecond, time.Now(), 1)

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "\"Password\":\"pw\"") {
		t.Fatalf("config password must persist: %s", s)
	}
	if strings.Contains(s, "Status") || strings.Contains(s, "Latency") {
		t.Fatalf("runtime health state must not be persisted: %s", s)
	}
	if !strings.Contains(s, "LastSuccessTime") {
		t.Fatalf("LastSuccessTime must persist for the 24h/7d filters: %s", s)
	}
}
