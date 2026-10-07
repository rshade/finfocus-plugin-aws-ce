# Quick start

## Build and install

Install Go 1.27.1, then run from the repository root:

```bash
make build
make install-local
```

The installed path uses the version from `manifest.json`. Set `FINFOCUS_HOME`
when using a separate FinFocus registry. Published releases provide archives
for Linux, Darwin, and Windows with `checksums.txt`. Unpack the binary into the
same versioned registry directory. For an unreleased source checkout, use the preceding source build.

## Authenticate

Select an existing AWS profile and region:

```bash
export AWS_PROFILE=default
export AWS_REGION=us-east-1
```

You can also use environment credentials or workload credentials through the
AWS SDK default chain. Temporary credentials need all three AWS key, secret
and session-token variables. See [AWS credentials](CREDENTIALS.md) for exact
host-supplied keys and IAM permissions. Enable Cost Explorer and its
resource-level opt-in before querying individual EC2 instances.

## Run the plugin

```bash
./bin/finfocus-plugin-aws-ce --port 0
```

Wait for `PORT=<assigned-port>` on standard output. The process accepts gRPC requests
at that local port and emits structured logs on stderr. Ctrl+C or SIGTERM
cancels the server and shuts it down. Use `--port 50051` for a fixed port.
The command-line flag takes precedence over `FINFOCUS_PLUGIN_PORT`, including zero.

For normal use, let an installed FinFocus core discover and start the binary:

```bash
finfocus plugin list
finfocus plugin validate --plugin aws-ce
```

The repository includes [a sample Pulumi plan](../examples/plan.json) with
synthetic EC2 identity values. Replace its id and ARN with an existing instance
from your account before requesting live costs. A newly previewed resource
has no billed usage yet. Choose Coordinated Universal Time dates within the last 14 days and allow
at least 24 hours for billing data, or up to 48 hours for resource-level data.

For example, on Linux with the `date` command from `GNU coreutils`:

```bash
START_DATE=$(date -u -d '7 days ago' +%F)
END_DATE=$(date -u -d '2 days ago' +%F)
finfocus cost actual --pulumi-json examples/plan.json --from "$START_DATE" --to "$END_DATE"
```

On macOS use `date -u -v-7d +%F` and `date -u -v-2d +%F` to assign those dates.
The end date is exclusive. The core commands match the sibling core source.
This run doesn't build or execute core or perform live-account verification.
Each paginated Cost Explorer request costs $0.01 in real use.

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Missing AWS region | Set `AWS_REGION=us-east-1` or configure your profile's region |
| `PermissionDenied` | Grant `ce:GetDimensionValues`, `ce:GetCostAndUsage` and, for EC2, `ce:GetCostAndUsageWithResources` |
| `Unauthenticated` | Refresh expired session credentials and include `AWS_SESSION_TOKEN` |
| `InvalidArgument` with `ERROR_CODE_INVALID_CREDENTIALS` | Check supported key names, matching key pair, and role ARN |
| Resource query has no data | Confirm resource opt-in, instance id, 14-day window and 24-to-48-hour data lag |
| Unsupported non-EC2 ARN | This service lacks resource attribution. Don't substitute a guessed id |
| `ResourceExhausted` | Wait for the rate limit to reset or reduce CE page requests |
| `GetProjectedCost` returns `Unimplemented` | This release serves actual costs. Use a plugin with projected-cost support |
| Invalid startup port or batch setting | Use port 0 to 65535 and the documented batch limits |

The plugin returns explicit errors for invalid requests, missing data, AWS
failures and unsupported resource queries. It doesn't turn failures into
successful zero-cost responses.

## Verify without AWS access

```bash
make test-integration
FINFOCUS_E2E=true go test -count=1 -v ./test/e2e/...
```

These commands use synthetic credentials and local fake CE endpoints. They
prove the protocol and request handling, including a real server's discovery
remote procedure calls, actual costs, and shutdown. They don't verify permissions or billed
amounts for a real AWS account. These tests require no AWS account.
