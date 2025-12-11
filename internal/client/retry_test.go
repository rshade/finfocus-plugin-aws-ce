package client

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 100*time.Millisecond {
		t.Errorf("expected BaseDelay=100ms, got %v", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("expected MaxDelay=30s, got %v", cfg.MaxDelay)
	}
}

func TestWithRetry_Success(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}

	callCount := 0
	result, err := WithRetry(context.Background(), cfg, func(ctx context.Context) (string, error, bool) {
		callCount++
		return "success", nil, false
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %s", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestWithRetry_NonRetryableError(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}

	expectedErr := errors.New("non-retryable error")
	callCount := 0
	_, err := WithRetry(context.Background(), cfg, func(ctx context.Context) (string, error, bool) {
		callCount++
		return "", expectedErr, false // not retryable
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retry for non-retryable), got %d", callCount)
	}
}

func TestWithRetry_RetryableSuccess(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}

	callCount := 0
	result, err := WithRetry(context.Background(), cfg, func(ctx context.Context) (string, error, bool) {
		callCount++
		if callCount < 3 {
			return "", errors.New("temporary error"), true // retryable
		}
		return "success after retry", nil, false
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success after retry" {
		t.Errorf("expected 'success after retry', got %s", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestWithRetry_MaxRetriesExhausted(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
	}

	expectedErr := errors.New("persistent error")
	callCount := 0
	_, err := WithRetry(context.Background(), cfg, func(ctx context.Context) (string, error, bool) {
		callCount++
		return "", expectedErr, true // always retryable
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	// MaxRetries=2 means 3 total attempts (initial + 2 retries)
	if callCount != 3 {
		t.Errorf("expected 3 calls (initial + 2 retries), got %d", callCount)
	}
}

func TestWithRetry_ContextCancelled(t *testing.T) {
	cfg := RetryConfig{
		MaxRetries: 10,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	callCount := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := WithRetry(ctx, cfg, func(ctx context.Context) (string, error, bool) {
		callCount++
		return "", errors.New("error"), true // retryable
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCalculateDelay(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	maxDelay := 10 * time.Second

	tests := []struct {
		attempt     int
		minExpected time.Duration
		maxExpected time.Duration
	}{
		{0, 90 * time.Millisecond, 120 * time.Millisecond},   // 100ms * 2^0 * (0.9-1.1 jitter)
		{1, 180 * time.Millisecond, 240 * time.Millisecond},  // 100ms * 2^1 * (0.9-1.1 jitter)
		{2, 360 * time.Millisecond, 480 * time.Millisecond},  // 100ms * 2^2 * (0.9-1.1 jitter)
		{10, maxDelay, maxDelay},                              // Should be capped at maxDelay
	}

	for _, tc := range tests {
		delay := calculateDelay(tc.attempt, baseDelay, maxDelay)
		if delay < tc.minExpected || delay > tc.maxExpected {
			t.Errorf("attempt %d: expected delay in range [%v, %v], got %v",
				tc.attempt, tc.minExpected, tc.maxExpected, delay)
		}
	}
}
