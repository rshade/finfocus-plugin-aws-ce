# Retry and backoff

## Purpose

Describe tested retry decisions and delay bounds for Cost Explorer calls.

## Requirements

### Requirement: Retry only retryable operations

The Cost Explorer client SHALL retry retryable failures up to the configured
limit, return non-retryable failures after one attempt, and stop when the context
is canceled. Covered by `TestWithRetry_RetryableSuccess`,
`TestWithRetry_MaxRetriesExhausted`, `TestWithRetry_NonRetryableError`, and
`TestWithRetry_ContextCancelled` in `internal/client/retry_test.go`.

#### Scenario: Retryable failure recovers

- **WHEN** an operation reports retryable failures before succeeding
- **THEN** the client returns the successful result within the retry limit

#### Scenario: Failure cannot be retried

- **WHEN** an operation reports a non-retryable failure
- **THEN** the client returns that failure without another attempt

### Requirement: Bound retry delay

The retry helper SHALL use exponential delays with jitter and cap them at the
configured maximum. Covered by `TestCalculateDelay` in
`internal/client/retry_test.go`.

#### Scenario: Delay reaches the cap

- **WHEN** the retry attempt would exceed the maximum delay
- **THEN** the calculated delay equals the maximum
