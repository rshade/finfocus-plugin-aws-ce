# Contract: Cache Invalidation API

**Feature**: 004-core-cost-e2e
**Date**: 2026-01-21
**Status**: Design

This document defines the cache invalidation mechanisms for the PulumiCost AWS CE plugin.

## Overview

The plugin implements hybrid caching (in-memory + disk persistence) with automatic 24-hour TTL and manual invalidation support. Cache invalidation can be triggered through environment variables for force-refresh scenarios.

## Automatic Invalidation

### Time-Based Expiration

**TTL**: 24 hours from cache write time

**Mechanism**: Dual-layer validation

1. **Embedded timestamp** (primary): `ExpiresAt` field in JSON
2. **File modification time** (fallback): Filesystem timestamp

**Eviction strategy**: Lazy (on-read)

- Cache entries are not actively cleaned up in background
- Stale entries removed when accessed or during cache load

**Implementation:**

```go
// Check embedded timestamp first (fastest)
if time.Now().After(entry.ExpiresAt) {
    return nil, false  // Expired, cache miss
}

// Fallback: check file modification time
if time.Since(fileInfo.ModTime()) > 24*time.Hour {
    os.Remove(cacheFilePath)  // Lazy cleanup
    return nil, false
}
```

### Cache Write

**When**: After successful AWS Cost Explorer API call

**Process:**

1. Transform AWS response to internal format
2. Create `CacheEntry` with `ExpiresAt = time.Now().Add(24 * time.Hour)`
3. Write to in-memory cache (immediate)
4. Write to disk cache (asynchronous, best-effort)

**Failure handling:**

- In-memory write failure: Log warning, continue (cache is optional)
- Disk write failure: Log warning, continue (cache is optional)
- Never return error to caller due to cache failures

## Manual Invalidation

### Environment Variable: Force Refresh

**Variable**: `FINFOCUS_CACHE_BYPASS`

**Values**:

- `"true"`: Bypass cache completely, fetch fresh data
- `"false"` or unset: Use cache normally

**Scope**: Process-wide (affects all requests for duration of plugin execution)

**Usage:**

```bash
# Local development
FINFOCUS_CACHE_BYPASS=true finfocus query --resource arn:aws:ec2:...

# CI/CD
FINFOCUS_CACHE_BYPASS=true make e2e
```

**Implementation:**

```go
func (c *Calculator) GetActualCost(ctx context.Context, req *pbc.GetActualCostRequest) (*pbc.GetActualCostResponse, error) {
    cacheBypass := os.Getenv("FINFOCUS_CACHE_BYPASS") == "true"

    var costs []CostEntry
    if !cacheBypass && c.cache != nil {
        cached, ok := c.cache.Get(cacheKey)
        if ok {
            c.logger.Debug().Str("cache_key", cacheKey).Msg("Cache hit")
            return buildResponse(cached), nil
        }
    }

    // Fetch from AWS Cost Explorer API
    costs, err := c.fetchFromAWS(ctx, req)
    if err != nil {
        return nil, err
    }

    // Cache results (unless bypass active)
    if !cacheBypass && c.cache != nil {
        if err := c.cache.Set(cacheKey, costs); err != nil {
            c.logger.Warn().Err(err).Msg("Failed to cache results")
        }
    }

    return buildResponse(costs), nil
}
```

**Logging:**

```json
{
  "level": "debug",
  "message": "Cache bypass enabled via environment variable",
  "cache_bypass": true,
  "operation": "GetActualCost",
  "resource_id": "i-12345"
}
```

### Future: Request-Level Invalidation

**Proposed**: Add `force_refresh` field to protobuf request

**Benefits:**

- Granular control per request
- No global state (environment variable)
- Supports concurrent requests with different cache preferences

**Requires**: Upstream proto change in finfocus-spec

**Example future API:**

```protobuf
message GetActualCostRequest {
    ResourceDescriptor resource = 1;
    google.protobuf.Timestamp start = 2;
    google.protobuf.Timestamp end = 3;
    bool force_refresh = 4;  // NEW: Bypass cache for this request
}
```

## Cache Directory Operations

### Clear All Cache

**Not implemented in Phase 1** (manual file deletion required)

**Manual procedure:**

```bash
# Linux/macOS
rm -rf ~/.cache/finfocus/aws-ce/*.json

# Windows
del %LocalAppData%\finfocus\aws-ce\*.json
```

**Future enhancement**: CLI command

```bash
finfocus-plugin-aws-ce cache clear
```

### Inspect Cache

**Not implemented in Phase 1** (manual file inspection required)

**Manual procedure:**

```bash
# List cache files
ls -lh ~/.cache/finfocus/aws-ce/

# View cache entry
cat ~/.cache/finfocus/aws-ce/cost_v1_i-12345___1704067200_1705276800.json | jq
```

**Future enhancement**: CLI command

```bash
finfocus-plugin-aws-ce cache list
finfocus-plugin-aws-ce cache inspect <resource-id>
```

## Cache Key Structure

**Format**: `cost:v1:RESOURCE:SERVICE:REGION:START:END`

**Components**:

- `cost`: Prefix for cache entry type
- `v1`: Schema version (allows evolution)
- `RESOURCE`: ResourceId (e.g., `i-12345`)
- `SERVICE`: Service filter (e.g., `AmazonEC2`, or `_` if not filtered)
- `REGION`: Region filter (e.g., `us-east-1`, or `_` if not filtered)
- `START`: Unix timestamp (seconds)
- `END`: Unix timestamp (seconds)

**Examples:**

```
cost:v1:i-12345:_:_:1704067200:1705276800
cost:v1:i-67890:AmazonEC2:us-east-1:1704067200:1705276800
cost:v1:db-ABC123:AmazonRDS:us-west-2:1704067200:1705276800
```

**File mapping:**

- Replace `:` with `_` for filesystem safety
- Add `.json` extension
- Example: `cost_v1_i-12345___1704067200_1705276800.json`

## Cache Consistency

### Read-Your-Writes

**Guaranteed**: Yes (in-memory cache updated synchronously)

**Scenario**: Same process, sequential requests

```
Request 1: GetActualCost(i-12345, Jan 1-7) → Cache MISS → Fetch AWS → Cache SET
Request 2: GetActualCost(i-12345, Jan 1-7) → Cache HIT → Return cached
```

### Cross-Process Consistency

**Guarantee**: Eventual consistency (24-hour window)

**Scenario**: Multiple plugin instances (different processes)

```
Process A: Fetches cost data at 10:00 AM → Caches with expires_at = 10:00 AM + 24h
Process B: Starts at 11:00 AM → Loads disk cache → Uses Process A's cached data
Process C: Starts at 10:01 AM next day → Cached data expired → Fetches fresh
```

**Staleness window**: Maximum 24 hours (by design)

### Cache Coherence

**Problem**: What if AWS data changes within 24-hour window?

**Answer**: Acceptable for use case

- AWS Cost Explorer data refreshes every ~8 hours
- Historical cost data rarely changes after 48 hours
- 24-hour TTL balances freshness vs. API cost/latency
- Users can force refresh if immediate data needed

## Error Handling

### Cache Read Failures

**Behavior**: Treat as cache miss, fetch from AWS

**Logging**:

```json
{
  "level": "warn",
  "message": "Failed to read cache entry",
  "error": "json: cannot unmarshal...",
  "cache_key": "cost:v1:i-12345:...",
  "warning_type": "cache_read_failed"
}
```

**User impact**: None (transparent fallback to API)

### Cache Write Failures

**Behavior**: Log warning, continue with response

**Logging**:

```json
{
  "level": "warn",
  "message": "Failed to cache results; performance may degrade",
  "error": "permission denied",
  "cache_key": "cost:v1:i-12345:...",
  "warning_type": "cache_write_failed"
}
```

**User impact**: Subsequent requests slower (no cache hit)

### Disk Full

**Behavior**: Cache writes fail, logged as warnings

**Mitigation**:

- In-memory cache still works for current process
- Lazy eviction will eventually clean up old files
- No automatic disk space management (user responsibility)

**Recommendation**: Monitor `~/.cache/finfocus/aws-ce/` size

## Performance Characteristics

### Cache Hit Latency

**In-Memory**: < 1ms (map lookup)
**Disk**: < 10ms (read + JSON unmarshal)

### Cache Miss Latency

**AWS API Call**: 2-5 seconds (typical)
**Cache Write**: < 5ms (async, non-blocking)

### Cache Size Estimates

**Per entry**: ~1-5 KB (depends on date range granularity)
**With 24h TTL**: Maximum ~100 entries per resource (assuming daily queries)
**Typical**: 10-20 entries (queries for different date ranges)

**Total disk usage**: < 1 MB for typical workload

### Eviction Performance

**Lazy eviction**: O(1) per expired entry encountered
**No active cleanup**: No background goroutines, no startup cost

## Testing Strategy

### Unit Tests

**CacheManager:**

- `TestCacheSet_StoresEntry`
- `TestCacheGet_ReturnsStoredEntry`
- `TestCacheGet_ReturnsNilForExpiredEntry`
- `TestCacheSet_CreatesCacheDirectoryIfMissing`
- `TestCacheGet_HandlesCorruptedJSONGracefully`

**Cache bypass:**

- `TestGetActualCost_BypassesCacheWhenEnvVarSet`
- `TestGetActualCost_UsesCacheWhenEnvVarUnset`

### E2E Tests

**Cache behavior:**

- `TestE2E_CacheHitReducesLatency`
- `TestE2E_CacheExpiredAfter24Hours`
- `TestE2E_ForceRefreshFetchesFreshData`

**Process:**

1. Execute query (cache miss)
2. Execute same query immediately (cache hit, verify fast)
3. Advance time 25 hours (mock or real delay)
4. Execute same query (cache expired, verify fresh data)

## References

- [data-model.md](./data-model.md#1-cache-entry-structure) - Cache data structures
- [research.md](./research.md#1-cache-storage-format--location) - Cache design decisions
