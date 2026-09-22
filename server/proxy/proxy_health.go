package proxy

import (
	"errors"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"

	"example.com/svcmgr/lib/file"
	"example.com/svcmgr/lib/logs"
)

// proxyHealthChecker probes every enabled external proxy node in the
// background: healthy nodes every CheckInterval, failing nodes every
// RetryInterval. Probing is capped by MaxConcurrency so importing a thousand
// nodes never starts a thousand simultaneous checks.
type proxyHealthChecker struct {
	mu       sync.Mutex
	pool     *ants.Pool
	poolSize int
	inFlight sync.Map // node id -> struct{}
	stopCh   chan struct{}
	started  bool
}

var (
	globalHealthChecker     *proxyHealthChecker
	globalHealthCheckerOnce sync.Once
)

func globalProxyHealthChecker() *proxyHealthChecker {
	globalHealthCheckerOnce.Do(func() {
		globalHealthChecker = &proxyHealthChecker{
			stopCh: make(chan struct{}),
		}
	})
	return globalHealthChecker
}

// ensurePool (re)sizes the worker pool to the configured concurrency limit.
func (h *proxyHealthChecker) ensurePool(size int) *ants.Pool {
	if size <= 0 {
		size = 20
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pool == nil {
		pool, err := ants.NewPool(size)
		if err != nil {
			logs.Error("proxy health: create pool failed: %v", err)
			return nil
		}
		h.pool = pool
		h.poolSize = size
		return pool
	}
	if h.poolSize != size {
		h.pool.Tune(size)
		h.poolSize = size
	}
	return h.pool
}

// submit queues one probe unless the node already has a probe in flight.
func (h *proxyHealthChecker) submit(id int, fn func()) {
	if _, loaded := h.inFlight.LoadOrStore(id, struct{}{}); loaded {
		return
	}
	pool := h.ensurePool(settingsSnapshot().MaxConcurrency)
	if pool == nil {
		h.inFlight.Delete(id)
		return
	}
	err := pool.Submit(func() {
		defer h.inFlight.Delete(id)
		fn()
	})
	if err != nil {
		h.inFlight.Delete(id)
		logs.Warn("proxy health: submit probe for node %d failed: %v", id, err)
	}
}

// CheckNodeNow probes one node; safe to call from the web layer.
func CheckNodeNow(id int) {
	h := globalProxyHealthChecker()
	h.submit(id, func() { runProxyCheck(id) })
}

// CheckAllNodes queues a probe for every node.
func CheckAllNodes() int {
	count := 0
	for _, node := range listProxyNodes() {
		h := globalProxyHealthChecker()
		if _, loaded := h.inFlight.Load(node.Id); loaded {
			continue
		}
		CheckNodeNow(node.Id)
		count++
	}
	return count
}

func listProxyNodes() []*file.ProxyNode {
	list := make([]*file.ProxyNode, 0)
	file.GetDb().JsonDb.Proxies.Range(func(key, value interface{}) bool {
		if node, ok := value.(*file.ProxyNode); ok && node != nil {
			list = append(list, node)
		}
		return true
	})
	return list
}

func settingsSnapshot() file.UnifiedSettingsCopy {
	settings := file.GetDb().JsonDb.GetUnifiedSettings()
	if settings == nil {
		def := file.NewUnifiedSettings()
		cp := def.Snapshot()
		return cp
	}
	return settings.Snapshot()
}

// runProxyCheck probes one node through its own upstream and records the
// outcome. The probe really traverses the proxy: it CONNECTs to the check URL
// host through the upstream and requires HTTP 204.
func runProxyCheck(id int) {
	node, err := file.GetDb().GetProxyNode(id)
	if err != nil || node == nil {
		return
	}
	if !node.EnabledState() {
		node.MarkDisabled()
		return
	}
	settings := settingsSnapshot()
	checkURL := settings.CheckURL
	if checkURL == "" {
		checkURL = "https://www.gstatic.com/generate_204"
	}
	protocol, err := probeProtocolFor(node)
	if err != nil {
		node.MarkCheckFailure("no supported protocol", time.Now(), settings.FailThreshold)
		file.GetDb().JsonDb.StoreProxyToJsonFile()
		return
	}
	latency, err := probeProxy(node, checkURL, protocol, time.Duration(settings.CheckTimeout)*time.Second)
	now := time.Now()
	if err != nil {
		node.MarkCheckFailure(truncateError(err.Error(), 200), now, settings.FailThreshold)
		logs.Warn("proxy health: node %d (%s) check failed: %v", node.Id, node.Name, err)
	} else {
		node.MarkCheckSuccess(latency, now, settings.RecoverSuccess)
	}
	file.GetDb().JsonDb.StoreProxyToJsonFile()
}

// probeProtocolFor picks which upstream protocol to use for the health probe:
// HTTP is preferred when both are available, because CONNECT through an HTTP
// proxy is the most widely supported form.
func probeProtocolFor(node *file.ProxyNode) (egressProtocol, error) {
	if node.IsTunnel() {
		return protoHttp, nil
	}
	if node.SupportsHttp() {
		return protoHttp, nil
	}
	if node.SupportsSocks5() {
		return protoSocks5, nil
	}
	return protoHttp, errors.New("neither HTTP nor SOCKS5 is enabled")
}

func truncateError(msg string, max int) string {
	if len(msg) <= max {
		return msg
	}
	return msg[:max]
}

// StartProxyHealthLoop launches the background scheduler. It is safe to call
// multiple times; the loop only starts once.
func StartProxyHealthLoop() {
	h := globalProxyHealthChecker()
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return
	}
	h.started = true
	h.mu.Unlock()
	go h.loop()
}

func (h *proxyHealthChecker) loop() {
	ticker := time.NewTicker(proxyHealthScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.scheduleDue()
		}
	}
}

// proxyHealthScanInterval is how often the scheduler looks for due probes.
// Internal constant: never exposed on the settings page.
const proxyHealthScanInterval = 5 * time.Second

// scheduleDue queues a probe for every node whose next check time has passed.
func (h *proxyHealthChecker) scheduleDue() {
	settings := settingsSnapshot()
	now := time.Now()
	for _, node := range listProxyNodes() {
		if !node.EnabledState() {
			continue
		}
		status, _, lastCheck, _, _, _ := node.HealthSnapshot()
		interval := settings.CheckIntervalDuration()
		if status != file.ProxyStatusAvailable {
			// Unknown and Unavailable nodes are retried faster.
			interval = settings.RetryIntervalDuration()
		}
		if interval <= 0 {
			continue
		}
		if lastCheck.IsZero() || now.Sub(lastCheck) >= interval {
			h.submit(node.Id, func() { runProxyCheck(node.Id) })
		}
	}
}
