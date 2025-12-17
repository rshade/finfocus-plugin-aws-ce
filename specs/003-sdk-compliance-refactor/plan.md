# Implementation Plan: SDK Compliance Refactor

**Branch**: `003-sdk-compliance-refactor` | **Date**: 2025-12-16 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/003-sdk-compliance-refactor/spec.md`

## Summary

Refactor the AWS Cost Explorer plugin to use standardized `pluginsdk` helpers for environment variable reading, logging initialization, request validation, and graceful shutdown. This brings the plugin into full compliance with the PulumiCost SDK patterns, ensuring consistent behavior across all plugins in the ecosystem.

**Key Changes:**

- Replace `log.Fatalf` with error returns in `main.go`
- Adopt `pluginsdk.GetPort()`, `pluginsdk.GetLogFile()`, `pluginsdk.GetLogLevel()` for configuration
- Use `pluginsdk.NewPluginLogger()` and `pluginsdk.LogOperation()` for standardized logging
- Use `pluginsdk.ValidateActualCostRequest()` for request validation
- Support `--port` CLI flag via `pluginsdk.ParsePortFlag()`

## Technical Context

**Language/Version**: Go 1.25.5
**Primary Dependencies**: pulumicost-spec v0.4.7, aws-sdk-go-v2, zerolog
**Storage**: N/A (stateless plugin with optional cache)
**Testing**: `go test` via `make test`, using `pluginsdk.NewTestPlugin(t, plugin)` pattern
**Target Platform**: Linux/macOS/Windows (gRPC server binary)
**Project Type**: Single binary gRPC plugin
**Performance Goals**: Plugin startup < 500ms, PORT announcement < 1s (per constitution)
**Constraints**: No `os.Exit` or `log.Fatal` calls; strict SDK compliance
**Scale/Scope**: Single plugin, ~500 LOC affected

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
| --------- | ------ | ----- |
| I. Code Quality & Simplicity | ✅ PASS | Refactoring reduces custom code by adopting SDK helpers |
| II. Testing Standards | ✅ PASS | Existing tests will be updated; no new complex mocking needed |
| III. User Experience Consistency | ✅ PASS | This feature explicitly implements UX consistency requirements |
| III.a zerolog structured logging | ✅ PASS | SDK logger uses zerolog internally |
| III.b pluginsdk.Serve() lifecycle | ✅ PASS | Already in use; refactor removes `log.Fatalf` |
| III.c Error codes via proto enum | ✅ PASS | SDK validation errors use proto-defined codes |
| IV. Performance Requirements | ✅ PASS | No performance impact; lazy init preserved |
| Security Requirements | ✅ PASS | No credential changes; loopback-only serving unchanged |
| Development Workflow | ✅ PASS | Feature branch follows `###-feature-name` convention |

**Gate Result**: PASS - No violations. Proceeding to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/003-sdk-compliance-refactor/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (N/A for refactoring)
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (N/A for refactoring)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
cmd/
└── plugin/
    └── main.go           # Refactor: env vars, logging, port flag, remove log.Fatalf

internal/
├── pricing/
│   ├── calculator.go     # Refactor: SDK validation, LogOperation()
│   └── calculator_test.go # Update: tests for new validation behavior
└── client/
    └── client.go         # Review: ensure no log.Fatal calls
```

**Structure Decision**: Existing single-binary structure. Changes affect `cmd/plugin/main.go` (startup/shutdown) and `internal/pricing/calculator.go` (validation/logging).

## Complexity Tracking

> No violations requiring justification. The refactoring reduces complexity by removing custom implementations in favor of SDK helpers.

| Violation | Why Needed | Simpler Alternative Rejected Because |
| --------- | ---------- | ------------------------------------ |
| None | N/A | N/A |
