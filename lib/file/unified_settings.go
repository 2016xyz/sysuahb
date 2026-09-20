package file

import (
	"sync"
	"time"
)

// UnifiedSettings holds the runtime knobs of the unified proxy / external
// proxy feature. It is persisted to conf/unified.json and every field is
// normalized into a safe range before use.
//
// Pure internal constants (sticky cache GC interval, worker queue sizes, ...)
// are deliberately NOT exposed here.
type UnifiedSettings struct {
	// --- health check ---
	CheckURL       string `json:"CheckURL"`       // probe target, must answer 204
	CheckInterval  int    `json:"CheckInterval"`  // seconds, healthy nodes
	RetryInterval  int    `json:"RetryInterval"`  // seconds, failing nodes
	CheckTimeout   int    `json:"CheckTimeout"`   // seconds, per probe
	FailThreshold  int    `json:"FailThreshold"`  // consecutive failures -> Unavailable
	RecoverSuccess int    `json:"RecoverSuccess"` // consecutive successes -> Available
	MaxConcurrency int    `json:"MaxConcurrency"` // parallel probe limit

	// --- sticky ---
	MinTTL int `json:"MinTTL"` // seconds
	MaxTTL int `json:"MaxTTL"` // seconds

	// --- proxy nodes ---
	AutoCheckOnAdd    bool `json:"AutoCheckOnAdd"`
	AutoCheckOnImport bool `json:"AutoCheckOnImport"`

	sync.RWMutex
}

// Default values required by the specification.
const (
	defaultCheckURL       = "https://www.gstatic.com/generate_204"
	defaultCheckInterval  = 300 // 5m
	defaultRetryInterval  = 60  // 1m
	defaultCheckTimeout   = 10  // 10s
	defaultFailThreshold  = 3
	defaultRecoverSuccess = 1
	defaultMaxConcurrency = 20
	defaultMinTTLSeconds  = 60
	defaultMaxTTLSeconds  = 24 * 60 * 60
)

// NewUnifiedSettings returns the defaults (never nil).
func NewUnifiedSettings() *UnifiedSettings {
	return &UnifiedSettings{
		CheckURL:          defaultCheckURL,
		CheckInterval:     defaultCheckInterval,
		RetryInterval:     defaultRetryInterval,
		CheckTimeout:      defaultCheckTimeout,
		FailThreshold:     defaultFailThreshold,
		RecoverSuccess:    defaultRecoverSuccess,
		MaxConcurrency:    defaultMaxConcurrency,
		MinTTL:            defaultMinTTLSeconds,
		MaxTTL:            defaultMaxTTLSeconds,
		AutoCheckOnAdd:    true,
		AutoCheckOnImport: true,
	}
}

// GetUnifiedSettings returns the live settings object, lazily created.
func (s *JsonDb) GetUnifiedSettings() *UnifiedSettings {
	if s.Unified == nil {
		s.Unified = NewUnifiedSettings()
	}
	return s.Unified
}

// Normalize clamps every field into its valid range and fills empty ones with
// defaults. It is called on load and on save, so a hand-edited JSON file can
// never break the runtime.
func (u *UnifiedSettings) Normalize() {
	u.Lock()
	defer u.Unlock()
	u.normalizeLocked()
}

// normalizeLocked is Normalize without locking: callers that already hold the
// write lock (Update) must use this to avoid a self-deadlock on the
// non-reentrant RWMutex.
func (u *UnifiedSettings) normalizeLocked() {
	if u.CheckURL == "" {
		u.CheckURL = defaultCheckURL
	}
	if u.CheckInterval < 10 || u.CheckInterval > 24*3600 {
		u.CheckInterval = defaultCheckInterval
	}
	if u.RetryInterval < 10 || u.RetryInterval > 24*3600 {
		u.RetryInterval = defaultRetryInterval
	}
	if u.CheckTimeout < 1 || u.CheckTimeout > 120 {
		u.CheckTimeout = defaultCheckTimeout
	}
	if u.FailThreshold < 1 || u.FailThreshold > 100 {
		u.FailThreshold = defaultFailThreshold
	}
	if u.RecoverSuccess < 1 || u.RecoverSuccess > 100 {
		u.RecoverSuccess = defaultRecoverSuccess
	}
	if u.MaxConcurrency < 1 || u.MaxConcurrency > 1000 {
		u.MaxConcurrency = defaultMaxConcurrency
	}
	if u.MinTTL < 30 || u.MinTTL > defaultMaxTTLSeconds {
		u.MinTTL = defaultMinTTLSeconds
	}
	if u.MaxTTL < u.MinTTL || u.MaxTTL > 7*24*3600 {
		u.MaxTTL = defaultMaxTTLSeconds
	}
}

// Update applies new settings under the write lock and normalizes them, so a
// concurrent reader (health scheduler, sticky cache) never sees a torn value.
func (u *UnifiedSettings) Update(from UnifiedSettings) {
	u.Lock()
	defer u.Unlock()
	u.CheckURL = from.CheckURL
	u.CheckInterval = from.CheckInterval
	u.RetryInterval = from.RetryInterval
	u.CheckTimeout = from.CheckTimeout
	u.FailThreshold = from.FailThreshold
	u.RecoverSuccess = from.RecoverSuccess
	u.MaxConcurrency = from.MaxConcurrency
	u.MinTTL = from.MinTTL
	u.MaxTTL = from.MaxTTL
	u.AutoCheckOnAdd = from.AutoCheckOnAdd
	u.AutoCheckOnImport = from.AutoCheckOnImport
	u.normalizeLocked()
}

// Snapshot returns a locked copy of the settings, safe to marshal or read.
func (u *UnifiedSettings) Snapshot() UnifiedSettings {
	u.RLock()
	defer u.RUnlock()
	return UnifiedSettings{
		CheckURL:          u.CheckURL,
		CheckInterval:     u.CheckInterval,
		RetryInterval:     u.RetryInterval,
		CheckTimeout:      u.CheckTimeout,
		FailThreshold:     u.FailThreshold,
		RecoverSuccess:    u.RecoverSuccess,
		MaxConcurrency:    u.MaxConcurrency,
		MinTTL:            u.MinTTL,
		MaxTTL:            u.MaxTTL,
		AutoCheckOnAdd:    u.AutoCheckOnAdd,
		AutoCheckOnImport: u.AutoCheckOnImport,
	}
}

// CheckIntervalDuration etc. helpers keep the time unit conversions in one place.
func (u *UnifiedSettings) CheckIntervalDuration() time.Duration {
	u.RLock()
	defer u.RUnlock()
	return time.Duration(u.CheckInterval) * time.Second
}

func (u *UnifiedSettings) RetryIntervalDuration() time.Duration {
	u.RLock()
	defer u.RUnlock()
	return time.Duration(u.RetryInterval) * time.Second
}

func (u *UnifiedSettings) CheckTimeoutDuration() time.Duration {
	u.RLock()
	defer u.RUnlock()
	return time.Duration(u.CheckTimeout) * time.Second
}

func (u *UnifiedSettings) MinTTLDuration() time.Duration {
	u.RLock()
	defer u.RUnlock()
	return time.Duration(u.MinTTL) * time.Second
}

func (u *UnifiedSettings) MaxTTLDuration() time.Duration {
	u.RLock()
	defer u.RUnlock()
	return time.Duration(u.MaxTTL) * time.Second
}
