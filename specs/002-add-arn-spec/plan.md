# Implementation Plan: Add ARN to GetActualCostRequest

**Branch**: `002-add-arn-spec` | **Date**: 2025-12-15 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/002-add-arn-spec/spec.md`

## Summary

Add an optional `arn` field to `GetActualCostRequest` in the upstream `pulumicost-spec`
protobuf definitions, then update the AWS CE plugin to consume this field for robust
resource identification and identity verification. The ARN serves as the canonical
cloud identifier, enabling precise cost lookups and mismatch detection.

## Technical Context

**Language/Version**: Go 1.25.5
**Primary Dependencies**: pulumicost-spec v0.4.7+ (requires upstream change), aws-sdk-go-v2
**Storage**: N/A (stateless plugin, optional cache)
**Testing**: go test, pluginsdk.NewTestPlugin pattern
**Target Platform**: Linux/macOS/Windows (gRPC plugin)
**Project Type**: Single project (gRPC plugin)
**Performance Goals**: GetActualCost RPC < 10s, Supports RPC < 10ms (per constitution)
**Constraints**: Backward compatible (existing requests without ARN must work)
**Scale/Scope**: Single plugin, 2 repos (spec + plugin)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Code Quality & Simplicity | ✅ PASS | ARN parsing is simple string splitting, no over-engineering |
| II. Testing Standards | ✅ PASS | Unit tests for ARN parsing, integration tests for RPC |
| III. User Experience Consistency | ✅ PASS | gRPC protocol extension via proto field, uses standard error codes |
| IV. Performance Requirements | ✅ PASS | ARN parsing is O(1) string ops, no latency impact |
| Security Requirements | ✅ PASS | ARN is metadata, no credential exposure |
| Development Workflow | ✅ PASS | Feature branch pattern, conventional commits |

**Gate Result**: ✅ PASS - No violations. Proceeding to Phase 0.

## Project Structure

### Documentation (this feature)

```text
specs/002-add-arn-spec/
├── spec.md              # Feature specification (completed)
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (proto changes)
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
# Upstream: rshade/pulumicost-spec (changes required first)
proto/pulumicost/v1/
└── costsource.proto     # Add arn field to GetActualCostRequest

# This repo: rshade/pulumicost-plugin-aws-ce
internal/
├── pricing/
│   ├── calculator.go    # Consume req.GetArn() in GetActualCost
│   ├── calculator_test.go # Unit tests for ARN handling
│   └── arn.go           # NEW: ARN parsing utilities
└── client/
    └── client.go        # No changes needed

cmd/plugin/
└── main.go              # No changes needed
```

**Structure Decision**: Single project with upstream dependency. No new packages
required beyond an optional `arn.go` for parsing utilities. Changes are localized
to `calculator.go` and tests.

## Complexity Tracking

> No violations to justify. Design follows KISS principle.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| None | N/A | N/A |
