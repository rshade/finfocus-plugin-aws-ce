# Implementation Plan: AWS Cost Explorer Plugin for PulumiCost

**Branch**: `001-aws-ce-plugin` | **Date**: 2025-12-10 | **Spec**: /mnt/c/GitHub/go/src/github.com/rshade/pulumicost-plugin-aws-ce/specs/001-aws-ce-plugin/spec.md
**Input**: Feature specification from `/specs/001-aws-ce-plugin/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Create a PulumiCost plugin that retrieves actual historical billing data from AWS Cost Explorer API. The plugin implements the PulumiCost plugin SDK gRPC interface, authenticates with AWS using standard credential providers, and provides cost data grouped by various dimensions (service, account, tags, availability zone). The implementation focuses on actual cost retrieval only (GetProjectedCost returns error), with support for date range queries, reservation utilization data, and graceful handling of AWS API rate limits.

## Technical Context

<!--
  ACTION REQUIRED: Replace the content in this section with the technical details
  for the project. The structure here is presented in advisory capacity to guide
  the iteration process.
-->

**Language/Version**: Go 1.25.5
**Primary Dependencies**: github.com/rshade/finfocus-spec (PulumiCost plugin SDK), github.com/aws/aws-sdk-go-v2 (AWS SDK for Cost Explorer API)
**Storage**: N/A (stateless API client plugin with hybrid in-memory/disk caching)
**Testing**: Go testing with pluginsdk.NewTestPlugin integration pattern, table-driven unit tests for pure functions
**Target Platform**: Linux (cross-platform Go binary compatible with PulumiCost core)
**Project Type**: Single project (gRPC plugin binary)
**Performance Goals**: <10 seconds for GetActualCost RPC, <500ms plugin startup, <5 seconds for Cost Explorer API calls, <10ms for Supports RPC
**Constraints**: AWS Cost Explorer API limits (14 months historical lookback), rate limiting with exponential backoff, read-only ce:GetCostAndUsage permissions, bounded memory usage, loopback-only gRPC serving
**Scale/Scope**: Single plugin binary handling concurrent RPC calls, supports 100+ concurrent requests, processes cost data for multiple AWS services/accounts

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**I. Code Quality & Simplicity**: ✅ PASS - Implementation follows KISS principle with single-responsibility packages, explicit error handling, and comprehensive documentation. File sizes will be kept under 300 lines where possible.

**II. Testing Standards**: ✅ PASS - Unit tests for pure functions (cost calculations, response builders), integration tests using pluginsdk.NewTestPlugin pattern, mocked AWS Cost Explorer API dependencies. No over-engineered mocking infrastructure.

**III. User Experience Consistency**: ✅ PASS - Strict adherence to gRPC CostSourceService protocol, proto-defined error codes, zerolog structured logging with component identifier, pluginsdk response builders and error types.

**IV. Performance Requirements**: ✅ PASS - Lazy AWS client initialization, appropriate timeouts (30s default), batch operations where possible, bounded memory usage with pagination for large result sets.

**Security Requirements**: ✅ PASS - Standard AWS SDK credential chain, read-only Cost Explorer permissions, input validation with graceful error handling, loopback-only gRPC serving.

**Development Workflow**: ✅ PASS - Feature branch naming convention, conventional commits, CI checks (lint, test, build), markdownlint for documentation.

**GATE STATUS**: ✅ ALL GATES PASS - No constitution violations detected. Implementation can proceed.

**POST-DESIGN REVIEW**: ✅ Constitution compliance maintained after Phase 1 design. Data model follows single responsibility principle. API contracts adhere to gRPC protocol requirements. No premature abstraction or over-engineering detected. Design remains simple and focused on core cost retrieval functionality.

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/plugin/
└── main.go              # Plugin entry point, gRPC server setup

internal/
├── client/
│   ├── client.go        # AWS Cost Explorer API client wrapper
│   └── client_test.go   # Unit tests for client functionality
└── pricing/
    ├── calculator.go    # Core Calculator struct with gRPC methods
    ├── calculator_test.go # Integration tests using pluginsdk pattern
    ├── data.go          # Cost data structures and response builders
    └── data_test.go     # Unit tests for data transformations

bin/
└── pulumicost-plugin-aws-ce # Built plugin binary (make build output)
```

**Structure Decision**: Single Go project following standard Go layout conventions. The `internal/` directory contains private packages not intended for external use. `cmd/plugin/` contains the main application entry point. This structure aligns with Go best practices and the existing PulumiCost plugin SDK patterns.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
