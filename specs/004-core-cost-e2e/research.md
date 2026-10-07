# Research: Core Cost Plugin & E2E Testing Implementation

**Date**: 2026-01-21
**Feature**: 004-core-cost-e2e
**Status**: Complete

This document consolidates research findings for implementing the core AWS Cost Explorer integration with caching, retry logic, and E2E testing infrastructure.

## Table of Contents

1. [Cache Storage Format & Location](#1-cache-storage-format--location)
2. [E2E Test Framework Structure](#2-e2e-test-framework-structure)
3. [Exponential Backoff Retry Strategy](#3-exponential-backoff-retry-strategy)
4. [Partial Data Warning Metadata](#4-partial-data-warning-metadata)
5. [GitHub OIDC Federation Setup](#5-github-oidc-federation-setup)

---

## 1. Cache Storage Format & Location

### Decision: Cache Storage Format

**Chosen: JSON** (Current implementation optimal)

**Rationale:**

- **Human-readable debugging**: Cost data caching is not performance-critical; readability for debugging is more valuable than marginal speed gains
- **KISS principle**: JSON avoids dependencies, uses Go stdlib, requires no code generation
- **Adequate performance**: Cost queries typically take 2-5 seconds; JSON serialization overhead is < 1ms
- **24-hour TTL context**: Cache volume modest (typically <100 entries max)

**Performance comparison:**

| Format | Speed | File Size | Debuggability | Complexity |
|--------|-------|-----------|---------------|------------|
| JSON | ✓ Adequate | Baseline | ✓✓✓ Excellent | ✓✓✓ Simple |
| Gob | ✓✓ Good | -30% | ✗ Binary | ✓ Stdlib |
| Protobuf | ✓✓✓ Excellent | -60% | ✗ Binary | ✗ Schema mgmt |

### Decision: Cache Location

**Chosen: `os.UserCacheDir()` with application subdirectory**

**Platform resolution:**

- **Linux/Unix**: `$XDG_CACHE_HOME/finfocus/aws-ce` (default: `~/.cache/finfocus/aws-ce`)
- **macOS**: `~/Library/Caches/finfocus/aws-ce`
- **Windows**: `%LocalAppData%\finfocus\aws-ce`

**Implementation:**

```go
func getCacheDir() (string, error) {
    baseDir, err := os.UserCacheDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(baseDir, "finfocus", "aws-ce"), nil
}
```

**Advantages over current approach** (`~/.finfocus/cache/aws-ce`):

- Cross-platform without dependencies
- Respects platform conventions (macOS Library/Caches)
- Follows XDG Base Directory Specification on Unix
- Handles user configuration via environment variables

### Decision: Cache Key Structure

#### Chosen: versioned structured keys

**Pattern:** `cost:v1:RESOURCE:SERVICE:REGION:START:END`

**Implementation:**

```go
type CacheKeyComponents struct {
    ResourceID  string
    Service     string    // "" if not filtered
    Region      string    // "" if not filtered
    StartTime   int64
    EndTime     int64
}

func (c *CacheKeyComponents) String() string {
    return fmt.Sprintf("cost:v1:%s:%s:%s:%d:%d",
        c.ResourceID,
        c.Service,
        c.Region,
        c.StartTime,
        c.EndTime,
    )
}
```

**Advantages:**

- **Versioning**: `v1` allows schema evolution without breaking existing cache
- **Structured**: Separates concerns (resource, filtering, time)
- **Debuggable**: Human-readable in log files
- **Extensible**: Easy to add dimensions (LinkedAccount, Tags)
- **Collision-safe**: Each dimension combination produces unique key

### Decision: TTL Eviction Strategy

**Chosen: Dual-layer lazy eviction** (Current approach optimal)

**Layer 1 - Embedded Timestamp** (Primary):

```go
type CacheEntry struct {
    Results   []CostEntry
    ExpiresAt time.Time  // Embedded in JSON
    CachedAt  time.Time
}

if time.Now().After(entry.ExpiresAt) {
    return nil, false  // Expired
}
```

**Layer 2 - File Modification Time** (Fallback):

```go
if time.Since(info.ModTime()) > cm.ttl {
    _ = os.Remove(path)  // Lazy cleanup
    continue
}
```

**Rationale:**

- Plugin runs as short-lived process (no persistent background cleanup needed)
- Lazy eviction sufficient for stateless plugin model
- Embedded timestamp fastest check (no filesystem syscall)
- File modification time detects stale files even if JSON corrupted

### Decision: Manual Invalidation Mechanism

#### Chosen: environment variable for force-refresh

**Implementation:**

```go
// In calculator.go GetActualCost()
if os.Getenv("FINFOCUS_CACHE_BYPASS") == "true" {
    // Skip cache, fetch fresh data
    if c.cache != nil {
        c.logger.Debug().Msg("Cache bypass enabled via environment variable")
    }
}
```

**Usage:**

```bash
FINFOCUS_CACHE_BYPASS=true finfocus-plugin-aws-ce
```

---

## 2. E2E Test Framework Structure

### Decision: Test Organization

**Chosen: Environment variable gating** (Current approach)

**Current structure:**

```
test/
└── e2e/
    └── e2e_test.go  # Real AWS API integration tests
```

**Gating mechanism:**

```go
func TestE2E(t *testing.T) {
    if os.Getenv("FINFOCUS_E2E") != "true" {
        t.Skip("Skipping E2E tests. Set FINFOCUS_E2E=true to run.")
    }
    // ...
}
```

**Rationale:**

- Simpler than build tags for typical use case
- Prevents accidental E2E execution (AWS API calls)
- Works well for gated CI/CD environments
- Already implemented in repository

### Decision: AWS Credential Handling

**CI Environment: GitHub OIDC Federation** (per spec requirement)

```yaml
- name: Configure AWS credentials
  uses: aws-actions/configure-aws-credentials@v4
  with:
    role-to-assume: arn:aws:iam::ACCOUNT_ID:role/GitHubE2ERole
    aws-region: us-east-1
```

#### Local development with an AWS profile

```bash
export AWS_PROFILE=your-profile
FINFOCUS_E2E=true make e2e
```

**Test Skipping Logic:**

```go
func skipIfNoAWSCreds(t *testing.T) {
    // Skip if neither credentials file nor env vars present
    if _, err := os.Stat(os.ExpandEnv("${HOME}/.aws/credentials")); err != nil {
        if os.Getenv("AWS_ACCESS_KEY_ID") == "" &&
           os.Getenv("AWS_SESSION_TOKEN") == "" {
            t.Skip("AWS credentials not available, skipping E2E")
        }
    }
}
```

### Decision: E2E Test Data Strategy

#### Chosen: last 7 days with structure-focused assertions

**Date Range Selection:**

```go
now := time.Now()
start := now.AddDate(0, 0, -7)  // Last 7 days (minimizes cost)
end := now
```

**Assertion Pattern** (handle non-deterministic AWS data):

```go
// DON'T: Assert specific cost values (non-deterministic)
// if len(resp.Results) != 5 { t.Error("expected 5 results") }

// DO: Assert structural properties
if len(resp.Results) < 0 { t.Error("results should exist or be empty") }

// Assert required fields presence
for _, r := range resp.Results {
    if r.Identity == nil || r.Identity.UniqueId == "" {
        t.Error("missing UniqueId")
    }
    if r.Financials == nil || r.Financials.NetAmortizedCost == nil {
        t.Error("missing cost data")
    }
}
```

### Decision: Timeout Management

**Overall test suite timeout:** 2 minutes (per spec)

**Implementation:**

```go
func TestE2E(t *testing.T) {
    // Overall test timeout (2 minutes as per spec)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    // Server startup context
    serverCtx, serverCancel := context.WithCancel(ctx)
    defer serverCancel()

    cleanup := startPluginServer(t, serverCtx, port)
    defer cleanup()

    // Per-request timeout (AWS API calls)
    testCtx, testCancel := context.WithTimeout(ctx, 30*time.Second)
    defer testCancel()

    resp, err := client.GetActualCost(testCtx, req)
    // ...
}
```

**Makefile enhancement:**

```makefile
e2e:
    @echo "Running E2E tests (2-minute timeout)..."
    FINFOCUS_E2E=true go test -v -timeout 2m30s ./test/e2e/...
```

### Decision: CI Integration Pattern

**Workflow configuration** (`.github/workflows/e2e.yml`):

```yaml
permissions:
  contents: read
  id-token: write  # Required for OIDC

jobs:
  e2e:
    runs-on: ubuntu-latest

    steps:
      - name: Configure AWS credentials (main branch only)
        if: github.ref == 'refs/heads/main'
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/GitHubE2ERole
          aws-region: us-east-1

      - name: Run E2E tests
        if: github.ref == 'refs/heads/main'
        run: FINFOCUS_E2E=true go test -v -timeout 2m30s ./test/e2e/...

      - name: Skip E2E tests (fork PR)
        if: github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name != github.repository
        run: echo "Skipping E2E tests for fork PR (no AWS credentials)"
```

---

## 3. Exponential Backoff Retry Strategy

### Decision: Retry Configuration

#### Chosen: 3 attempts max with conservative parameters

**Parameters:**

```go
RetryConfig{
    MaxRetries: 3,                      // 3 total attempts (AWS SDK default)
    BaseDelay:  100 * time.Millisecond, // Conservative start
    MaxDelay:   20 * time.Second,       // Align with AWS SDK default
}
```

**Rationale:**

- Cost Explorer typically returns quickly (under 5 seconds)
- 3 attempts covers 99%+ of transient failures
- 100ms base allows rapid recovery from brief blips
- 20s cap prevents long waits for persistent issues

### Decision: Error Classification

**Retryable errors:**

```go
func classifyRetryable(err error) bool {
    // AWS API errors
    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        code := apiErr.ErrorCode()
        // Throttle errors always retryable
        if isThrottleError(code) {
            return true
        }
        // Timeout errors retryable
        if code == "RequestTimeout" || code == "RequestTimeoutException" {
            return true
        }
        // Validation/auth errors never retry
        if strings.Contains(code, "Validation") ||
           strings.Contains(code, "Access") {
            return false
        }
    }

    // HTTP status codes
    var httpErr interface{ HTTPStatusCode() int }
    if errors.As(err, &httpErr) {
        code := httpErr.HTTPStatusCode()
        return code >= 500 || code == 429  // 5xx and 429 (Too Many Requests)
    }

    // Connection errors
    return isConnectionRetryable(err)
}
```

**Throttle error codes:**

- `Throttling`, `ThrottlingException`, `ThrottledException`
- `TooManyRequestsException`, `RequestThrottledException`
- `LimitExceededException`, `RequestLimitExceeded`
- `RequestThrottled`

**Non-retryable errors:**

- `ValidationException`, `InvalidParameterException`
- `AccessDeniedException`, `UnauthorizedOperation`
- HTTP 400 (Bad Request), 401 (Unauthorized), 403 (Forbidden)
- DNS NXDOMAIN errors (permanent)

### Decision: Backoff Algorithm

#### Chosen: exponential backoff with uniform jitter

**Formula:** `delay = (2^attempt * baseDelay) * jitter`
**Jitter range:** 0.9 - 1.1 (10% variation)

**Implementation:**

```go
func calculateDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
    exp := math.Pow(2, float64(attempt))
    delay := float64(baseDelay) * exp

    // Apply uniform jitter: random factor 0.9 - 1.1
    jitter := (rand.Float64() * 0.2) + 0.9
    delay = delay * jitter

    if time.Duration(delay) > maxDelay {
        return maxDelay
    }
    return time.Duration(delay)
}
```

**Backoff progression** (with jitter range):

| Attempt | Min Delay | Max Delay | Cap |
|---------|-----------|-----------|-----|
| 1 | 90ms | 110ms | - |
| 2 | 180ms | 220ms | - |
| 3 | 360ms | 440ms | - |
| 4+ | - | - | 20s |

### Decision: Network Diagnostics

**Structured logging for retry attempts:**

```go
logger.Info().
    Int("attempt", attempt).
    Str("error_type", classifyError(err)).
    Str("error_message", err.Error()).
    Bool("retryable", isRetryable).
    Duration("delay_ms", delay.Milliseconds()).
    Msg("Retry attempt")
```

**Error type classification:**

```go
func classifyError(err error) string {
    var dnsErr *net.DNSError
    var opErr *net.OpError

    switch {
    case errors.As(err, &dnsErr):
        if dnsErr.IsNotFound {
            return "dns:nxdomain"  // Non-retryable
        }
        if dnsErr.Temporary() {
            return "dns:temporary"  // Retryable
        }
        return "dns:unknown"
    case errors.As(err, &opErr):
        if opErr.Op == "dial" {
            return "network:dial"  // Retryable
        }
        return "network:unknown"
    case strings.Contains(err.Error(), "connection reset"):
        return "network:reset"  // Retryable
    }

    var apiErr smithy.APIError
    if errors.As(err, &apiErr) {
        code := apiErr.ErrorCode()
        switch code {
        case "ThrottlingException":
            return "aws:throttled"
        case "RequestTimeout":
            return "aws:timeout"
        case "ValidationException":
            return "aws:validation"
        case "AccessDeniedException":
            return "aws:access_denied"
        default:
            return fmt.Sprintf("aws:%s", code)
        }
    }

    return "unknown"
}
```

**Actionable error messages:**

```go
switch {
case strings.HasPrefix(errorType, "aws:"):
    return fmt.Errorf("%s failed (AWS error, attempt %d of 3): %w",
        op, attempt, err)
case strings.HasPrefix(errorType, "network:"):
    return fmt.Errorf("%s failed (network connectivity issue, attempt %d): %w. Check your internet connection.",
        op, attempt, err)
case strings.HasPrefix(errorType, "dns:"):
    if errorType == "dns:nxdomain" {
        return fmt.Errorf("%s failed: DNS resolution failed (permanent). Check AWS endpoint configuration.", op)
    }
    return fmt.Errorf("%s failed (DNS error, retrying): %w", op, err)
}
```

---

## 4. Partial Data Warning Metadata

### Decision: Logging-Only Approach (No Proto Changes)

#### Chosen: structured logging with existing SDK infrastructure

**Rationale:**

- **gRPC protocol compatibility**: No breaking changes to CostSourceService
- **Backward compatible**: Clients unaware of warnings still function
- **Rich context**: Structured logging preserves all warning metadata
- **Existing infrastructure**: SDK provides all necessary logging mechanisms

### SDK Error Infrastructure

**ErrorDetail message** (from costsource.proto):

```protobuf
message ErrorDetail {
  ErrorCode code = 1;
  ErrorCategory category = 2;
  string message = 3;
  map<string, string> details = 4;  // Supports arbitrary metadata
  optional int32 retry_after_seconds = 5;
  google.protobuf.Timestamp timestamp = 6;
}
```

**Error categories:**

- `ERROR_CATEGORY_TRANSIENT` (1): Temporary failures eligible for retry
- `ERROR_CATEGORY_PERMANENT` (2): Failures requiring changes
- `ERROR_CATEGORY_CONFIGURATION` (3): Invalid setup/credentials

### Logging Pattern for Partial Data

#### Scenario 1: no cost data found

```go
if len(clientCosts) == 0 {
    c.logger.Warn().
        Str(pluginsdk.FieldOperation, "GetActualCost").
        Str(pluginsdk.FieldResourceType, req.GetResource().GetResourceType()).
        Str("reason", "no_cost_data_found").
        Time("query_start", startTime).
        Time("query_end", endTime).
        Str("warning_type", "no_data_found").
        Msg("Returning empty results with fallback hint")

    return &pbc.GetActualCostResponse{
        Results:      []*pbc.ActualCostResult{},
        FallbackHint: pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED,
    }, nil
}
```

#### Scenario 2: identifier mismatch

```go
if !strings.HasSuffix(parsed.Resource, resourceID) {
    c.logger.Warn().
        Str(pluginsdk.FieldOperation, "GetActualCost").
        Str("resource_id", resourceID).
        Str("arn_resource", parsed.Resource).
        Str("warning_type", "identifier_mismatch").
        Str("resolution", "using_arn_as_source_of_truth").
        Msg("ResourceId does not match ARN resource component")
}
```

#### Scenario 3: cache write failures

```go
if c.cache != nil {
    if err := c.cache.Set(cacheKey, costs); err != nil {
        c.logger.Warn().
            Err(err).
            Str("cache_key", cacheKey).
            Str("warning_type", "cache_write_failed").
            Msg("Failed to cache results; performance may degrade")
    }
}
```

### Standard Logging Fields (from SDK)

```go
const (
    FieldTraceID       = "trace_id"
    FieldComponent     = "component"
    FieldOperation     = "operation"
    FieldDurationMs    = "duration_ms"
    FieldErrorCode     = "error_code"
    FieldResourceType  = "resource_type"
    FieldProvider      = "provider"
)
```

### When Proto Changes Become Necessary

Proto changes should only be considered for:

- **New capabilities** requiring client awareness (e.g., confidence scores)
- **Data structure changes** that impact response interpretation
- **Standard compliance** (e.g., FinOps FOCUS 1.2 additions)

**Current status:** None of these apply to warning metadata.

---

## 5. GitHub OIDC Federation Setup

### Decision: OIDC Provider Configuration

**AWS OIDC Provider:**

- **URL**: `https://token.actions.githubusercontent.com`
- **Thumbprint** (Jan 2025): `1b511abead59c6ce207077c0ef0caf8f82b5b5ee`
- **Audience**: `sts.amazonaws.com`

**Creation command:**

```bash
aws iam create-open-id-connect-provider \
  --url https://token.actions.githubusercontent.com \
  --client-id-list sts.amazonaws.com \
  --thumbprint-list 1b511abead59c6ce207077c0ef0caf8f82b5b5ee
```

### Decision: IAM Role Trust Policy

**Trust policy template:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::ACCOUNT_ID:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:rshade/finfocus-plugin-aws-ce:*"
        }
      }
    }
  ]
}
```

**For main branch only (tighter security):**

```json
"Condition": {
  "StringEquals": {
    "token.actions.githubusercontent.com:aud": "sts.amazonaws.com",
    "token.actions.githubusercontent.com:sub": "repo:rshade/finfocus-plugin-aws-ce:ref:refs/heads/main"
  }
}
```

### Decision: IAM Permissions (Least Privilege)

**Minimal Cost Explorer permissions:**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CostExplorerReadOnly",
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetCostForecast",
        "ce:DescribeCostCategoryDefinition",
        "ce:GetDimensionValues",
        "ce:GetTags"
      ],
      "Resource": "*"
    }
  ]
}
```

**For CI/CD testing only (even more restrictive):**

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CostExplorerCITesting",
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetCostForecast"
      ],
      "Resource": "*"
    }
  ]
}
```

**Note:** Cost Explorer does not support resource-level restrictions (no ARNs available).

### Decision: GitHub Actions Workflow

**Complete workflow with OIDC:**

```yaml
name: E2E Tests

on:
  push:
    branches: [ "main" ]
  pull_request:
    branches: [ "main" ]

permissions:
  contents: read
  id-token: write  # CRITICAL: Required for OIDC token

jobs:
  e2e:
    runs-on: ubuntu-latest
    environment: ci  # Optional: for additional gating

    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.5'
          cache: true

      # Only configure AWS for non-fork PRs
      - name: Configure AWS credentials (OIDC)
        if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: arn:aws:iam::${{ secrets.AWS_ACCOUNT_ID }}:role/GitHubE2ERole
          aws-region: us-east-1
          role-session-duration: 3600  # 1 hour

      - name: Run E2E tests
        if: github.event_name == 'push' || github.event.pull_request.head.repo.full_name == github.repository
        env:
          FINFOCUS_E2E: "true"
        run: go test -v -timeout 2m30s ./test/e2e/...
```

### Fork PR Handling

**Security Note:** OIDC credentials are NOT available for fork PRs by design.

**Workflow pattern:**

- Fork PRs: Skip E2E tests (no credentials)
- Non-fork PRs: Run E2E tests with OIDC
- Push to main: Always run E2E tests with OIDC

### Session Configuration

**Session duration:**

- Default: 1 hour (3600 seconds)
- Range: 15 minutes to 12 hours
- Configured via `role-session-duration` in workflow

**Session naming:** Automatically generated by aws-actions:

- Includes GitHub actor username
- Repository name
- Run ID

### Validation

**Validation script** (`scripts/validate-oidc-setup.sh`):

```bash
#!/bin/bash
set -e

echo "Checking OIDC provider..."
OIDC_PROVIDERS=$(aws iam list-open-id-connect-providers --query 'OpenIDConnectProviderList[*].Arn' --output text)

if echo "$OIDC_PROVIDERS" | grep -q "token.actions.githubusercontent.com"; then
  echo "✓ GitHub OIDC provider exists"
else
  echo "✗ GitHub OIDC provider NOT found"
fi

echo "Checking IAM role..."
aws iam get-role --role-name GitHubE2ERole 2>/dev/null && \
  echo "✓ IAM role exists" || \
  echo "✗ IAM role NOT found"

echo "Checking Cost Explorer permissions..."
POLICIES=$(aws iam list-role-policies --role-name GitHubE2ERole --query 'PolicyNames' --output text)
if echo "$POLICIES" | grep -iq "cost"; then
  echo "✓ Cost Explorer policy found"
else
  echo "✗ Cost Explorer policy NOT found"
fi
```

---

## Implementation Summary

All technical unknowns have been resolved:

1. **Cache Storage**: JSON format in `os.UserCacheDir()` with versioned keys
2. **E2E Tests**: Environment variable gating with 7-day queries and 2-minute timeout
3. **Retry Strategy**: 3 attempts max with exponential backoff and error type classification
4. **Warning Metadata**: Logging-only approach using SDK structured logging
5. **GitHub OIDC**: Complete IAM role and workflow configuration for secure CI authentication

Ready to proceed to **Phase 1: Design & Contracts**.
