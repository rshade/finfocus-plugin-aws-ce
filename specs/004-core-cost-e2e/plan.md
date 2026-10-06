# Implementation Plan: Core Cost Plugin (Spec 001) & E2E Testing

**Branch**: `004-core-cost-e2e` | **Date**: 2026-01-21 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/004-core-cost-e2e/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement the core AWS Cost Explorer integration for the PulumiCost plugin, enabling retrieval of actual cloud spending data through the `GetActualCost` gRPC endpoint. Add hybrid caching (in-memory + disk persistence) with 24-hour TTL and manual invalidation to minimize AWS API calls. Establish comprehensive E2E testing infrastructure that executes real AWS API calls in CI using GitHub OIDC federation for secure, credential-free authentication. Implement resilient error handling with exponential backoff retries and detailed diagnostics for network failures and partial data scenarios.

## Technical Context

**Language/Version**: Go 1.25.5
**Primary Dependencies**: finfocus-spec v0.5.2+ (plugin SDK), aws-sdk-go-v2 (Cost Explorer), zerolog (logging)
**Storage**: Disk-based cache for cost data persistence (format/location: NEEDS CLARIFICATION)
**Testing**: Go testing framework + pluginsdk.NewTestPlugin() for integration tests, E2E tests with real AWS API (NEEDS CLARIFICATION: E2E test framework structure)
**Target Platform**: Linux server (gRPC plugin, loopback-only serving)
**Project Type**: Single (Go plugin)
**Performance Goals**:
  - Plugin startup: < 500ms
  - PORT announcement: < 1s
  - GetActualCost() RPC: < 10s for 30-day ranges
  - E2E test execution: < 2min with 7-day max queries
**Constraints**:
  - gRPC protocol compliance (must not break CostSourceService interface)
  - AWS Cost Explorer API rate limits (caching required)
  - Memory bounded operation (pagination for large result sets)
  - 24-hour cache TTL with manual invalidation support
  - Exponential backoff retry (3 attempts max) for network failures
**Scale/Scope**:
  - Support 100+ concurrent RPC calls
  - Handle multi-account AWS cost queries
  - E2E tests limited to 7-day date ranges (cost/time optimization)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **Code Quality & Simplicity** | ✅ PASS | Feature extends existing plugin architecture using SDK helpers; no premature abstraction planned |
| **Testing Standards** | ✅ PASS | Unit tests for cache logic, integration tests via pluginsdk.NewTestPlugin(), E2E tests with real AWS API |
| **User Experience Consistency** | ✅ PASS | Implements GetActualCost() per gRPC CostSourceService protocol; uses pluginsdk error helpers; structured logging with zerolog |
| **Performance Requirements** | ✅ PASS | Lazy AWS client init; caching strategy addresses API call optimization; timeout constraints defined (2min E2E, 10s RPC) |
| **Security Requirements** | ✅ PASS | GitHub OIDC federation for CI (no long-lived credentials); standard AWS SDK credential chain; loopback-only gRPC serving |
| **Development Workflow** | ✅ PASS | Feature branch `004-core-cost-e2e`; conventional commits enforced; will update CLAUDE.md with E2E test conventions |

**Gate Result**: ✅ ALL CHECKS PASS - Proceed to Phase 0

## Project Structure

### Documentation (this feature)

```text
specs/004-core-cost-e2e/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (pending)
├── data-model.md        # Phase 1 output (pending)
├── quickstart.md        # Phase 1 output (pending)
├── contracts/           # Phase 1 output (pending)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Single project structure (Go plugin)

cmd/plugin/
└── main.go              # gRPC server entry point (already exists)

internal/
├── pricing/
│   ├── calculator.go    # Core Calculator implementation (already exists)
│   ├── calculator_test.go
│   └── cache.go         # NEW: Hybrid caching implementation
│   └── cache_test.go    # NEW: Cache unit tests
├── client/
│   ├── client.go        # AWS Cost Explorer client (already exists)
│   └── client_test.go
└── retry/               # NEW: Exponential backoff retry logic
    ├── retry.go
    └── retry_test.go

tests/
├── e2e/                 # NEW: E2E test suite
│   ├── e2e_test.go      # Real AWS API integration tests
│   └── testdata/        # Test fixtures (if needed)
└── integration/
    └── plugin_test.go   # pluginsdk.NewTestPlugin() tests (already exists)

.github/workflows/
└── e2e.yml              # NEW: CI workflow for E2E tests with OIDC

Makefile                 # NEW target: `make e2e`
```

**Structure Decision**: Extends existing single-project Go plugin structure. New cache implementation in `internal/pricing/cache.go` collocated with calculator logic. Separate `internal/retry/` package for reusable exponential backoff logic (follows SRP). E2E tests in dedicated `tests/e2e/` directory to clearly distinguish from unit/integration tests. GitHub Actions workflow for OIDC-based CI execution.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

*No violations - table not required.*

## Phase 0: Outline & Research

### Research Tasks

The following unknowns from Technical Context require investigation:

1. **Cache Storage Format & Location**
   - Decision needed: File format (JSON, gob, protobuf)
   - Location: XDG cache dir vs. plugin-specific directory
   - Cache key structure for multi-dimensional queries
   - Eviction strategy for 24-hour TTL
   - Research: Best practices for disk-based caching in Go plugins

2. **E2E Test Framework Structure**
   - Integration with existing `make test` vs. separate `make e2e`
   - Test data generation strategy (real AWS account setup)
   - Skipping logic when AWS credentials unavailable
   - CI environment variable configuration
   - Research: Go E2E testing patterns for external API dependencies

3. **Exponential Backoff Retry Strategy**
   - Base delay, max delay, multiplier values
   - Retry-able vs. non-retry-able AWS errors
   - Timeout integration with retry attempts
   - Research: AWS SDK v2 retry configuration best practices

4. **Partial Data Warning Metadata**
   - gRPC response field for warnings (proto changes needed?)
   - Logging format for partial data scenarios
   - Research: pluginsdk error/warning conventions

5. **GitHub OIDC Federation Setup**
   - AWS IAM role configuration requirements
   - Trust policy for GitHub Actions
   - Permissions boundary for Cost Explorer read-only access
   - Research: GitHub Actions OIDC with AWS integration patterns

### Research Output

**Output**: `research.md` (to be generated)

## Phase 1: Design & Contracts

### Prerequisites

- `research.md` complete with all NEEDS CLARIFICATION resolved

### Deliverables

1. **data-model.md**
   - Cache entry structure
   - AWS Cost Explorer request/response mappings
   - Retry state machine
   - Warning metadata model

2. **contracts/**
   - No new gRPC contracts (extends existing CostSourceService)
   - Document AWS Cost Explorer API integration contract
   - Cache invalidation API (environment variable or request parameter)

3. **quickstart.md**
   - Local E2E test execution steps
   - AWS credentials setup for development
   - Cache debugging commands

4. **Agent context update**
   - Run `.specify/scripts/bash/update-agent-context.sh claude`
   - Add E2E testing conventions to agent context

### Constitution Re-check

*Performed after Phase 1 design completion.*

| Principle | Status | Notes |
|-----------|--------|-------|
| **Code Quality & Simplicity** | ✅ PASS | Design maintains KISS: JSON for cache (no code generation), structured keys (no abstraction), lazy eviction (no background processes) |
| **Testing Standards** | ✅ PASS | Clear testing strategy: Unit tests for cache/retry logic, integration tests via pluginsdk pattern, E2E tests with real AWS API |
| **User Experience Consistency** | ✅ PASS | No gRPC protocol changes; uses existing SDK error helpers; structured logging with standard fields; warning metadata via logs only |
| **Performance Requirements** | ✅ PASS | Cache design meets latency targets: <100ms cache hit, <10s RPC call; 24h TTL balances freshness vs. cost; retry strategy optimized for Cost Explorer |
| **Security Requirements** | ✅ PASS | OIDC design eliminates long-lived credentials; minimal IAM permissions (read-only Cost Explorer); loopback-only serving maintained |
| **Development Workflow** | ✅ PASS | Makefile targets defined; E2E conventions documented; CLAUDE.md updated with cache/warning patterns |

**Gate Result**: ✅ ALL CHECKS PASS - Phase 1 complete, ready for implementation

**Key Design Validations:**
- ✅ No premature abstraction (cache uses simple JSON + filesystem)
- ✅ Explicit error handling (retry logic with type-based classification)
- ✅ No magic behavior (cache bypass via explicit env var)
- ✅ Protocol compatibility maintained (logging-only warnings)

## Completion Status

- [x] Phase 0: Research completed - All unknowns resolved
- [x] Phase 1: Design artifacts generated
  - [x] `research.md` - Comprehensive research findings
  - [x] `data-model.md` - Cache, retry, and AWS integration structures
  - [x] `contracts/aws-cost-explorer.md` - AWS API integration contract
  - [x] `contracts/cache-invalidation.md` - Cache behavior specification
  - [x] `quickstart.md` - Developer onboarding guide
  - [x] Agent context updated (`CLAUDE.md`)
  - [x] Constitution re-check passed
- [ ] Phase 2: Task generation (next: `/speckit.tasks`)

## Next Steps

**Ready for user approval** - Review plan.md and design artifacts before proceeding to `/speckit.tasks` for task breakdown.
