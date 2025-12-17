# Data Model: SDK Compliance Refactor

**Date**: 2025-12-16
**Feature**: 003-sdk-compliance-refactor

## Overview

This feature is a refactoring task that migrates existing code to use SDK helpers. **No new data entities or schema changes are introduced.**

## Existing Entities (Unchanged)

The following entities remain unchanged by this refactoring:

| Entity | Location | Purpose |
| ------ | -------- | ------- |
| `Calculator` | `internal/pricing/calculator.go` | Plugin implementation struct |
| `CostEntry` | `internal/pricing/data.go` | Internal cost data representation |
| `CacheManager` | `internal/pricing/cache.go` | Optional cost data caching |
| `Client` | `internal/client/client.go` | AWS Cost Explorer API client |
| `CostResult` | `internal/client/client.go` | API response mapping struct |

## Configuration Changes

While no data entities change, the plugin's configuration surface expands:

### Environment Variables (SDK Standard)

| Variable | Type | Default | Description |
| -------- | ---- | ------- | ----------- |
| `PULUMICOST_PLUGIN_PORT` | int | 0 (auto) | gRPC server port |
| `PULUMICOST_LOG_FILE` | string | "" (stderr) | Log output file path |
| `PULUMICOST_LOG_LEVEL` | string | "info" | Log verbosity (debug/info/warn/error) |
| `PULUMICOST_LOG_FORMAT` | string | "json" | Log format (json/text) |
| `PULUMICOST_TEST_MODE` | string | "false" | Enable test mode behaviors |

### CLI Flags (New)

| Flag | Type | Description |
| ---- | ---- | ----------- |
| `--port` | int | Override port (takes precedence over env var) |

## State Transitions (Unchanged)

Plugin lifecycle states remain unchanged:

```text
[Init] → [Serving] → [Shutdown]
           ↑    ↓
       [Processing]
```

The refactoring changes HOW shutdown is signaled (context cancellation instead of `os.Exit`), but not the states themselves.
