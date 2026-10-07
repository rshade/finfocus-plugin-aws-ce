# GEMINI.md

This file provides context and instructions for Gemini agents working on the `finfocus-plugin-aws-ce` project.

## Project overview

`finfocus-plugin-aws-ce` is a FinFocus plugin designed to retrieve **actual** and **projected** cloud costs directly from the AWS Cost Explorer API. It integrates with the `finfocus-core` engine via gRPC, adhering to the `finfocus-spec` interface.

**Key features:**

- Retrieves actual historical cost data from the Cost Explorer API.
- Calculates projected costs with the Forecasting API.
- Plans to support AWS Budgets and anomaly detection.
- Plans to provide optimization recommendations for rightsizing and Savings Plans.
- Compliant with FinOps FOCUS 1.2 data standards.

## Build and run

**Prerequisites:**

- Go 1.25.5
- `make`
- `golangci-lint` for linting
- `goreleaser` for release builds

**Commands:**

| Command | Description |
| :--- | :--- |
| `make build` | Compiles the plugin binary to `bin/finfocus-plugin-aws-ce`. |
| `make test` | Runs all unit tests. |
| `make lint` | Runs `golangci-lint` to ensure code quality. |
| `make install` | Builds and installs the plugin to `~/.finfocus/plugins/aws-ce/1.0.0/`. |
| `make ensure` | Updates Go dependencies as an alias for `deps`. |
| `make deps` | Updates Go dependencies with `go mod tidy` and `go mod download`. |
| `make fmt` | Formats code using `go fmt`. |

### Running tests

To run specific tests, such as end-to-end integration tests that require AWS credentials:

```bash
go test -v -tags=e2e ./tests/...
```

## Architecture

This project is a single-binary gRPC server.

### Directory structure

- `cmd/plugin/`: contains `main.go`, the entry point that initializes the gRPC server and starts listening.
- `internal/pricing/`: contains the core logic.
  - `calculator.go`: implements the `Plugin` interface methods, including `GetActualCost` and `GetProjectedCost`.
- `internal/client/`: wraps the AWS SDK v2 clients for Cost Explorer and Budgets.
- `specs/`: contains feature specifications in Markdown following the Spec-Kit methodology.
- `.github/workflows/`: defines continuous integration and delivery workflows for tests and releases.

### Integration points

- **Core engine**: communicates via gRPC with Protocol Buffers from `finfocus-spec`.
- **AWS API**: authenticates using standard AWS SDK credential chains for environment variables, profiles, and roles.

## Development conventions

1. **SDK usage**: you must use `pluginsdk` helpers for:
    - Environment variables through `pluginsdk/env`.
    - Request validation through `pluginsdk/validation`.
    - Logging through `pluginsdk/logging` with `zerolog`.
    - Data construction through `pluginsdk/focus_builder` for FOCUS 1.2 records.
2. **Logging**: use structured JSON logging. Respect `FINFOCUS_LOG_LEVEL` and `FINFOCUS_LOG_FILE`.
3. **Error handling**: don't use `os.Exit`. Return errors via gRPC status codes. Use `pluginsdk.NotSupportedError()` where applicable.
4. **Testing**:
    - Unit tests for logic.
    - Integration tests with mocked AWS clients.
    - End-to-end tests with real AWS credentials. Guard these tests with build tags or environment variables.

## Plugin SDK reference for v0.5.2

The `pluginsdk` package at `github.com/rshade/finfocus-spec/sdk/go/pluginsdk`
provides standardized helpers. You must use these helpers.

### 1. Environment variables

Use the helpers in `env.go` to replace manual `os.Getenv` calls.

- `GetPort()` reads `FINFOCUS_PLUGIN_PORT`.
- `GetLogLevel()` reads `FINFOCUS_LOG_LEVEL`.
- `GetLogFile()` reads `FINFOCUS_LOG_FILE`, an absolute path.
- `IsTestMode()` checks `FINFOCUS_TEST_MODE == "true"`.

### 2. Validation

Call the helpers in `validation.go` at the start of remote procedure call handlers.

- `ValidateProjectedCostRequest(req)`
- `ValidateActualCostRequest(req)`

These helpers return predefined errors, such as `ErrActualCostTimeRangeInvalid`.

### 3. FOCUS 1.2 builder

Use `focus_builder.go` to construct `FocusCostRecord` values for `GetActualCost`.

- `NewFocusRecordBuilder().WithIdentity(...).WithFinancials(...).Build()`

The builder ensures compliance with the FinOps FOCUS 1.2 schema.

### 4. Logging

Use `logging.go` to set up structured logging with `zerolog`.

- `NewLogWriter()` returns a writer for `FINFOCUS_LOG_FILE`.
- `NewPluginLogger(name, version, level, writer)` creates a standard logger.
- `LogOperation(logger, "OperationName")` returns a function to defer for timing.

### 5. Server and flags

Use `sdk.go` for the main entry point.

- `ParsePortFlag()` parses `--port`. Call `flag.Parse()` first.
- `Serve(ctx, config)` starts the gRPC server.
- Implement `BudgetsProvider` and `RecommendationsProvider` for new features.

## Roadmap and active issues

The project follows these milestones and issues:

### Foundation and continuous integration and delivery for v0.1.0

- **Issue #6**: update dependencies and refactor for SDK compliance with spec v0.5.2, SDK helpers, and `zerolog`, `FINFOCUS_LOG_FILE`, and `--port`.
- **Issue #7**: establish continuous integration and delivery infrastructure with workflows, `goreleaser`, and `release-please`.
- **Issue #11**: implement the core cost plugin from spec 001 and end-to-end testing with AWS integration and workflow secrets and FOCUS 1.2 compliance.
- **Issue #12**: polish installation and documentation with a `Makefile` version fix and a `README` rewrite and manifest consolidation.

### Core features for v0.2.0

- **Issue #8**: add AWS Budgets support with a new spec and the `getbudgets` remote procedure call.
- **Issue #9**: add cost forecasting with a new spec and the `GetProjectedCost` remote procedure call.
- **Issue #10**: add anomaly detection with a new spec and anomaly logic.

### Advanced features for v0.3.0

- **Issue #13**: add optimization recommendations for rightsizing and Savings Plans.

## Active technologies

- `001-cicd-infrastructure` uses Go 1.25.5, `goreleaser` for cross-platform binary builds, `golangci-lint` v2.6.2 for code quality, `release-please` for automated versioning, and GitHub Actions for continuous integration and delivery workflows.
- `001-cicd-infrastructure` changes configuration and documentation files only.

## Recent changes

- `001-cicd-infrastructure` added Go 1.25.5, `goreleaser` for cross-platform binary builds, `golangci-lint` v2.6.2 for code quality, `release-please` for automated versioning, and GitHub Actions for continuous integration and delivery workflows.
