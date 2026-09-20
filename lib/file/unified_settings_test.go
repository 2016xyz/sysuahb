package file

import (
	"testing"
	"time"
)

func TestUnifiedSettingsDefaults(t *testing.T) {
	s := NewUnifiedSettings()
	s.Normalize()
	if s.CheckURL == "" {
		t.Fatalf("default check url must be set")
	}
	if s.CheckInterval != 300 || s.RetryInterval != 60 || s.CheckTimeout != 10 {
		t.Fatalf("defaults wrong: %d/%d/%d", s.CheckInterval, s.RetryInterval, s.CheckTimeout)
	}
	if s.FailThreshold != 3 || s.RecoverSuccess != 1 || s.MaxConcurrency != 20 {
		t.Fatalf("defaults wrong: %d/%d/%d", s.FailThreshold, s.RecoverSuccess, s.MaxConcurrency)
	}
	if s.MinTTLDuration() != time.Minute || s.MaxTTLDuration() != 24*time.Hour {
		t.Fatalf("ttl bounds wrong: %v/%v", s.MinTTLDuration(), s.MaxTTLDuration())
	}
}

func TestUnifiedSettingsNormalizeClamps(t *testing.T) {
	s := NewUnifiedSettings()
	s.CheckInterval = 1
	s.RetryInterval = 0
	s.CheckTimeout = 999
	s.FailThreshold = 0
	s.RecoverSuccess = -1
	s.MaxConcurrency = 100000
	s.MinTTL = 5
	s.MaxTTL = 999999999
	s.Normalize()
	if s.CheckInterval < 30 {
		t.Fatalf("interval not clamped: %d", s.CheckInterval)
	}
	if s.RetryInterval < 10 {
		t.Fatalf("retry not clamped: %d", s.RetryInterval)
	}
	if s.FailThreshold < 1 || s.RecoverSuccess < 1 {
		t.Fatalf("thresholds not clamped: %d/%d", s.FailThreshold, s.RecoverSuccess)
	}
	if s.MaxConcurrency > 512 {
		t.Fatalf("concurrency not clamped: %d", s.MaxConcurrency)
	}
	if s.MinTTLDuration() < time.Minute || s.MaxTTLDuration() > 24*time.Hour {
		t.Fatalf("ttl bounds not clamped: %v/%v", s.MinTTLDuration(), s.MaxTTLDuration())
	}
}
