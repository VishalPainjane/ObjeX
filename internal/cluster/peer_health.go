package cluster

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/VishalPainjane/objex/internal/metrics"
)

const (
	defaultPeerProbeInterval   = 5 * time.Second
	defaultPeerProbeTimeout    = 2 * time.Second
	defaultPeerFailThreshold   = 3
	defaultPeerRecoverThreshold = 2
)

// PeerHealthTracker holds runtime reachability for cluster peers (not persisted).
type PeerHealthTracker struct {
	mu        sync.RWMutex
	localID   string
	reachable map[string]bool
	failures  map[string]int
	successes map[string]int
}

// NewPeerHealthTracker initializes peer state as reachable until proven otherwise.
func NewPeerHealthTracker(membership Membership) *PeerHealthTracker {
	t := &PeerHealthTracker{
		localID:   membership.LocalNodeID(),
		reachable: make(map[string]bool),
		failures:  make(map[string]int),
		successes: make(map[string]int),
	}
	for _, n := range membership.ListNodes() {
		if n.ID == t.localID {
			continue
		}
		t.reachable[n.ID] = true
	}
	return t
}

// IsReachable reports whether a peer is considered reachable (defaults true until marked down).
func (t *PeerHealthTracker) IsReachable(nodeID string) bool {
	if t == nil || nodeID == t.localID {
		return true
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	reachable, ok := t.reachable[nodeID]
	if !ok {
		return true
	}
	return reachable
}

// Snapshot returns a copy of peer reachability (excludes local node).
func (t *PeerHealthTracker) Snapshot() map[string]bool {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]bool, len(t.reachable))
	for id, ok := range t.reachable {
		out[id] = ok
	}
	return out
}

func (t *PeerHealthTracker) recordSuccess(nodeID string, recoverThreshold int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.failures[nodeID] = 0
	if !t.reachable[nodeID] {
		t.successes[nodeID]++
		if t.successes[nodeID] >= recoverThreshold {
			t.reachable[nodeID] = true
			t.successes[nodeID] = 0
			metrics.SetPeerReachable(nodeID, true)
			return true
		}
		return false
	}
	t.successes[nodeID] = 0
	return false
}

func (t *PeerHealthTracker) recordFailure(nodeID string, failThreshold int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.successes[nodeID] = 0
	if t.reachable[nodeID] {
		t.failures[nodeID]++
		if t.failures[nodeID] >= failThreshold {
			t.reachable[nodeID] = false
			t.failures[nodeID] = 0
			metrics.SetPeerReachable(nodeID, false)
			return true
		}
		return false
	}
	t.failures[nodeID]++
	return false
}

// PeerHealthMonitor probes peer /health/live endpoints.
type PeerHealthMonitor struct {
	membership       Membership
	tracker          *PeerHealthTracker
	client           *http.Client
	interval         time.Duration
	timeout          time.Duration
	failThreshold    int
	recoverThreshold int
	onRecovered      func(nodeID string)
	logger           *slog.Logger
}

// PeerHealthConfig configures peer probing.
type PeerHealthConfig struct {
	Interval         time.Duration
	Timeout          time.Duration
	FailThreshold    int
	RecoverThreshold int
	OnRecovered      func(nodeID string)
}

// NewPeerHealthMonitor creates a peer health monitor.
func NewPeerHealthMonitor(membership Membership, tracker *PeerHealthTracker, cfg PeerHealthConfig, logger *slog.Logger) *PeerHealthMonitor {
	if tracker == nil {
		tracker = NewPeerHealthTracker(membership)
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = defaultPeerProbeInterval
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultPeerProbeTimeout
	}
	failThreshold := cfg.FailThreshold
	if failThreshold <= 0 {
		failThreshold = defaultPeerFailThreshold
	}
	recoverThreshold := cfg.RecoverThreshold
	if recoverThreshold <= 0 {
		recoverThreshold = defaultPeerRecoverThreshold
	}
	return &PeerHealthMonitor{
		membership:       membership,
		tracker:          tracker,
		client:           &http.Client{Timeout: timeout},
		interval:         interval,
		timeout:          timeout,
		failThreshold:    failThreshold,
		recoverThreshold: recoverThreshold,
		onRecovered:      cfg.OnRecovered,
		logger:           logger,
	}
}

// Run probes peers until ctx is cancelled.
func (m *PeerHealthMonitor) Run(ctx context.Context) {
	if m == nil || m.membership == nil || m.membership.Len() <= 1 {
		return
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	m.probeOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.probeOnce(ctx)
		}
	}
}

// ProbeOnce runs a single probe round (for tests).
func (m *PeerHealthMonitor) ProbeOnce(ctx context.Context) {
	m.probeOnce(ctx)
}

func (m *PeerHealthMonitor) probeOnce(ctx context.Context) {
	localID := m.membership.LocalNodeID()
	for _, n := range m.membership.ListNodes() {
		if n.ID == localID {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nodeBaseURL(n.Address)+"/health/live", nil)
		if err != nil {
			if m.tracker.recordFailure(n.ID, m.failThreshold) && m.logger != nil {
				m.logger.Warn("peer marked unreachable", "peer", n.ID, "error", err)
			}
			continue
		}
		resp, err := m.client.Do(req)
		if err != nil {
			if m.tracker.recordFailure(n.ID, m.failThreshold) && m.logger != nil {
				m.logger.Warn("peer marked unreachable", "peer", n.ID, "error", err)
			}
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if m.tracker.recordFailure(n.ID, m.failThreshold) && m.logger != nil {
				m.logger.Warn("peer marked unreachable", "peer", n.ID, "status", resp.StatusCode)
			}
			continue
		}
		if m.tracker.recordSuccess(n.ID, m.recoverThreshold) {
			if m.logger != nil {
				m.logger.Info("peer recovered", "peer", n.ID)
			}
			metrics.RecordPeerRecovery(n.ID)
			if m.onRecovered != nil {
				m.onRecovered(n.ID)
			}
		}
	}
}
