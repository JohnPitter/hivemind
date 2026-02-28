package services

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWithRetry_SuccessFirstAttempt(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	result, err := WithRetry(context.Background(), cfg, "test-op", func(ctx context.Context) (string, error) {
		return "ok", nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "ok" {
		t.Errorf("expected 'ok', got %q", result)
	}
}

func TestWithRetry_SuccessAfterRetries(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	attempt := 0
	result, err := WithRetry(context.Background(), cfg, "test-op", func(ctx context.Context) (int, error) {
		attempt++
		if attempt < 3 {
			return 0, fmt.Errorf("attempt %d failed", attempt)
		}
		return 42, nil
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
}

func TestWithRetry_AllAttemptsFail(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 3,
		InitialWait: 10 * time.Millisecond,
		MaxWait:     100 * time.Millisecond,
		Multiplier:  2.0,
	}

	attempt := 0
	_, err := WithRetry(context.Background(), cfg, "test-op", func(ctx context.Context) (string, error) {
		attempt++
		return "", fmt.Errorf("fail %d", attempt)
	})

	if err == nil {
		t.Fatal("expected error after all retries failed")
	}
	if attempt != 3 {
		t.Errorf("expected 3 attempts, got %d", attempt)
	}
}

func TestWithRetry_ContextCanceled(t *testing.T) {
	cfg := RetryConfig{
		MaxAttempts: 5,
		InitialWait: 100 * time.Millisecond,
		MaxWait:     1 * time.Second,
		Multiplier:  2.0,
	}

	ctx, cancel := context.WithCancel(context.Background())
	attempt := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := WithRetry(ctx, cfg, "test-op", func(ctx context.Context) (string, error) {
		attempt++
		return "", fmt.Errorf("fail")
	})

	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

func TestAdaptiveTimeout(t *testing.T) {
	minT := 5 * time.Second
	maxT := 30 * time.Second

	// Normal latency: 3x avg
	timeout := AdaptiveTimeout(100, minT, maxT)
	if timeout != 300*time.Millisecond {
		// Should be clamped to min
		if timeout != minT {
			t.Errorf("expected min timeout %s for 100ms latency, got %s", minT, timeout)
		}
	}

	// High latency: 3x avg but clamped to max
	timeout = AdaptiveTimeout(15000, minT, maxT)
	if timeout != maxT {
		t.Errorf("expected max timeout %s for 15000ms latency, got %s", maxT, timeout)
	}

	// Medium latency
	timeout = AdaptiveTimeout(5000, minT, maxT)
	expected := 15 * time.Second
	if timeout != expected {
		t.Errorf("expected %s for 5000ms latency, got %s", expected, timeout)
	}
}

func TestIsSlowPeer(t *testing.T) {
	if IsSlowPeer(100, 50) {
		t.Error("100ms should not be slow with 50ms avg (2x, threshold is 5x)")
	}

	if !IsSlowPeer(300, 50) {
		t.Error("300ms should be slow with 50ms avg (6x, above 5x threshold)")
	}

	if IsSlowPeer(100, 0) {
		t.Error("should return false when room avg is 0")
	}
}

func TestExponentialBackoff(t *testing.T) {
	base := 2 * time.Second
	maxB := 8 * time.Second

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 8 * time.Second}, // clamped to max
	}

	for _, tt := range tests {
		result := ExponentialBackoff(tt.attempt, base, maxB)
		if result != tt.expected {
			t.Errorf("attempt %d: expected %s, got %s", tt.attempt, tt.expected, result)
		}
	}
}
