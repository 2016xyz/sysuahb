package proxy

import (
	"testing"
	"time"
)

func TestParseRoute(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     RouteRequest
		wantOK   bool
	}{
		// --- auto ---------------------------------------------------------
		{name: "auto is fully random", username: "auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeRandom, TTL: unifiedDefaultTTL}},
		{name: "auto with ttl is invalid", username: "auto-30m", wantOK: false},
		{name: "uppercase AUTO invalid", username: "AUTO", wantOK: false},

		// --- fixed client id ----------------------------------------------
		{name: "fixed client 12", username: "12", wantOK: true,
			want: RouteRequest{Mode: unifiedModeFixed, ClientID: 12, TTL: unifiedDefaultTTL}},
		{name: "fixed client 1", username: "1", wantOK: true,
			want: RouteRequest{Mode: unifiedModeFixed, ClientID: 1, TTL: unifiedDefaultTTL}},
		{name: "client 0 invalid", username: "0", wantOK: false},
		{name: "negative client invalid", username: "-5", wantOK: false},

		// --- tag random ----------------------------------------------------
		{name: "gz.auto is tag random", username: "gz.auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeRandom, Tags: []string{"gz"}, TTL: unifiedDefaultTTL}},
		{name: "jp.auto is tag random", username: "jp.auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeRandom, Tags: []string{"jp"}, TTL: unifiedDefaultTTL}},
		{name: "tag random must not carry a ttl", username: "gz.auto-10m", wantOK: false},
		{name: "dot requires the auto keyword", username: "gz.hk", wantOK: false},

		// --- sticky random -------------------------------------------------
		{name: "abc-auto sticky default ttl", username: "abc-auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", TTL: unifiedDefaultTTL}},
		{name: "abc-auto-10m", username: "abc-auto-10m", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", TTL: 10 * time.Minute}},
		{name: "abc-auto-30m", username: "abc-auto-30m", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", TTL: 30 * time.Minute}},
		{name: "crawler.jp-auto", username: "crawler.jp-auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "crawler", Tags: []string{"jp"}, TTL: unifiedDefaultTTL}},
		{name: "crawler.jp-auto-2h", username: "crawler.jp-auto-2h", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "crawler", Tags: []string{"jp"}, TTL: 2 * time.Hour}},
		{name: "user001.hk-auto-1d", username: "user001.hk-auto-1d", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "user001", Tags: []string{"hk"}, TTL: 24 * time.Hour}},
		{name: "abc.gz-auto", username: "abc.gz-auto", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", Tags: []string{"gz"}, TTL: unifiedDefaultTTL}},
		{name: "abc.gz-auto-30m", username: "abc.gz-auto-30m", wantOK: true,
			want: RouteRequest{Mode: unifiedModeSticky, StickyKey: "abc", Tags: []string{"gz"}, TTL: 30 * time.Minute}},

		// --- invalid -------------------------------------------------------
		{name: "empty", username: "", wantOK: false},
		{name: "blank", username: "   ", wantOK: false},
		{name: "plain word", username: "foo", wantOK: false},
		{name: "auto without key", username: "-auto", wantOK: false},
		{name: "empty ttl after marker", username: "abc-auto-", wantOK: false},
		{name: "auto with empty ttl", username: "auto-", wantOK: false},
		{name: "bare auto dashes", username: "-auto-", wantOK: false},
		{name: "dot without tag", username: ".auto", wantOK: false},
		{name: "empty sticky key before tag", username: "abc.-auto", wantOK: false},
		{name: "separator only key", username: "--auto", wantOK: false},
		{name: "separator only tag", username: "-.auto", wantOK: false},
		{name: "zero ttl", username: "abc-auto-0m", wantOK: false},
		{name: "ttl unit only", username: "abc-auto-m", wantOK: false},
		{name: "unknown unit", username: "abc-auto-5x", wantOK: false},
		{name: "uppercase unit", username: "abc-auto-30M", wantOK: false},
		{name: "trailing garbage", username: "abc-auto-30m-x", wantOK: false},
		{name: "ttl above 24h", username: "abc-auto-25h", wantOK: false},
		{name: "ttl 2d above 24h", username: "abc-auto-2d", wantOK: false},
		{name: "dot in tag", username: "abc.gz.hk-auto", wantOK: false},
		{name: "space in key", username: "a bc-auto", wantOK: false},
		{name: "uppercase tag", username: "abc.GZ-auto", wantOK: false},
		{name: "dash only key", username: "--auto", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRoute(tt.username)
			if !tt.wantOK {
				if err == nil {
					t.Fatalf("ParseRoute(%q) error = nil, want error (got %+v)", tt.username, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRoute(%q) unexpected error = %v", tt.username, err)
			}
			if got.Mode != tt.want.Mode {
				t.Fatalf("ParseRoute(%q).Mode = %v, want %v", tt.username, got.Mode, tt.want.Mode)
			}
			if got.ClientID != tt.want.ClientID {
				t.Fatalf("ParseRoute(%q).ClientID = %d, want %d", tt.username, got.ClientID, tt.want.ClientID)
			}
			if got.StickyKey != tt.want.StickyKey {
				t.Fatalf("ParseRoute(%q).StickyKey = %q, want %q", tt.username, got.StickyKey, tt.want.StickyKey)
			}
			if len(got.Tags) != len(tt.want.Tags) {
				t.Fatalf("ParseRoute(%q).Tags = %v, want %v", tt.username, got.Tags, tt.want.Tags)
			}
			for i := range tt.want.Tags {
				if got.Tags[i] != tt.want.Tags[i] {
					t.Fatalf("ParseRoute(%q).Tags = %v, want %v", tt.username, got.Tags, tt.want.Tags)
				}
			}
			if got.TTL != tt.want.TTL {
				t.Fatalf("ParseRoute(%q).TTL = %v, want %v", tt.username, got.TTL, tt.want.TTL)
			}
		})
	}
}

func TestParseUnifiedTTLBounds(t *testing.T) {
	if ttl, err := parseUnifiedTTL("1m"); err != nil || ttl != time.Minute {
		t.Fatalf("1m => %v, %v", ttl, err)
	}
	if ttl, err := parseUnifiedTTL("24h"); err != nil || ttl != 24*time.Hour {
		t.Fatalf("24h => %v, %v", ttl, err)
	}
	if ttl, err := parseUnifiedTTL("1d"); err != nil || ttl != 24*time.Hour {
		t.Fatalf("1d => %v, %v", ttl, err)
	}
	if _, err := parseUnifiedTTL("1441m"); err == nil {
		t.Fatalf("1441m must exceed the 24h ceiling")
	}
	if _, err := parseUnifiedTTL("0m"); err == nil {
		t.Fatalf("0m must be rejected")
	}
}
