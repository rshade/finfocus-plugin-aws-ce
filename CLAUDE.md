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
make deps       # Update dependencies (go mod tidy && go mod download)
```

Run a single test:

```bash
go test -v -run TestCalculatorName ./internal/pricing/
```

## Architecture

This is a PulumiCost plugin that retrieves **actual costs** from AWS Cost
Explorer. It implements the `pulumicost-spec` plugin SDK interface.

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

### Testing Pattern

Uses `pluginsdk.NewTestPlugin(t, plugin)` for integration tests:

```go
testPlugin := pluginsdk.NewTestPlugin(t, plugin)
testPlugin.TestName("aws-ce")
testPlugin.TestProjectedCost(resource, expectError)
testPlugin.TestActualCost(resourceID, from, to, expectError)
```

## Dependencies

- `github.com/rshade/pulumicost-spec` - Plugin SDK and protobuf definitions
- `github.com/aws/aws-sdk-go-v2` - AWS SDK for Cost Explorer API

## Notes

- Plugin supports `aws` provider only (configured in `NewCalculator()`)
- Cost Explorer is a global AWS service; region doesn't affect data access
- Client initialization is lazy (on first API call)
