# `finfocus-plugin-aws-ce` development guidelines

Feature plans provide these guidelines. Last updated: 2025-12-10

## Active technologies

- `001-cicd-infrastructure` uses Go 1.25.5, `goreleaser` for cross-platform binary builds, `golangci-lint` v2.6.2 for code quality, `release-please` for automated versioning, and GitHub Actions for continuous integration and delivery workflows.
- `001-cicd-infrastructure` changes configuration and documentation files only.

- `001-aws-ce-plugin` uses Go 1.25.5, `github.com/rshade/finfocus-spec` for the FinFocus plugin SDK, and `github.com/aws/aws-sdk-go-v2` for the AWS Cost Explorer API SDK.

## Project structure

```text
src/
tests/
```

## Commands

- `make test`: run all tests
- `make lint`: run linters
- `make build`: build the plugin binary
- `make ensure`: update dependencies
- `make install`: install plugin to local registry

## Code style

Go 1.25.5: follow standard conventions

## Recent changes

- `001-cicd-infrastructure` added Go 1.25.5, `goreleaser` for cross-platform binary builds, `golangci-lint` v2.6.2 for code quality, `release-please` for automated versioning, and GitHub Actions for continuous integration and delivery workflows.

- `001-aws-ce-plugin` added Go 1.25.5, `github.com/rshade/finfocus-spec` for the FinFocus plugin SDK, and `github.com/aws/aws-sdk-go-v2` for the AWS Cost Explorer API SDK.

## Roadmap and active issues

The project follows these milestones and issues:

### Foundation and continuous integration and delivery for v0.1.0

- **Issue #6**: update dependencies and refactor for SDK compliance with spec v0.5.2, SDK helpers, and `zerolog`.
- **Issue #7**: establish continuous integration and delivery infrastructure with workflows, `goreleaser`, and `release-please`.
- **Issue #11**: implement the core cost plugin from spec 001 and end-to-end testing with AWS integration and workflow secrets.
- **Issue #12**: polish installation and documentation with a `Makefile` version fix and a `README` rewrite.

### Core features for v0.2.0

- **Issue #8**: add AWS Budgets support with a new spec and the `getbudgets` remote procedure call.
- **Issue #9**: add cost forecasting with a new spec and the `GetProjectedCost` remote procedure call.
- **Issue #10**: add anomaly detection with a new spec and anomaly logic.

### Advanced features for v0.3.0

- **Issue #13**: add optimization recommendations for rightsizing and Savings Plans.

## Context for the next agent

- `product.md` contains the master plan and analysis.
- `specs/` contains the completed `001-aws-ce-plugin/spec.md`. Create new specs in this directory for the roadmap issues.
- **Refactoring**: use the `env` and `mapping` helpers in `pluginsdk`, and use `zerolog` when touching any code.

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
