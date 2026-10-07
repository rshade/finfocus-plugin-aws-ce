# Data Model: Core Cost Plugin & E2E Testing

**Feature**: 004-core-cost-e2e
**Date**: 2026-01-21
**Status**: Design

This document defines the data structures for caching, retry logic, and AWS Cost Explorer integration.

## Table of Contents

1. [Cache Entry Structure](#1-cache-entry-structure)
2. [AWS Cost Explorer Request/Response Mappings](#2-aws-cost-explorer-requestresponse-mappings)
3. [Retry State Machine](#3-retry-state-machine)
4. [Warning Metadata Model](#4-warning-metadata-model)

---

## 1. Cache Entry Structure

### 1.1 CacheEntry

**Purpose**: In-memory and disk-persisted representation of cached cost data.

**Structure:**

```go
type CacheEntry struct {
    // Results from AWS Cost Explorer API
    Results   []CostEntry `json:"results"`

    // Cache metadata
    CachedAt  time.Time `json:"cached_at"`
    ExpiresAt time.Time `json:"expires_at"`

    // Query context (for debugging/validation)
    QueryMeta CacheQueryMeta `json:"query_meta"`
}

type CostEntry struct {
    ResourceID string    `json:"resource_id"`
    StartTime  time.Time `json:"start_time"`
    EndTime    time.Time `json:"end_time"`
    Cost       float64   `json:"cost"`
    Unit       string    `json:"unit"`  // "USD"
    Service    string    `json:"service,omitempty"`
    Region     string    `json:"region,omitempty"`
}

type CacheQueryMeta struct {
    ResourceID string   `json:"resource_id"`
    ARN        string   `json:"arn,omitempty"`
    StartTime  int64    `json:"start_time"`  // Unix seconds
    EndTime    int64    `json:"end_time"`    // Unix seconds
    Dimensions []string `json:"dimensions,omitempty"`
}
```

**Validation rules:**

- `Results` may be empty (no cost data available)
- `ExpiresAt` must be > `CachedAt`
- `CachedAt` must be <= current time
- `QueryMeta.StartTime` must be < `QueryMeta.EndTime`

**State transitions:**

```
[Fresh] → (time > ExpiresAt) → [Expired]
[Expired] → (lazy eviction) → [Deleted]
[Any] → (FINFOCUS_CACHE_BYPASS=true) → [Bypassed]
```

### 1.2 CacheKeyComponents

**Purpose**: Structured cache key for multi-dimensional cost queries.

**Structure:**

```go
type CacheKeyComponents struct {
    ResourceID  string
    Service     string  // "" if not filtered
    Region      string  // "" if not filtered
    StartTime   int64   // Unix seconds
    EndTime     int64   // Unix seconds
}

func (c *CacheKeyComponents) String() string {
    return fmt.Sprintf("cost:v1:%s:%s:%s:%d:%d",
        c.ResourceID,
        valueOrEmpty(c.Service),
        valueOrEmpty(c.Region),
        c.StartTime,
        c.EndTime,
    )
}

func valueOrEmpty(s string) string {
    if s == "" {
        return "_"  // Placeholder for empty dimension
    }
    return s
}
```

**Key format examples:**

- `cost:v1:i-12345:_:_:1704067200:1705276800` (no filters)
- `cost:v1:i-12345:AmazonEC2:us-east-1:1704067200:1705276800` (service + region)

**Collision prevention:**

- Version prefix (`v1`) enables schema evolution
- Underscore (`_`) distinguishes empty from set values
- Unix timestamps ensure deterministic ordering

### 1.3 Cache File Structure

**Disk layout:**

```
~/.cache/finfocus/aws-ce/
├── cost_v1_i-12345___1704067200_1705276800.json
├── cost_v1_i-67890_AmazonEC2_us-east-1_1704067200_1705276800.json
└── ...
```

**File naming:**

- Derived from `CacheKeyComponents.String()`
- Replace `:` with `_` for filesystem compatibility
- Extension: `.json`

**JSON format example:**

```json
{
  "results": [
    {
      "resource_id": "i-12345",
      "start_time": "2024-01-01T00:00:00Z",
      "end_time": "2024-01-02T00:00:00Z",
      "cost": 2.45,
      "unit": "USD",
      "service": "AmazonEC2",
      "region": "us-east-1"
    }
  ],
  "cached_at": "2024-01-15T10:30:00Z",
  "expires_at": "2024-01-16T10:30:00Z",
  "query_meta": {
    "resource_id": "i-12345",
    "arn": "arn:aws:ec2:us-east-1:123456789012:instance/i-12345",
    "start_time": 1704067200,
    "end_time": 1705276800,
    "dimensions": ["SERVICE", "REGION"]
  }
}
```

---

## 2. AWS Cost Explorer Request/Response Mappings

### 2.1 GetActualCost Request Flow

```
PulumiCost Core
    ↓
    └→ GetActualCostRequest (gRPC)
         ├─ ResourceDescriptor
         │    ├─ ResourceId: "i-12345"
         │    ├─ Arn: "arn:aws:ec2:us-east-1:123456789012:instance/i-12345"
         │    └─ ResourceType: "aws.ec2.Instance"
         ├─ Start: timestamp
         └─ End: timestamp
    ↓
Plugin Calculator
    ├─ Check cache (CacheKey)
    │    ├─ Hit → Return cached results
    │    └─ Miss → Continue
    ↓
AWS Cost Explorer Client (with retry)
    └→ ce.GetCostAndUsage
         ├─ TimePeriod: {Start, End}
         ├─ Granularity: DAILY
         ├─ Metrics: ["UnblendedCost"]
         └─ Filter: ResourceId filter expression
    ↓
AWS Cost Explorer API
    ← ce.GetCostAndUsageOutput
         └─ ResultsByTime[]
              ├─ TimePeriod: {Start, End}
              ├─ Total: {"UnblendedCost": {Amount, Unit}}
              └─ Groups[] (if grouped by dimensions)
    ↓
Plugin Calculator
    ├─ Transform to FOCUS 1.2 records
    ├─ Cache results (24h TTL)
    └→ GetActualCostResponse (gRPC)
         ├─ Results: ActualCostResult[]
         │    ├─ Identity: {UniqueId, ResourceId}
         │    └─ Financials: {NetAmortizedCost}
         └─ FallbackHint: FALLBACK_HINT_NONE | FALLBACK_HINT_RECOMMENDED
```

### 2.2 AWS Cost Explorer Filter Expressions

**By ResourceId:**

```go
filter := &types.Expression{
    Dimensions: &types.DimensionValues{
        Key: types.DimensionResourceId,
        Values: []string{resourceID},
    },
}
```

**By ARN (when available):**

```go
// Parse ARN to extract Resource ID
parsed, _ := arn.Parse(req.GetArn())
resourceID := parsed.Resource  // "instance/i-12345" → "i-12345"
```

**By Service + Region:**

```go
filter := &types.Expression{
    And: []*types.Expression{
        {
            Dimensions: &types.DimensionValues{
                Key: types.DimensionService,
                Values: []string{"Amazon Elastic Compute Cloud - Compute"},
            },
        },
        {
            Dimensions: &types.DimensionValues{
                Key: types.DimensionRegion,
                Values: []string{"us-east-1"},
            },
        },
    },
}
```

### 2.3 Transformation: AWS CE → FOCUS 1.2

**AWS Cost Explorer Output:**

```go
type ResultByTime struct {
    TimePeriod struct {
        Start string  // "2024-01-01"
        End   string  // "2024-01-02"
    }
    Total map[string]struct {
        Amount string  // "2.45"
        Unit   string  // "USD"
    }
    Groups []Group  // Optional: when grouped by dimensions
}
```

**FOCUS 1.2 Record (via pluginsdk):**

```go
record := pluginsdk.NewFocusRecordBuilder().
    WithIdentity(&pbc.Identity{
        UniqueId:   fmt.Sprintf("%s:%s:%s", resourceID, startTime, endTime),
        ResourceId: resourceID,
    }).
    WithFinancials(&pbc.Financials{
        NetAmortizedCost: &pbc.Amount{
            Value:    amount,  // float64 from AWS "Amount" string
            Currency: "USD",
        },
    }).
    Build()
```

**Mapping rules:**

| AWS Field | FOCUS Field | Transformation |
|-----------|-------------|----------------|
| `ResultByTime.Total["UnblendedCost"].Amount` | `Financials.NetAmortizedCost.Value` | `strconv.ParseFloat()` |
| `ResultByTime.Total["UnblendedCost"].Unit` | `Financials.NetAmortizedCost.Currency` | Direct copy |
| `ResultByTime.TimePeriod.Start` | `Identity.UniqueId` | Included in composite key |
| `ResourceId` (from filter) | `Identity.ResourceId` | Direct copy |

---

## 3. Retry State Machine

### 3.1 RetryConfig

**Purpose**: Configuration for exponential backoff retry strategy.

**Structure:**

```go
type RetryConfig struct {
    MaxRetries int           // Maximum retry attempts (default: 3)
    BaseDelay  time.Duration // Initial delay (default: 100ms)
    MaxDelay   time.Duration // Maximum delay cap (default: 20s)
}

func DefaultRetryConfig() RetryConfig {
    return RetryConfig{
        MaxRetries: 3,
        BaseDelay:  100 * time.Millisecond,
        MaxDelay:   20 * time.Second,
    }
}
```

### 3.2 Retry State Transitions

```
[Initial]
    ↓
    └→ Attempt 1
         ├─ Success → [Complete]
         └─ Failure → Classify error
              ├─ Non-retryable (auth, validation) → [Failed]
              └─ Retryable (throttle, network) → Delay → Attempt 2
                   ├─ Success → [Complete]
                   └─ Failure → Classify error
                        ├─ Non-retryable → [Failed]
                        └─ Retryable → Delay → Attempt 3
                             ├─ Success → [Complete]
                             └─ Failure → [MaxRetriesExceeded]
```

**State definitions:**

- **Initial**: First attempt, no delay
- **Delay**: Exponential backoff sleep before next attempt
- **Complete**: Operation succeeded
- **Failed**: Non-retryable error encountered
- **MaxRetriesExceeded**: All retry attempts exhausted

### 3.3 Error Classification

**Retryable errors:**

| Category | Examples | Rationale |
|----------|----------|-----------|
| Throttling | `ThrottlingException`, `LimitExceededException` | Rate limit, will clear |
| Network | `connection reset`, `dial timeout` | Transient network issues |
| Service | HTTP 500, 502, 503, 504 | AWS infrastructure issues |
| Timeout | `RequestTimeout`, `context deadline exceeded` | May succeed if retried |

**Non-retryable errors:**

| Category | Examples | Rationale |
|----------|----------|-----------|
| Validation | `ValidationException`, `InvalidParameterException` | Request malformed |
| Authorization | `AccessDeniedException`, `UnauthorizedOperation` | Credentials invalid |
| Client | HTTP 400, 401, 403 | Client-side error |
| DNS | `DNS NXDOMAIN` | Permanent resolution failure |

### 3.4 Delay Calculation

**Formula:** `delay = min((2^attempt * baseDelay) * jitter, maxDelay)`

**Jitter:** Uniform random [0.9, 1.1] (10% variation)

**Example progression:**

| Attempt | Base Delay | With Jitter (Min-Max) | Actual |
|---------|------------|-----------------------|--------|
| 1 | 100ms | 90ms - 110ms | ~100ms |
| 2 | 200ms | 180ms - 220ms | ~200ms |
| 3 | 400ms | 360ms - 440ms | ~400ms |
| 4+ | (capped at 20s) | 18s - 20s | 20s |

---

## 4. Warning Metadata Model

### 4.1 Warning Types

**Purpose**: Structured logging for non-fatal issues that don't warrant gRPC errors.

**Warning taxonomy:**

```go
const (
    WarningTypeNoDataFound         = "no_data_found"
    WarningTypeIdentifierMismatch  = "identifier_mismatch"
    WarningTypeCacheWriteFailed    = "cache_write_failed"
    WarningTypeCacheReadFailed     = "cache_read_failed"
    WarningTypePartialResults      = "partial_results"
)
```

### 4.2 Structured Log Format

**Schema:**

```go
type WarningLog struct {
    Level         string            // "warn"
    Timestamp     time.Time         // RFC3339
    Message       string            // Human-readable description
    WarningType   string            // From taxonomy above
    Operation     string            // "GetActualCost", "GetServiceActualCost"
    ResourceType  string            // "aws.ec2.Instance"
    ResourceID    string            // "i-12345"
    Context       map[string]string // Additional structured data
}
```

**Example JSON output:**

```json
{
  "level": "warn",
  "timestamp": "2024-01-15T10:30:00Z",
  "message": "ResourceId does not match ARN resource component",
  "warning_type": "identifier_mismatch",
  "operation": "GetActualCost",
  "resource_type": "aws.ec2.Instance",
  "resource_id": "i-12345",
  "context": {
    "arn_resource": "i-67890",
    "resolution": "using_arn_as_source_of_truth"
  }
}
```

### 4.3 Warning Scenarios

#### Scenario 1: No data found

```go
c.logger.Warn().
    Str(pluginsdk.FieldOperation, "GetActualCost").
    Str(pluginsdk.FieldResourceType, req.GetResource().GetResourceType()).
    Str("warning_type", WarningTypeNoDataFound).
    Time("query_start", startTime).
    Time("query_end", endTime).
    Msg("No cost data returned from AWS Cost Explorer")

// Response: Empty results with fallback hint
return &pbc.GetActualCostResponse{
    Results:      []*pbc.ActualCostResult{},
    FallbackHint: pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED,
}, nil
```

#### Scenario 2: Identifier mismatch

```go
c.logger.Warn().
    Str(pluginsdk.FieldOperation, "GetActualCost").
    Str("resource_id", resourceID).
    Str("arn_resource", parsed.Resource).
    Str("warning_type", WarningTypeIdentifierMismatch).
    Str("resolution", "using_arn_as_source_of_truth").
    Msg("ResourceId does not match ARN resource component")

// Continue with ARN-based resource ID
```

#### Scenario 3: Cache write failure

```go
if err := c.cache.Set(cacheKey, costs); err != nil {
    c.logger.Warn().
        Err(err).
        Str("cache_key", cacheKey).
        Str("warning_type", WarningTypeCacheWriteFailed).
        Msg("Failed to cache results; performance may degrade")
}

// Continue returning results (cache is best-effort)
```

### 4.4 FallbackHint Usage

**When to use fallback hints:**

| Hint | Scenario | User Impact |
|------|----------|-------------|
| `FALLBACK_HINT_NONE` | Normal successful response | None - data complete |
| `FALLBACK_HINT_RECOMMENDED` | No data found, empty results | PulumiCost core should try other plugins |
| `FALLBACK_HINT_REQUIRED` | (Not used by this plugin) | Reserved for critical failures |

**Response pattern:**

```go
// Empty results (no cost data available)
return &pbc.GetActualCostResponse{
    Results:      []*pbc.ActualCostResult{},
    FallbackHint: pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED,
}, nil

// Partial results (some data missing)
return &pbc.GetActualCostResponse{
    Results:      partialResults,  // Non-empty but incomplete
    FallbackHint: pbc.FallbackHint_FALLBACK_HINT_NONE,  // Don't cascade
}, nil
```

---

## Entity Relationships

```
GetActualCostRequest
    ↓
    ├─→ CacheKeyComponents → CacheEntry (if hit)
    │        ↓
    │        └─→ []CostEntry → GetActualCostResponse
    │
    └─→ (cache miss) → AWS CE Client
             ↓ (with RetryConfig)
             ├─→ ce.GetCostAndUsageOutput
             │        ↓
             │        └─→ Transform to []CostEntry
             │                 ↓
             │                 ├─→ Cache (CacheEntry)
             │                 └─→ GetActualCostResponse
             │
             └─→ (retry failures) → WarningLog → GetActualCostResponse (error or empty)
```

---

## Summary

All data structures maintain **backward compatibility** with existing gRPC protocol while adding:

- **Robust caching** with TTL and manual invalidation
- **Resilient retries** with error classification
- **Observability** through structured warning metadata

No proto changes required - all enhancements leverage existing SDK infrastructure.
