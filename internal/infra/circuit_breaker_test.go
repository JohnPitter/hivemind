package infra

import (
	"testing"
	"time"
)

func TestCircuitBreaker_StartsOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: 1 * time.Second,
		PeerID:       "test-peer",
	})

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed state, got %s", cb.State().String())
	}

	if !cb.AllowRequest() {
		t.Error("closed circuit should allow requests")
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: 1 * time.Second,
		PeerID:       "test-peer",
	})

	// Record 3 failures
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after 2 failures, got %s", cb.State().String())
	}

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("expected open after 3 failures, got %s", cb.State().String())
	}

	if cb.AllowRequest() {
		t.Error("open circuit should not allow requests")
	}
}

func TestCircuitBreaker_SuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: 1 * time.Second,
		PeerID:       "test-peer",
	})

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	if cb.FailCount() != 0 {
		t.Errorf("expected fail count 0 after success, got %d", cb.FailCount())
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after success, got %s", cb.State().String())
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: 50 * time.Millisecond,
		PeerID:       "test-peer",
	})

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State().String())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Should allow probe request and transition to half-open
	if !cb.AllowRequest() {
		t.Error("should allow probe after reset timeout")
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected half-open, got %s", cb.State().String())
	}
}

func TestCircuitBreaker_HalfOpenToClosedOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: 50 * time.Millisecond,
		PeerID:       "test-peer",
	})

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)
	cb.AllowRequest() // Transition to half-open

	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after half-open success, got %s", cb.State().String())
	}
}

func TestCircuitBreaker_StateChangeCallback(t *testing.T) {
	var transitions []string

	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: 50 * time.Millisecond,
		PeerID:       "test-peer",
		OnStateChange: func(peerID string, from, to CircuitState) {
			transitions = append(transitions, from.String()+"->"+to.String())
		},
	})

	cb.RecordFailure()
	cb.RecordFailure() // closed -> open

	time.Sleep(60 * time.Millisecond)
	cb.AllowRequest() // open -> half-open

	cb.RecordSuccess() // half-open -> closed

	expected := []string{"closed->open", "open->half-open", "half-open->closed"}
	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %v", len(expected), len(transitions), transitions)
	}

	for i, e := range expected {
		if transitions[i] != e {
			t.Errorf("transition %d: expected %q, got %q", i, e, transitions[i])
		}
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  2,
		ResetTimeout: 1 * time.Second,
		PeerID:       "test-peer",
	})

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Fatalf("expected open, got %s", cb.State().String())
	}

	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("expected closed after reset, got %s", cb.State().String())
	}
	if cb.FailCount() != 0 {
		t.Errorf("expected fail count 0 after reset, got %d", cb.FailCount())
	}
}
