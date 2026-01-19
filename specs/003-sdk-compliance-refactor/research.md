# Research: SDK Compliance Refactor

**Date**: 2025-12-16
**Feature**: 003-sdk-compliance-refactor

## Overview

This document captures research findings for the SDK compliance refactoring. Since this is a refactoring task (not new feature development), the research focuses on SDK helper API signatures and migration patterns.

## SDK Helper Functions (Verified in pluginsdk v0.5.2)

### Environment Variable Helpers

| Function | Signature | Behavior |
| -------- | --------- | -------- |
| `GetPort()` | `func GetPort() int` | Reads `PULUMICOST_PLUGIN_PORT`, returns 0 if unset |
| `GetLogLevel()` | `func GetLogLevel() string` | Reads `PULUMICOST_LOG_LEVEL`, falls back to `LOG_LEVEL` |
| `GetLogFile()` | `func GetLogFile() string` | Reads `PULUMICOST_LOG_FILE`, empty = stderr |
| `GetLogFormat()` | `func GetLogFormat() string` | Reads `PULUMICOST_LOG_FORMAT` (json/text) |
| `IsTestMode()` | `func IsTestMode() bool` | Reads `PULUMICOST_TEST_MODE == "true"` |

**Decision**: Use all environment variable helpers directly. No custom fallback logic needed.
**Rationale**: SDK helpers provide consistent behavior across all PulumiCost plugins.
**Alternatives Considered**: Keep existing manual `os.Getenv` calls - rejected for consistency.

### CLI Flag Helpers

| Function | Signature | Behavior |
| -------- | --------- | -------- |
| `ParsePortFlag()` | `func ParsePortFlag() int` | Parses `--port` flag after `flag.Parse()` |

**Decision**: Call `flag.Parse()` in main, then use `ParsePortFlag()` to get port override.
**Rationale**: CLI flags take precedence over environment variables (per FR-005).
**Alternatives Considered**: Custom flag parsing - rejected; SDK handles precedence correctly.

### Logging Helpers

| Function | Signature | Behavior |
| -------- | --------- | -------- |
| `NewLogWriter()` | `func NewLogWriter() io.Writer` | Returns file writer if `PULUMICOST_LOG_FILE` set, else stderr |
| `NewPluginLogger()` | `func NewPluginLogger(pluginName, version string, level zerolog.Level, w io.Writer) zerolog.Logger` | Creates configured zerolog logger |
| `LogOperation()` | `func LogOperation(logger zerolog.Logger, operation string) func()` | Returns defer-able function that logs operation timing |
| `ResetLogWriter()` | `func ResetLogWriter()` | Closes and resets the global log writer (for tests) |

**Decision**: Replace existing logger initialization with SDK pattern.
**Rationale**: SDK logger includes standard fields (`component`, `plugin_name`, `plugin_version`).
**Alternatives Considered**: Keep existing zerolog setup - rejected; SDK provides better defaults.

### Validation Helpers

| Function | Signature | Behavior |
| -------- | --------- | -------- |
| `ValidateActualCostRequest()` | `func ValidateActualCostRequest(req *pbc.GetActualCostRequest) error` | Validates required fields, time range |
| `ValidateProjectedCostRequest()` | `func ValidateProjectedCostRequest(req *pbc.GetProjectedCostRequest) error` | Validates resource descriptor fields |

**Error Constants Available**:

- `ErrActualCostRequestNil` - request is required
- `ErrActualCostResourceIDEmpty` - resource_id is required
- `ErrActualCostStartTimeNil` - start_time is required
- `ErrActualCostEndTimeNil` - end_time is required
- `ErrActualCostTimeRangeInvalid` - end_time must be strictly after start_time

**Decision**: Replace custom validation in `GetActualCost()` with SDK helper.
**Rationale**: SDK validation returns gRPC-compatible error codes.
**Alternatives Considered**: Keep custom validation - rejected; inconsistent with other plugins.

### Response Builders

| Function | Signature | Behavior |
| -------- | --------- | -------- |
| `NoDataError()` | `func NoDataError(resourceID string) error` | Returns standardized no-data error |
| `NotSupportedError()` | `func NotSupportedError(resource *pbc.ResourceDescriptor) error` | Returns standardized not-supported error |

**Decision**: Continue using existing SDK error helpers (already in use).
**Rationale**: Already compliant; no change needed.

## Migration Patterns

### Pattern 1: main.go Startup

**Current Code** (`cmd/plugin/main.go:37-40`):

```go
log.Printf("Starting %s plugin...", plugin.Name())
if err := pluginsdk.Serve(ctx, config); err != nil {
    log.Fatalf("Failed to serve plugin: %v", err)
}
```

**Migrated Code**:

```go
// Initialize logger using SDK helpers
logWriter := pluginsdk.NewLogWriter()
level := parseLogLevel(pluginsdk.GetLogLevel())
logger := pluginsdk.NewPluginLogger("aws-ce", "1.0.0", level, logWriter)

// Parse port with CLI precedence
flag.Parse()
port := pluginsdk.ParsePortFlag()
if port == 0 {
    port = pluginsdk.GetPort()
}

logger.Info().Msg("Starting plugin")
if err := pluginsdk.Serve(ctx, config); err != nil {
    logger.Error().Err(err).Msg("Failed to serve plugin")
    return // Exit without os.Exit or log.Fatal
}
```

**Key Change**: Replace `log.Fatalf` with `logger.Error()` + return.

### Pattern 2: Request Validation

**Current Code** (`internal/pricing/calculator.go:103-104`):

```go
if resourceID == "" {
    return nil, fmt.Errorf("invalid request: ResourceId is required")
}
```

**Migrated Code**:

```go
if err := pluginsdk.ValidateActualCostRequest(req); err != nil {
    return nil, status.Error(codes.InvalidArgument, err.Error())
}
```

**Key Change**: Replace manual field checks with SDK validation.

### Pattern 3: Operation Logging

**Current Code** (`internal/pricing/calculator.go:189-201`):

```go
start := time.Now()
clientCosts, err := c.ceClient.GetCost(ctx, filter, dimensions, startTime, endTime, granularity)
duration := time.Since(start)

c.logger.Info().
    Int("results_count", len(clientCosts)).
    Dur("duration", duration).
    Msg("Retrieved costs from AWS")
```

**Migrated Code**:

```go
done := pluginsdk.LogOperation(c.logger, "GetCost")
clientCosts, err := c.ceClient.GetCost(ctx, filter, dimensions, startTime, endTime, granularity)
done()  // Logs operation with timing

c.logger.Info().
    Int("results_count", len(clientCosts)).
    Msg("Retrieved costs from AWS")
```

**Key Change**: Use `LogOperation()` for automatic timing.

## Files Requiring Changes

| File | Changes |
| ---- | ------- |
| `cmd/plugin/main.go` | Add logging init, port flag parsing, remove `log.Fatalf` |
| `internal/pricing/calculator.go` | Add SDK validation, use `LogOperation()` |
| `internal/pricing/calculator_test.go` | Update tests for new validation error messages |
| `CLAUDE.md` | Document SDK helper usage patterns |
| `README.md` | Document environment variables and CLI flags |

## Open Questions (Resolved)

1. **Q**: Does `ParsePortFlag()` require `flag.Parse()` to be called first?
   **A**: Yes, standard Go flag package behavior.

2. **Q**: How does `NewLogWriter()` handle non-writable paths?
   **A**: Returns stderr as fallback; logs warning if file creation fails.

3. **Q**: Does SDK validation cover the 14-month lookback limit?
   **A**: No, this is AWS-specific business logic. Keep custom check.

## Conclusion

All SDK helpers are available and well-documented. The migration is straightforward with no blocking issues. Custom 14-month lookback validation should be retained as it's AWS Cost Explorer specific business logic not covered by SDK validation.
