# pulumicost-plugin-aws-ce Development Guidelines

Auto-generated from all feature plans. Last updated: 2025-12-10

## Active Technologies

- Go 1.25.5 + github.com/rshade/pulumicost-spec (PulumiCost plugin SDK), github.com/aws/aws-sdk-go-v2 (AWS SDK for Cost Explorer API) (001-aws-ce-plugin)

## Project Structure

```text
src/
tests/
```

## Commands

# Add commands for Go 1.25.5

## Code Style

Go 1.25.5: Follow standard conventions

## Recent Changes

- 001-aws-ce-plugin: Added Go 1.25.5 + github.com/rshade/pulumicost-spec (PulumiCost plugin SDK), github.com/aws/aws-sdk-go-v2 (AWS SDK for Cost Explorer API)

## Roadmap & Active Issues

The project is currently executing against the following milestones and issues:

### v0.1.0 - Foundation & CI/CD
- **Issue #6**: Update Dependencies & Refactor for SDK Compliance (Spec v0.4.6, SDK helpers, Zerolog).
- **Issue #7**: Establish CI/CD Infrastructure (Workflows, Goreleaser, release-please).
- **Issue #11**: Implement Core Cost Plugin (Spec 001) & E2E Testing (AWS Integration, CI Secrets).
- **Issue #12**: Polish: Installation & Documentation (Makefile version fix, README rewrite).

### v0.2.0 - Core Features
- **Issue #8**: Feature: AWS Budgets Support (New Spec, `getbudgets` RPC).
- **Issue #9**: Feature: Cost Forecasting (New Spec, `GetProjectedCost` RPC).
- **Issue #10**: Feature: Anomaly Detection (New Spec, Anomaly logic).

### v0.3.0 - Advanced Features
- **Issue #13**: Feature: Optimization Recommendations (Rightsizing, Savings Plans).

## Context for Next Agent
- **product.md**: Contains the master plan and analysis.
- **specs/**: Contains the completed `001-aws-ce-plugin/spec.md`. New specs should be created in this directory as per the issues above.
- **Refactoring**: Be mindful of `pluginsdk` helpers (`env`, `mapping`) and `zerolog` when touching any code.

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