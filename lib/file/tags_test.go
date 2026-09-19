package file

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeTags(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "nil", in: nil, want: nil},
		{name: "empty", in: []string{}, want: nil},
		{name: "trim lower and dedupe", in: []string{" GZ ", "gz", "Telecom"}, want: []string{"gz", "telecom"}},
		{name: "blank entries dropped", in: []string{"", "  ", "gz"}, want: []string{"gz"}},
		{name: "dash and underscore allowed", in: []string{"jp-1", "hk_2"}, want: []string{"jp-1", "hk_2"}},
		{name: "digit tag allowed", in: []string{"12"}, want: []string{"12"}},
		{name: "dot rejected", in: []string{"gz.hk"}, wantErr: true},
		{name: "space inside rejected", in: []string{"gz hk"}, wantErr: true},
		{name: "upper is normalized not rejected", in: []string{"GZ"}, want: []string{"gz"}},
		{name: "slash rejected", in: []string{"gz/hk"}, wantErr: true},
		{name: "chinese rejected", in: []string{"广州"}, wantErr: true},
		{name: "star rejected", in: []string{"*"}, wantErr: true},
		{name: "order preserved", in: []string{"b", "a"}, want: []string{"b", "a"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeTags(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeTags(%v) error = nil, want error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTags(%v) unexpected error = %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("NormalizeTags(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitTagsText(t *testing.T) {
	got := SplitTagsText(" gz,\ntelecom; jp ")
	want := []string{"gz", "telecom", "jp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitTagsText() = %v, want %v", got, want)
	}
	if SplitTagsText("   ") != nil {
		t.Fatalf("SplitTagsText(blank) should be nil")
	}
}

func TestClientHasTagAndSetTags(t *testing.T) {
	c := &Client{Id: 1, Flow: new(Flow), Status: true, IsConnect: true}

	if err := c.SetTags([]string{" GZ ", "gz", "Telecom", "bad.tag"}); err == nil {
		t.Fatalf("SetTags with a dotted tag must fail")
	}
	if err := c.SetTags([]string{" GZ ", "gz", "Telecom"}); err != nil {
		t.Fatalf("SetTags() unexpected error = %v", err)
	}
	if !reflect.DeepEqual(c.Tags, []string{"gz", "telecom"}) {
		t.Fatalf("Tags = %v, want [gz telecom]", c.Tags)
	}
	if !c.HasTag("gz") || !c.HasTag("GZ") {
		t.Fatalf("HasTag(gz) should be true")
	}
	if c.HasTag("jp") || c.HasTag("") {
		t.Fatalf("HasTag(jp)/HasTag(empty) should be false")
	}
	if got := c.TagList(); !reflect.DeepEqual(got, []string{"gz", "telecom"}) {
		t.Fatalf("TagList() = %v, want [gz telecom]", got)
	}

	if err := c.SetTags(nil); err != nil {
		t.Fatalf("SetTags(nil) unexpected error = %v", err)
	}
	if len(c.Tags) != 0 || c.HasTag("gz") {
		t.Fatalf("tags should be cleared, got %v", c.Tags)
	}
}

func TestClientAvailable(t *testing.T) {
	c := &Client{Id: 1, Status: true, IsConnect: true}
	if !c.Available() {
		t.Fatalf("connected + allowed client must be available")
	}
	c.IsConnect = false
	if c.Available() {
		t.Fatalf("offline client must not be available")
	}
	c.IsConnect = true
	c.Status = false
	if c.Available() {
		t.Fatalf("disallowed client must not be available")
	}
	var nilClient *Client
	if nilClient.Available() || nilClient.HasTag("gz") {
		t.Fatalf("nil client must not be available and must not own tags")
	}
}

// TestClientTagsJSONRoundTrip makes sure tags survive the JSON persistence
// used by clients.json, i.e. they survive a NPS restart.
func TestClientTagsJSONRoundTrip(t *testing.T) {
	c := &Client{Id: 7, VerifyKey: "v7", Flow: new(Flow)}
	if err := c.SetTags([]string{"gz", "telecom"}); err != nil {
		t.Fatalf("SetTags() unexpected error = %v", err)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal error = %v", err)
	}
	if !strings.Contains(string(raw), `"Tags":["gz","telecom"]`) {
		t.Fatalf("marshalled client misses tags: %s", raw)
	}
	var back Client
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if !reflect.DeepEqual(back.Tags, []string{"gz", "telecom"}) {
		t.Fatalf("round trip tags = %v, want [gz telecom]", back.Tags)
	}
}
