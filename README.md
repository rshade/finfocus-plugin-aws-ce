# aws-ce

PulumiCost plugin for aws-ce cost calculation.

## Overview

This plugin provides cost calculation capabilities for aws resources in PulumiCost. It implements both projected cost estimation and actual cost retrieval functionality.

**Supported Providers:** aws

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

Once installed, the plugin will be automatically discovered by PulumiCost:

```bash
# List installed plugins
pulumicost plugin list

# Validate plugin installation
pulumicost plugin validate

# Calculate projected costs
pulumicost cost projected --pulumi-json plan.json

# Get actual costs
pulumicost cost actual --pulumi-json plan.json --from 2025-01-01
```

## Development

### Prerequisites

- Go 1.21+
- PulumiCost Core development environment
- Cloud provider credentials (for actual cost retrieval)

### Building

```bash
# Build the plugin
make build

# Run tests
make test

# Run linters
make lint
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

The project includes testing utilities from the PulumiCost SDK:

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

The plugin supports the following configuration options:

- Environment variables for cloud provider credentials
- Pricing data files for offline pricing calculations
- Regional pricing variations

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
