package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/joaopedro/hivemind/internal/infra"
	"github.com/joaopedro/hivemind/internal/logger"
	"github.com/joaopedro/hivemind/internal/models"
)

// ResilienceService handles fault tolerance: retry with backoff,
// layer redistribution when peers disconnect, and peer recovery.
type ResilienceService struct {
	roomSvc      RoomService
	peerRegistry *infra.PeerRegistry
	monitor      *infra.HealthMonitor
	localPeerID  string
}

// NewResilienceService creates a resilience service.
func NewResilienceService(
	roomSvc RoomService,
	peerRegistry *infra.PeerRegistry,
	localPeerID string,
) *ResilienceService {
	rs := &ResilienceService{
		roomSvc:      roomSvc,
		peerRegistry: peerRegistry,
		localPeerID:  localPeerID,
	}

	rs.monitor = infra.NewHealthMonitor(infra.HealthMonitorConfig{
		Registry:    peerRegistry,
		Interval:    5 * time.Second,
		MaxFails:    3,
		OnPeerDead:  rs.handlePeerDead,
		OnPeerAlive: rs.handlePeerAlive,
	})

	return rs
}

// Start begins health monitoring.
func (rs *ResilienceService) Start() {
	rs.monitor.Start()
}

// Stop stops health monitoring.
func (rs *ResilienceService) Stop() {
	rs.monitor.Stop()
}

// RegisterPeer adds a peer to health monitoring.
func (rs *ResilienceService) RegisterPeer(peerID string) {
	rs.monitor.RegisterPeer(peerID)
}

// UnregisterPeer removes a peer from health monitoring.
func (rs *ResilienceService) UnregisterPeer(peerID string) {
	rs.monitor.UnregisterPeer(peerID)
}

// Monitor returns the health monitor for external queries.
func (rs *ResilienceService) Monitor() *infra.HealthMonitor {
	return rs.monitor
}

// handlePeerDead is called when a peer is detected as dead.
// It triggers layer redistribution across remaining peers.
func (rs *ResilienceService) handlePeerDead(peerID string) {
	logger.Warn("handling peer death",
		"dead_peer", peerID,
		"local_peer", rs.localPeerID,
	)

	room := rs.roomSvc.CurrentRoom()
	if room == nil {
		return
	}

	// Only host redistributes layers
	if room.HostID != rs.localPeerID {
		logger.Info("not host, skipping layer redistribution",
			"host_id", room.HostID,
			"local_id", rs.localPeerID,
		)
		return
	}

	// Gather surviving peers
	var alivePeers []models.Peer
	for _, p := range room.Peers {
		if p.ID != peerID {
			alivePeers = append(alivePeers, p)
		}
	}

	if len(alivePeers) == 0 {
		logger.Error("all peers dead, room cannot continue")
		return
	}

	// Redistribute layers across alive peers
	newAssignments := AssignLayersByVRAM(alivePeers, room.TotalLayers)
	if newAssignments == nil {
		logger.Error("layer redistribution failed")
		return
	}

	// Update peer layer assignments in the registry
	for pID, layers := range newAssignments {
		int32Layers := make([]int32, len(layers))
		for i, l := range layers {
			int32Layers[i] = int32(l)
		}
		rs.peerRegistry.UpdatePeerLayers(pID, int32Layers)
	}

	logger.Info("layers redistributed after peer death",
		"dead_peer", peerID,
		"alive_peers", len(alivePeers),
		"total_layers", room.TotalLayers,
	)

	// Remove the dead peer from registry
	rs.peerRegistry.RemovePeer(peerID)
}

// handlePeerAlive is called when a previously dead peer recovers.
func (rs *ResilienceService) handlePeerAlive(peerID string) {
	logger.Info("peer recovered, may need layer rebalancing",
		"recovered_peer", peerID,
	)

	// In a production system, this would trigger hot-join layer redistribution
	// For now, log the event — full hot-join is complex (pause → redistribute → resume)
}

// RetryConfig holds retry configuration for resilient operations.
type RetryConfig struct {
	MaxAttempts int
	InitialWait time.Duration
	MaxWait     time.Duration
	Multiplier  float64
}

// DefaultRetryConfig returns the default retry configuration.
// 3 attempts with exponential backoff: 500ms, 1s, 2s.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 500 * time.Millisecond,
		MaxWait:     2 * time.Second,
		Multiplier:  2.0,
	}
}

// TensorTransferRetryConfig returns retry config optimized for tensor transfers.
func TensorTransferRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		InitialWait: 500 * time.Millisecond,
		MaxWait:     2 * time.Second,
		Multiplier:  2.0,
	}
}

// WithRetry executes a function with exponential backoff retry.
// Returns the result of the first successful attempt, or the last error.
func WithRetry[T any](ctx context.Context, cfg RetryConfig, operation string, fn func(ctx context.Context) (T, error)) (T, error) {
	var lastErr error
	var zero T
	wait := cfg.InitialWait

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		result, err := fn(ctx)
		if err == nil {
			if attempt > 1 {
				logger.Info("retry succeeded",
					"operation", operation,
					"attempt", attempt,
				)
			}
			return result, nil
		}

		lastErr = err

		if attempt == cfg.MaxAttempts {
			break
		}

		logger.Warn("operation failed, retrying",
			"operation", operation,
			"attempt", attempt,
			"max_attempts", cfg.MaxAttempts,
			"error", err.Error(),
			"next_wait", wait.String(),
		)

		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(wait):
		}

		// Exponential backoff with cap
		wait = time.Duration(float64(wait) * cfg.Multiplier)
		if wait > cfg.MaxWait {
			wait = cfg.MaxWait
		}
	}

	return zero, fmt.Errorf("%s failed after %d attempts: %w", operation, cfg.MaxAttempts, lastErr)
}

// AdaptiveTimeout calculates a timeout based on measured latency.
// Returns 3x the average latency, clamped between min and max.
func AdaptiveTimeout(avgLatencyMs float64, minTimeout, maxTimeout time.Duration) time.Duration {
	timeout := time.Duration(avgLatencyMs*3) * time.Millisecond

	if timeout < minTimeout {
		return minTimeout
	}
	if timeout > maxTimeout {
		return maxTimeout
	}

	return timeout
}

// IsSlowPeer checks if a peer's latency is significantly above the room average.
// Returns true if the peer's latency is > 5x the room average.
func IsSlowPeer(peerLatencyMs, roomAvgLatencyMs float64) bool {
	if roomAvgLatencyMs <= 0 {
		return false
	}
	return peerLatencyMs > roomAvgLatencyMs*5
}

// ExponentialBackoff calculates the backoff duration for a given attempt.
func ExponentialBackoff(attempt int, base time.Duration, maxBackoff time.Duration) time.Duration {
	backoff := time.Duration(float64(base) * math.Pow(2, float64(attempt-1)))
	if backoff > maxBackoff {
		return maxBackoff
	}
	return backoff
}
