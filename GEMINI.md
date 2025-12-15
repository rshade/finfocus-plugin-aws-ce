# GEMINI.md

This file provides context and instructions for Gemini agents working on the `pulumicost-plugin-aws-ce` project.

## Project Overview

`pulumicost-plugin-aws-ce` is a PulumiCost plugin designed to retrieve **actual** and **projected** cloud costs directly from the AWS Cost Explorer API. It integrates with the `pulumicost-core` engine via gRPC, adhering to the `pulumicost-spec` interface.

**Key Features:**
- Retrieves actual historical cost data (Cost Explorer API).
- Calculates projected costs (Forecasting API).
- Supports AWS Budgets and Anomaly Detection (Planned).
- Provides optimization recommendations (Rightsizing, Savings Plans - Planned).
- Compliant with FinOps FOCUS 1.2 data standards.

## Build & Run

**Prerequisites:**
- Go 1.25.5
- `make`
- `golangci-lint` (for linting)
- `goreleaser` (for release builds)

**Commands:**

| Command | Description |
| :--- | :--- |
| `make build` | Compiles the plugin binary to `bin/pulumicost-plugin-aws-ce`. |
| `make test` | Runs all unit tests. |
| `make lint` | Runs `golangci-lint` to ensure code quality. |
| `make install` | Builds and installs the plugin to `~/.pulumicost/plugins/aws-ce/1.0.0/`. |
| `make deps` | Updates Go dependencies (`go mod tidy` + `download`). |
| `make fmt` | Formats code using `go fmt`. |

**Running Tests:**
To run specific tests (e.g., E2E integration tests requiring AWS creds):
```bash
go test -v -tags=e2e ./tests/...
```

## Architecture

This project is a single-binary gRPC server.

### Directory Structure
- `cmd/plugin/`: Contains `main.go`, the entry point that initializes the gRPC server and starts listening.
- `internal/pricing/`: Contains the core logic.
    - `calculator.go`: Implements the `Plugin` interface methods (`GetActualCost`, `GetProjectedCost`, etc.).
- `internal/client/`: Wraps the AWS SDK v2 clients (Cost Explorer, Budgets).
- `specs/`: Contains feature specifications (Markdown) following the Spec-Kit methodology.
- `.github/workflows/`: CI/CD definitions (Test, Release).

### Integration Points
- **Core Engine**: Communicates via gRPC (Protocol Buffers defined in `pulumicost-spec`).
- **AWS API**: Authenticates using standard AWS SDK credential chains (Env vars, Profile, Role).

## Development Conventions

1.  **SDK Usage**: You **MUST** use `pluginsdk` helpers for:
    - Environment variables (`pluginsdk/env`).
    - Request validation (`pluginsdk/validation`).
    - Logging (`pluginsdk/logging` with `zerolog`).
    - Data construction (`pluginsdk/focus_builder` for FOCUS 1.2 records).
2.  **Logging**: Structured JSON logging is required. Respect `PULUMICOST_LOG_LEVEL` and `PULUMICOST_LOG_FILE`.
3.  **Error Handling**: Do not use `os.Exit`. Return errors via gRPC status codes. Use `pluginsdk.NotSupportedError()` where applicable.
4.  **Testing**:
    - Unit tests for logic.
    - Integration tests with mocked AWS clients.
    - E2E tests with real AWS credentials (guarded by build tags or env vars).

## Plugin SDK Reference (v0.4.6)

The `pluginsdk` package (`github.com/rshade/pulumicost-spec/sdk/go/pluginsdk`) provides standardized helpers that **MUST** be used.

### 1. Environment Variables (`env.go`)
- **Usage**: Replace manual `os.Getenv` calls.
- `GetPort()`: `PULUMICOST_PLUGIN_PORT`
- `GetLogLevel()`: `PULUMICOST_LOG_LEVEL`
- `GetLogFile()`: `PULUMICOST_LOG_FILE` (Absolute path)
- `IsTestMode()`: `PULUMICOST_TEST_MODE == "true"`

### 2. Validation (`validation.go`)
- **Usage**: Call at the start of RPC handlers.
- `ValidateProjectedCostRequest(req)`
- `ValidateActualCostRequest(req)`
- Returns pre-defined errors (e.g., `ErrActualCostTimeRangeInvalid`).

### 3. FOCUS 1.2 Builder (`focus_builder.go`)
- **Usage**: Constructing `FocusCostRecord`s for `GetActualCost`.
- `NewFocusRecordBuilder().WithIdentity(...).WithFinancials(...).Build()`
- Ensures compliance with FinOps FOCUS 1.2 schema.

### 4. Logging (`logging.go`)
- **Usage**: Structured Zerolog setup.
- `NewLogWriter()`: Returns writer for `PULUMICOST_LOG_FILE`.
- `NewPluginLogger(name, version, level, writer)`: Creates standard logger.
- `LogOperation(logger, "OperationName")`: returns a done function to defer for timing.

### 5. Server & Flags (`sdk.go`)
- **Usage**: Main entry point.
- `ParsePortFlag()`: Parses `--port` (call `flag.Parse()` first).
- `Serve(ctx, config)`: Starts gRPC server.
- **Interfaces**: Implement `BudgetsProvider` and `RecommendationsProvider` for new features.

## Roadmap & Active Issues

The project is currently executing against the following milestones and issues:

### v0.1.0 - Foundation & CI/CD
- **Issue #6**: Update Dependencies & Refactor for SDK Compliance (Spec v0.4.6, SDK helpers, Zerolog, `PULUMICOST_LOG_FILE`, `--port`).
- **Issue #7**: Establish CI/CD Infrastructure (Workflows, Goreleaser, release-please).
- **Issue #11**: Implement Core Cost Plugin (Spec 001) & E2E Testing (AWS Integration, CI Secrets, FOCUS 1.2 Compliance).
- **Issue #12**: Polish: Installation & Documentation (Makefile version fix, README rewrite, Manifest consolidation).

### v0.2.0 - Core Features
- **Issue #8**: Feature: AWS Budgets Support (New Spec, `getbudgets` RPC).
- **Issue #9**: Feature: Cost Forecasting (New Spec, `GetProjectedCost` RPC).
- **Issue #10**: Feature: Anomaly Detection (New Spec, Anomaly logic).

### v0.3.0 - Advanced Features
- **Issue #13**: Feature: Optimization Recommendations (Rightsizing, Savings Plans).
