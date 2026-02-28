package infra

import (
	"context"
	"sync"
	"time"

	"github.com/joaopedro/hivemind/gen/peerpb"
	"github.com/joaopedro/hivemind/internal/logger"
)

// PeerHealth tracks the health state of a single peer.
type PeerHealth struct {
	PeerID        string
	IsAlive       bool
	LastSeen      time.Time
	ConsecFails   int
	AvgLatencyMs  float64
	CircuitBreaker *CircuitBreaker
}

// HealthMonitor continuously monitors peer health via gRPC health checks.
// Peers failing 3 consecutive checks are marked dead and trigger layer redistribution.
type HealthMonitor struct {
	mu           sync.RWMutex
	peers        map[string]*PeerHealth
	registry     *PeerRegistry
	interval     time.Duration
	maxFails     int
	cancel       context.CancelFunc
	onPeerDead   func(peerID string)
	onPeerAlive  func(peerID string)
}

// HealthMonitorConfig configures the health monitor.
type HealthMonitorConfig struct {
	Registry    *PeerRegistry
	Interval    time.Duration // Health check interval (default: 5s)
	MaxFails    int           // Consecutive failures before marking dead (default: 3)
	OnPeerDead  func(peerID string)
	OnPeerAlive func(peerID string)
}

// NewHealthMonitor creates a peer health monitor.
func NewHealthMonitor(cfg HealthMonitorConfig) *HealthMonitor {
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.MaxFails <= 0 {
		cfg.MaxFails = 3
	}

	return &HealthMonitor{
		peers:       make(map[string]*PeerHealth),
		registry:    cfg.Registry,
		interval:    cfg.Interval,
		maxFails:    cfg.MaxFails,
		onPeerDead:  cfg.OnPeerDead,
		onPeerAlive: cfg.OnPeerAlive,
	}
}

// Start begins periodic health checking of all registered peers.
func (hm *HealthMonitor) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	hm.cancel = cancel

	go hm.run(ctx)
	logger.Info("health monitor started", "interval", hm.interval.String())
}

// Stop stops the health monitor.
func (hm *HealthMonitor) Stop() {
	if hm.cancel != nil {
		hm.cancel()
	}
	logger.Info("health monitor stopped")
}

// RegisterPeer adds a peer to health monitoring.
func (hm *HealthMonitor) RegisterPeer(peerID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	hm.peers[peerID] = &PeerHealth{
		PeerID:   peerID,
		IsAlive:  true,
		LastSeen: time.Now(),
		CircuitBreaker: NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:  hm.maxFails,
			ResetTimeout: 30 * time.Second,
			PeerID:       peerID,
			OnStateChange: func(pid string, _, to CircuitState) {
				if to == CircuitOpen && hm.onPeerDead != nil {
					hm.onPeerDead(pid)
				}
			},
		}),
	}
}

// UnregisterPeer removes a peer from health monitoring.
func (hm *HealthMonitor) UnregisterPeer(peerID string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	delete(hm.peers, peerID)
}

// GetPeerHealth returns the health state of a specific peer.
func (hm *HealthMonitor) GetPeerHealth(peerID string) (*PeerHealth, bool) {
	hm.mu.RLock()
	defer hm.mu.RUnlock()
	ph, ok := hm.peers[peerID]
	return ph, ok
}

// GetAllHealth returns health state for all monitored peers.
func (hm *HealthMonitor) GetAllHealth() map[string]*PeerHealth {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	result := make(map[string]*PeerHealth, len(hm.peers))
	for k, v := range hm.peers {
		result[k] = v
	}
	return result
}

// AlivePeerCount returns how many peers are currently alive.
func (hm *HealthMonitor) AlivePeerCount() int {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	count := 0
	for _, ph := range hm.peers {
		if ph.IsAlive {
			count++
		}
	}
	return count
}

func (hm *HealthMonitor) run(ctx context.Context) {
	ticker := time.NewTicker(hm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hm.checkAllPeers(ctx)
		}
	}
}

func (hm *HealthMonitor) checkAllPeers(ctx context.Context) {
	hm.mu.RLock()
	peerIDs := make([]string, 0, len(hm.peers))
	for id := range hm.peers {
		peerIDs = append(peerIDs, id)
	}
	hm.mu.RUnlock()

	for _, peerID := range peerIDs {
		hm.checkPeer(ctx, peerID)
	}
}

func (hm *HealthMonitor) checkPeer(ctx context.Context, peerID string) {
	peer, ok := hm.registry.GetPeer(peerID)
	if !ok {
		return
	}

	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	start := time.Now()
	_, err := peer.Client().HealthCheck(checkCtx, &peerpb.Ping{
		Timestamp: time.Now().UnixMilli(),
	})

	latency := float64(time.Since(start).Milliseconds())

	hm.mu.Lock()
	ph, exists := hm.peers[peerID]
	if !exists {
		hm.mu.Unlock()
		return
	}

	if err != nil {
		ph.ConsecFails++
		ph.CircuitBreaker.RecordFailure()

		if ph.ConsecFails >= hm.maxFails && ph.IsAlive {
			ph.IsAlive = false
			hm.mu.Unlock()

			logger.Warn("peer marked as dead",
				"peer_id", peerID,
				"consecutive_failures", ph.ConsecFails,
			)

			if hm.onPeerDead != nil {
				hm.onPeerDead(peerID)
			}
			return
		}

		hm.mu.Unlock()
		return
	}

	// Success
	wasAlive := ph.IsAlive
	ph.IsAlive = true
	ph.LastSeen = time.Now()
	ph.ConsecFails = 0
	ph.AvgLatencyMs = (ph.AvgLatencyMs*0.7 + latency*0.3) // Exponential moving average
	ph.CircuitBreaker.RecordSuccess()
	hm.mu.Unlock()

	if !wasAlive {
		logger.Info("peer recovered", "peer_id", peerID, "latency_ms", latency)
		if hm.onPeerAlive != nil {
			hm.onPeerAlive(peerID)
		}
	}
}
