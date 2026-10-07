# `CLAUDE.md`

This file provides guidance to Claude Code at `claude.ai/code` when working with
code in this repository.

## Build commands

```bash
make build      # Build plugin binary to bin/finfocus-plugin-aws-ce
make test       # Run all tests
make lint       # Run golangci-lint
make install    # Build and install to ~/.finfocus/plugins/aws-ce/1.0.0/
make fmt        # Format code with go fmt
make ensure     # Update dependencies (alias for deps)
make deps       # Update dependencies (go mod tidy && go mod download)
```

Run a single test:

```bash
go test -v -run TestCalculatorName ./internal/pricing/
```

## Architecture

This is a FinFocus plugin that retrieves **actual costs** from AWS Cost
Explorer. It implements the `finfocus-spec` plugin SDK interface.

### Key components

- `cmd/plugin/main.go` sets up the gRPC server at the plugin entry point
- `internal/pricing/calculator.go` contains the core `Calculator` struct
  - `GetProjectedCost()` returns an error because the plugin supports actual costs only
  - `GetActualCost()` retrieves historical costs from the Cost Explorer API
  - `GetServiceActualCost()` and `GetAccountActualCost()` handle service and account queries
- `internal/client/client.go` wraps the AWS Cost Explorer API client
  - `CostExplorerAPI` interface enables mocking for tests
  - `CostResult` struct represents cost data that the API returns

### Plugin SDK integration

The plugin embeds `pluginsdk.BasePlugin` and uses:

- `pluginsdk.Matcher()` - Resource type matching
- `pluginsdk.Calculator()` - Response builders
- `pluginsdk.NotSupportedError()`, `pluginsdk.NoDataError()` - Standard errors

### SDK compliance for v0.5.2+

The plugin uses standardized SDK helpers for configuration and logging:

**Entry point in `cmd/plugin/main.go`:**

```go
// Initialize logger using SDK helpers
logWriter := pluginsdk.NewLogWriter()
level := parseLogLevel(pluginsdk.GetLogLevel())
logger := pluginsdk.NewPluginLogger("aws-ce", "0.1.0", level, logWriter)


```

Command-line port flags take precedence over the environment. The entry point uses
`flag.Visit` to detect an explicit flag. For `--port 0`, it supplies an
ephemeral TCP listener to `ServeConfig.Listener` so the SDK doesn't read the
environment port again. Invalid ports exit nonzero before serving.

**Remote procedure call handlers in `internal/pricing/calculator.go`:**

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

**Key patterns:**

- Remote procedure call handlers return errors. The entry point exits nonzero for invalid startup configuration.
- SDK validation before business logic - returns standardized error messages
- `LogOperation` for all remote procedure call methods provides timing and structured logging

### Testing pattern

Uses `pluginsdk.NewTestPlugin(t, plugin)` for integration tests:

```go
testPlugin := pluginsdk.NewTestPlugin(t, plugin)
testPlugin.TestName("aws-ce")
testPlugin.TestProjectedCost(resource, expectError)
testPlugin.TestActualCost(resourceID, from, to, expectError)
```

## Dependencies

- `github.com/rshade/finfocus-spec` - Plugin SDK and Protocol Buffers definitions
- `github.com/aws/aws-sdk-go-v2` - AWS SDK for Cost Explorer API

## Notes

- Plugin supports the `aws` provider only, as configured in `NewCalculator()`
- Cost Explorer is a global AWS service. Region doesn't affect data access
- Client initializes on the first API call

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

## Active technologies

- `002-add-arn-spec` uses Go 1.25.5, `finfocus-spec` v0.5.2+ with a required upstream change, and `aws-sdk-go-v2`.
- `002-add-arn-spec` uses a stateless plugin with an optional cache.
- `003-sdk-compliance-refactor` uses Go 1.25.5, `finfocus-spec` v0.5.2, `aws-sdk-go-v2`, and `zerolog`.
- `003-sdk-compliance-refactor` uses a stateless plugin with an optional cache.
- `004-core-cost-e2e` uses Go 1.25.5, `finfocus-spec` v0.5.2+ for the plugin SDK, `aws-sdk-go-v2` for Cost Explorer, and `zerolog` for logging.

## Recent changes

- `002-add-arn-spec` added Go 1.25.5, `finfocus-spec` v0.5.2+ with a required upstream change, and `aws-sdk-go-v2`.

## Release token ownership

Release Please uses the repository secret `RELEASE_PLEASE_TOKEN`. The owner
creates a fine-grained personal access token restricted to this repository,
with Contents, Pull requests and Issues read and write, and Metadata read.
Events created with this token can trigger the release and PR check workflows.

Set an expiration date under the organization's token policy, record that date
in the owner's secret inventory, and rotate before expiry. To rotate, create a
replacement with the same repository access and permissions, update the secret
in repository Settings, then manually run Release Please and check its result.
Revoke the previous token after the replacement succeeds. An expired token
requires replacement. A nonempty expired token doesn't fall back automatically.

Before the first release PR, the owner confirms the secret exists and that a
manual Release Please run succeeds without missing-token or authentication
errors. Agents don't read or set the token. Merge the release PR to create a
plain `vX.Y.Z` tag and release. The release-created event runs `goreleaser`.
