package infra

import (
	"sync"
	"time"

	"github.com/joaopedro/hivemind/internal/logger"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed allows requests through.
	CircuitClosed CircuitState = iota
	// CircuitOpen blocks all requests.
	CircuitOpen
	// CircuitHalfOpen allows a single probe request.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements a per-peer circuit breaker pattern.
// After `maxFailures` consecutive failures, the circuit opens for `resetTimeout`.
// After the timeout, a single probe request is allowed (half-open state).
type CircuitBreaker struct {
	mu           sync.RWMutex
	state        CircuitState
	failCount    int
	maxFailures  int
	resetTimeout time.Duration
	lastFailure  time.Time
	peerID       string
	onStateChange func(peerID string, from, to CircuitState)
}

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	MaxFailures   int
	ResetTimeout  time.Duration
	PeerID        string
	OnStateChange func(peerID string, from, to CircuitState)
}

// NewCircuitBreaker creates a circuit breaker for a specific peer.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.MaxFailures <= 0 {
		cfg.MaxFailures = 3
	}
	if cfg.ResetTimeout <= 0 {
		cfg.ResetTimeout = 30 * time.Second
	}

	return &CircuitBreaker{
		state:         CircuitClosed,
		maxFailures:   cfg.MaxFailures,
		resetTimeout:  cfg.ResetTimeout,
		peerID:        cfg.PeerID,
		onStateChange: cfg.OnStateChange,
	}
}

// AllowRequest checks if the circuit breaker allows a request.
// Returns true if the request should be allowed.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if reset timeout has elapsed
		if time.Since(cb.lastFailure) >= cb.resetTimeout {
			cb.transitionTo(CircuitHalfOpen)
			return true // Allow a single probe
		}
		return false

	case CircuitHalfOpen:
		return false // Only one probe at a time

	default:
		return false
	}
}

// RecordSuccess records a successful request, resetting the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount = 0
	if cb.state != CircuitClosed {
		cb.transitionTo(CircuitClosed)
	}
}

// RecordFailure records a failed request, potentially opening the circuit.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failCount++
	cb.lastFailure = time.Now()

	if cb.failCount >= cb.maxFailures && cb.state != CircuitOpen {
		cb.transitionTo(CircuitOpen)
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// FailCount returns the current consecutive failure count.
func (cb *CircuitBreaker) FailCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failCount
}

// Reset forces the circuit back to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failCount = 0
	cb.transitionTo(CircuitClosed)
}

func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState

	logger.Info("circuit breaker state change",
		"peer_id", cb.peerID,
		"from", oldState.String(),
		"to", newState.String(),
		"fail_count", cb.failCount,
	)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.peerID, oldState, newState)
	}
}
