package client

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// RetryConfig defines configuration for exponential backoff retry.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig returns a default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 5,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   30 * time.Second,
	}
}

// RetryableFunc is a function that can be retried.
// It returns a result, an error, and a boolean indicating if the error is retryable.
type RetryableFunc[T any] func(ctx context.Context) (T, error, bool)

// WithRetry executes a function with exponential backoff retry logic.
func WithRetry[T any](ctx context.Context, cfg RetryConfig, op RetryableFunc[T]) (T, error) {
	var empty T
	var err error
	var retryable bool
	var result T

	for i := 0; i <= cfg.MaxRetries; i++ {
		result, err, retryable = op(ctx)
		if err == nil {
			return result, nil
		}

		if !retryable {
			return empty, err
		}

		if i == cfg.MaxRetries {
			break
		}

		delay := calculateDelay(i, cfg.BaseDelay, cfg.MaxDelay)
		
		select {
		case <-ctx.Done():
			return empty, ctx.Err()
		case <-time.After(delay):
			continue
		}
	}

	return empty, err
}

// calculateDelay calculates the delay for the next retry using exponential backoff with jitter.
func calculateDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	exp := math.Pow(2, float64(attempt))
	delay := float64(baseDelay) * exp
	
	// Add jitter (0-20%)
	jitter := (rand.Float64() * 0.2) + 0.9
	delay = delay * jitter

	if delay > float64(maxDelay) {
		return maxDelay
	}
	return time.Duration(delay)
}