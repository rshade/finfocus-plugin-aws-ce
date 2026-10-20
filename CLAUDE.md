# `CLAUDE.md`

This file provides guidance to Claude Code at `claude.ai/code` when working with
code in this repository.

For behavior changes, use OpenSpec: propose, apply, verify, and archive. Current
tested behavior lives in `openspec/specs/`. The `specs/` and `.specify/`
directories preserve frozen Spec Kit history; do not add new work there.

## Build commands

```bash
make build      # Build plugin binary to bin/finfocus-plugin-aws-ce
make test       # Run all tests
make lint       # Run golangci-lint
make install    # Build and install to ~/.finfocus/plugins/aws-ce/0.1.0/
make fmt        # Format code with go fmt
make ensure     # Update dependencies (alias for deps)
make deps       # Update dependencies (go mod tidy && go mod download)
```

Installation uses the version in `manifest.json`, whose value is `0.1.0`. Override
`PLUGIN_VERSION` or `FINFOCUS_HOME` when using another registry directory.

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
  - `GetProjectedCost()` serves `aws:account` and `aws:service` only and declines other resources with `InvalidArgument`. It returns month-to-date `AmortizedCost` (`ce:GetCostAndUsage`) plus the `ce:GetCostForecast` to the first of next month, and shifts the AWS interval bounds by the month-to-date amount
    An EC2 instance with an instance type and zone takes the Spot comparison path in `spot.go`, which also calls EC2,
    the Price List API, and the Spot Instance Advisor feed
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
logger := pluginsdk.NewPluginLogger("aws-ce", version.Version, level, logWriter)


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

Integration tests build the plugin and query its real gRPC server through
`internal/testutil.StartPlugin`. The custom checks in `test/conformance` cover
protocol validation and use `pluginsdk.ValidateFocusRecord` for FOCUS records.
`make conformance` runs the pinned upstream Standard suite against offline AWS
fixtures, with a companion recommendation remote procedure call check.

```bash
make test-integration
```

## Spec v0.7.8 features

- `GetPluginInfo` advertises plugin settings from `internal/pricing/settings.go`. Add an entry there when the plugin reads a new
  environment variable. A test checks the list with `ValidatePluginSettings`.
- Never add `Parse`, the cost-parsing remote procedure call, to this plugin. `aws-public` owns `Parse` for the `aws` provider.
- The plugin also doesn't adopt `GetRecommendationHistory` or the `ParsedResource` action hint. They serve projected-cost and recommendation plugins, not an actual-cost plugin.
- With `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, an EC2 `GetProjectedCost` with an instance type and an availability zone is a Spot comparison. It is off by default so a Spot quote never replaces an on-demand price.
  Account and service requests stay on `ce:GetCostForecast`. Any other single resource returns `InvalidArgument`.
  Read `attributes` before `sku` and tags. `DescribeSpotPriceHistory` filters instance type, product description, and zone.
  Linux uses `Linux/UNIX`. `pricing:GetProducts` in `us-east-1` supplies the shared-tenancy on-demand hourly rate.
  `on_demand_equivalent_monthly_cost` and the `STANDARD` `On-Demand` price option use the same hour count as `cost_per_month`.
  `savings_fraction` is `(spot unit price - on-demand unit price) / spot unit price`. Leave the on-demand fields unset when the price list has no row.
  Advisor `r` maps to `0.05`, `0.10`, `0.15`, and `0.20`. The `>20%` bucket leaves the score at 0 and keeps the label in metadata.
  Don't derive the on-demand price from advisor field `s`, and don't pick the cheapest zone.
  Spot and on-demand rates cache for five minutes. The advisor document caches for one hour.

## Dependencies

- `github.com/rshade/finfocus-spec` - Plugin SDK and Protocol Buffers definitions
- `github.com/aws/aws-sdk-go-v2` - AWS SDK for Cost Explorer API

## Notes

- Plugin supports the `aws` provider only, as configured in `NewCalculator()`
- Cost Explorer is a global AWS service. Region doesn't affect data access
- `GetRecommendations` combines optimization opportunities and anomalies. Category `ANOMALY` selects `ce:GetAnomalies` for the last 90 days
- Vale and reviewdog skip `CHANGELOG.md` because Release Please generates it
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

## AWS recommendations for issue #13

`GetRecommendations` implements EC2 rightsizing and Compute/EC2 Instance Savings
Plans together. See `specs/005-aws-ce-recommendations-plugin/spec.md` and README
for defaults, filters, permissions, and limitations. Keep SDK validation,
selection, pagination, summary helpers, and `zerolog` operation logging. Preserve
request-scoped credentials, retry budgets, the 100-page AWS cap, and all-source
failure semantics. Never generate recommendations, purchase plans, or mutate
resources. Ordinary tests use offline fixtures. The `awslive` recommendation
test also requires `FINFOCUS_AWS_LIVE=true`. Don't run it without an explicit
request for real AWS verification. Live verification remains pending.
