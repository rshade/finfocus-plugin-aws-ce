# aws-ce

FinFocus plugin for aws-ce cost calculation.

## Overview

This plugin provides cost calculation capabilities for aws resources in FinFocus. It retrieves actual costs from AWS Cost Explorer. Projected cost requests return gRPC `Unimplemented` (CE-6.10).

**Supported Providers:** aws

## Actual cost

An EC2 instance id (`i-` plus 8 to 17 lowercase hex characters), from an instance ARN or a bare resource id, is queried with `GetCostAndUsageWithResources` and `RESOURCE_ID` set to that id, never the full ARN. Other ids, including `contract-*`, stay on unfiltered `GetCostAndUsage` grouped by service. Resource-level data covers the last 14 days and needs the Cost Explorer resource-level opt-in. A non-EC2 ARN returns an error instead of a guessed id.

The query metric is `UnblendedCost`. `AmortizedCost` is used only for a row whose requested group key is a reservation id or a savings plan ARN and that metric is present. `BlendedCost` is never used. `GetActualCost` does not group by reservation or savings plan, so those FOCUS fields stay unset. That is a gap, not a stub. Quantity and status are not invented. `GetReservationUtilization` and `GetSavingsPlansCoverage` are not called.

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

### Testing

The project includes testing utilities from the FinFocus SDK:

```go
func TestPluginName(t *testing.T) {
    plugin := pricing.NewCalculator()
    testPlugin := pluginsdk.NewTestPlugin(t, plugin)
    testPlugin.TestName("aws-ce")
}
```

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

## Spec compatibility

The plugin uses `finfocus-spec v0.7.5`. On actual-cost requests, the id or ARN in a resource
descriptor takes precedence over legacy identifiers. When the
descriptor has neither, the plugin falls back to `resource_id` and `arn`.
The SDK still requires `resource_id` on every request.

Pass `billing_account_id` to receive FOCUS records. The plugin uses that value
verbatim for the FOCUS billing account. Without it, costs are returned with
`focus_record` unset, as required by the spec; no billing account is invented.

## Actual-cost metadata

When the caller supplies `billing_account_id`, FOCUS `extended_columns` include:

| Key | Value |
| --- | --- |
| `data_source` | `AWS Cost Explorer` |
| `granularity` | `DAILY` |
| `metric` | The cost metric used, normally `UnblendedCost` |
| `estimated` | `true` if any contributing AWS period is estimated, otherwise `false` |
| `lookback` | Query limit: `14_days` for resource data, `14_months` for service totals |
| `amount_decimal` | Exact decimal sum before conversion to the RPC cost number |
| `group_key` | Cost Explorer group key |
| `currency` | Currency reported by Cost Explorer |

Currency also appears in `focus_record.billing_currency`. There is no
response-level metadata map. Without a billing account, FOCUS and its extended
columns are absent and the cost remains available.

## Batch configuration

The SDK serves `BatchCost` using concurrent per-resource actual-cost calls.
Set `FINFOCUS_AWS_CE_MAX_BATCH_SIZE` to an integer from 1 to 1000 (default 100)
and `FINFOCUS_AWS_CE_BATCH_WORKERS` to an integer from 1 to 50 (default 10).
Unset or empty values use the defaults. Invalid values stop startup with a
configuration error naming the variable. Each CE page still consumes the
configured per-minute request budget. Default client initialization is shared
safely; initialization failures have a one-second retry delay.

## Data freshness and offline testing

Cost Explorer data lags by 24 hours or more. Resource-level data may lag up to
48 hours. Recent or estimated results are cached for 15 minutes; historical
closed results are cached for 24 hours. Each paginated CE request costs $0.01
in real use. This run verifies local contract fixtures and fake endpoints only.

Set `FINFOCUS_E2E=true` to run the subprocess E2E test with a local fake CE
endpoint. The legacy `finfocus_E2E` name remains a fallback when the uppercase
name is unset. An explicit uppercase `false` disables the fallback. The E2E
test injects synthetic credentials and never queries a live AWS service.
Real-account and FinFocus core E2E verification remain blocked on credentials.

## Per-request AWS credentials

The host may supply `access_key_id` and `secret_access_key` together, an optional
`session_token` with that pair, and optional `role_arn` to assume an IAM role.
A role alone uses the default AWS credential chain as its STS source. Names are
case-insensitive. Unsupported names, incomplete pairs, whitespace-only values
and malformed role ARNs return `InvalidArgument` with
`ERROR_CODE_INVALID_CREDENTIALS`. Invalid credentials never fall back to the
process chain. Credential-bearing requests bypass the shared cache, and their
values are excluded from logs and returned errors.

See [AWS credentials](docs/CREDENTIALS.md) for permissions, supported shapes
and error handling. These names belong to this plugin; the spec intentionally
keeps credential names free-form.
