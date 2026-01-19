# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Build Commands

```bash
make build      # Build plugin binary to bin/pulumicost-plugin-aws-ce
make test       # Run all tests
make lint       # Run golangci-lint
make install    # Build and install to ~/.pulumicost/plugins/aws-ce/1.0.0/
make fmt        # Format code with go fmt
make ensure     # Update dependencies (alias for deps)
make deps       # Update dependencies (go mod tidy && go mod download)
```

Run a single test:

```bash
go test -v -run TestCalculatorName ./internal/pricing/
```

## Architecture

This is a PulumiCost plugin that retrieves **actual costs** from AWS Cost
Explorer. It implements the `finfocus-spec` plugin SDK interface.

### Key Components

- **cmd/plugin/main.go**: Plugin entry point, sets up gRPC server
- **internal/pricing/calculator.go**: Core `Calculator` struct
  - `GetProjectedCost()` - Returns error (actual costs only)
  - `GetActualCost()` - Retrieves historical costs from Cost Explorer API
  - `GetServiceActualCost()`, `GetAccountActualCost()` - Service/account queries
- **internal/client/client.go**: AWS Cost Explorer API client wrapper
  - `CostExplorerAPI` interface enables mocking for tests
  - `CostResult` struct represents cost data returned from API

### Plugin SDK Integration

The plugin embeds `pluginsdk.BasePlugin` and uses:

- `pluginsdk.Matcher()` - Resource type matching
- `pluginsdk.Calculator()` - Response builders
- `pluginsdk.NotSupportedError()`, `pluginsdk.NoDataError()` - Standard errors

### SDK Compliance (v0.5.2+)

The plugin uses standardized SDK helpers for configuration and logging:

**Entry Point (`cmd/plugin/main.go`):**

```go
// Initialize logger using SDK helpers
logWriter := pluginsdk.NewLogWriter()
level := parseLogLevel(pluginsdk.GetLogLevel())
logger := pluginsdk.NewPluginLogger("aws-ce", "1.0.0", level, logWriter)

// Determine port: CLI flag takes precedence over environment variable
port := pluginsdk.ParsePortFlag()
if port == 0 {
    port = pluginsdk.GetPort()
}
```

**RPC Handlers (`internal/pricing/calculator.go`):**

```go
func (c *Calculator) GetActualCost(ctx context.Context, req *pbc.GetActualCostRequest) (*pbc.GetActualCostResponse, error) {
    // Log operation timing using SDK helper
    done := pluginsdk.LogOperation(c.logger, "GetActualCost")
    defer done()

    // Validate request using SDK validation helper
    if err := pluginsdk.ValidateActualCostRequest(req); err != nil {
        return nil, status.Error(codes.InvalidArgument, err.Error())
    }
    // ... implementation
}
```

**Key Patterns:**

- No `log.Fatal` or `os.Exit` calls - use `logger.Error()` + return
- SDK validation before business logic - returns standardized error messages
- LogOperation for all RPC methods - provides timing and structured logging

### Testing Pattern

Uses `pluginsdk.NewTestPlugin(t, plugin)` for integration tests:

```go
testPlugin := pluginsdk.NewTestPlugin(t, plugin)
testPlugin.TestName("aws-ce")
testPlugin.TestProjectedCost(resource, expectError)
testPlugin.TestActualCost(resourceID, from, to, expectError)
```

## Dependencies

- `github.com/rshade/finfocus-spec` - Plugin SDK and protobuf definitions
- `github.com/aws/aws-sdk-go-v2` - AWS SDK for Cost Explorer API

## Notes

- Plugin supports `aws` provider only (configured in `NewCalculator()`)
- Cost Explorer is a global AWS service; region doesn't affect data access
- Client initialization is lazy (on first API call)

## Plugin SDK Reference (v0.5.2)

The `pluginsdk` package (`github.com/rshade/finfocus-spec/sdk/go/pluginsdk`) provides standardized helpers that **MUST** be used.

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

## Active Technologies
- Go 1.25.5 + finfocus-spec v0.5.2+ (requires upstream change), aws-sdk-go-v2 (002-add-arn-spec)
- N/A (stateless plugin, optional cache) (002-add-arn-spec)
- Go 1.25.5 + finfocus-spec v0.5.2, aws-sdk-go-v2, zerolog (003-sdk-compliance-refactor)
- N/A (stateless plugin with optional cache) (003-sdk-compliance-refactor)

## Recent Changes
- 002-add-arn-spec: Added Go 1.25.5 + finfocus-spec v0.5.2+ (requires upstream change), aws-sdk-go-v2
