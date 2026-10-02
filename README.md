# aws-ce

FinFocus plugin for aws-ce cost calculation.

## Overview

This plugin provides cost calculation capabilities for aws resources in FinFocus. It implements both projected cost estimation and actual cost retrieval functionality.

**Supported Providers:** aws

## Actual cost

An EC2 instance id (`i-` plus 8 to 17 lowercase hex characters), from an instance ARN or a bare resource id, is queried with `GetCostAndUsageWithResources` and `RESOURCE_ID` set to that id, never the full ARN. Other ids, including `contract-*`, stay on unfiltered `GetCostAndUsage` grouped by service. Resource-level data covers the last 14 days and needs the Cost Explorer resource-level opt-in. A non-EC2 ARN returns an error instead of a guessed id.

Rows labelled "No resource ID" are not a service total.

`GetActualCostRequest.tags` are ignored in v0.1.0.

## Installation

### From Source

1. Clone the repository:

   ```bash
   git clone <repository-url>
   cd aws-ce
   ```

2. Build the plugin:

   ```bash
   make build
   ```

3. Install to local plugin registry:

   ```bash
   make install
   ```

### Configuration

The plugin may require cloud provider credentials to function properly. See the configuration section for details.

## Usage

Once installed, the plugin will be automatically discovered by FinFocus:

```bash
# List installed plugins
finfocus plugin list

# Validate plugin installation
finfocus plugin validate

# Calculate projected costs
finfocus cost projected --pulumi-json plan.json

# Get actual costs
finfocus cost actual --pulumi-json plan.json --from 2025-01-01
```

## Development

### Prerequisites

- Go 1.27.1
- FinFocus Core development environment
- Cloud provider credentials (for actual cost retrieval)

### Building

```bash
# Build the plugin
make build

# Run tests
make test

# Run linters
make lint

# Update dependencies
make ensure

# Install to local registry
make install
```

### Project Structure

- `cmd/plugin`: Plugin entry point
- `internal/pricing`: Pricing logic and calculators
- `internal/client`: Cloud provider client implementation
- `examples`: Example usage
- `bin`: Compiled binaries

### Implementing Pricing Logic

Edit `internal/pricing/calculator.go` to implement your pricing logic:

```go
func (c *Calculator) GetProjectedCost(ctx context.Context, req *pbc.GetProjectedCostRequest) (*pbc.GetProjectedCostResponse, error) {
    // 1. Check if resource is supported
    if !c.Matcher().Supports(req.Resource) {
        return nil, pluginsdk.NotSupportedError(req.Resource)
    }

    // 2. Extract resource properties
    resourceType := req.Resource.ResourceType
    properties := req.Resource.Tags

    // 3. Calculate pricing based on resource type and properties
    unitPrice := c.calculateResourceCost(resourceType, properties)

    // 4. Return response
    return c.Calculator().CreateProjectedCostResponse("USD", unitPrice, "description"), nil
}
```

#### Actual Cost Retrieval

Edit `internal/client/client.go` to implement cloud provider API integration:

```go
func (c *Client) GetResourceCost(ctx context.Context, resourceID string, startTime, endTime int64) (float64, error) {
    // 1. Call cloud provider billing API
    // 2. Parse response and calculate total cost
    // 3. Return cost value
    return totalCost, nil
}
```

### Testing

The project includes testing utilities from the FinFocus SDK:

```go
func TestPluginName(t *testing.T) {
    plugin := pricing.NewCalculator()
    testPlugin := pluginsdk.NewTestPlugin(t, plugin)
    testPlugin.TestName("aws-ce")
}
```

### Adding Pricing Data

1. Update pricing data structures in `internal/pricing/data.go`
2. Implement pricing lookups in `internal/pricing/calculator.go`
3. Add test cases for new resource types

### Configuration

#### Environment Variables

Configure the plugin using standard FinFocus environment variables:

```bash
# AWS credentials. A request that carries none uses the standard AWS SDK chain.
# The host may pass credentials on the request instead.
export AWS_REGION=us-east-1
export AWS_ACCESS_KEY_ID=your-key
export AWS_SECRET_ACCESS_KEY=your-secret

# Optional: Plugin configuration
export FINFOCUS_PLUGIN_PORT=50051        # Specific port (default: auto-assign)
export FINFOCUS_LOG_FILE=/var/log/finfocus-aws-ce.log  # Log to file (default: stderr)
export FINFOCUS_LOG_LEVEL=debug          # Verbosity: debug|info|warn|error (default: info)
```

`FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE` caps Cost Explorer calls per minute, counting each page; unset or `0` means no limit.

#### CLI Flags

The `--port` flag overrides the environment variable:

```bash
# Use environment variable port
./finfocus-plugin-aws-ce

# Override with CLI flag (takes precedence)
./finfocus-plugin-aws-ce --port 50052
```

#### Log Output

Logs use structured JSON format with standard fields:

```json
{
  "level": "info",
  "component": "finfocus-plugin-aws-ce",
  "plugin_name": "aws-ce",
  "plugin_version": "1.0.0",
  "operation": "GetActualCost",
  "duration_ms": 1234,
  "message": "Operation completed"
}
```

#### Graceful Shutdown

The plugin responds cleanly to shutdown signals (SIGINT, SIGTERM):

```bash
# Start plugin in background
./bin/finfocus-plugin-aws-ce &
PID=$!

# Send SIGTERM for graceful shutdown
kill -TERM $PID
# Plugin completes in-flight requests before exiting
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Run `make lint test`
6. Submit a pull request

## License

[Add your license information here]

## Support

[Add support contact information here]
