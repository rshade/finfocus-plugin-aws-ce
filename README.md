# FinFocus AWS Cost Explorer plugin

`finfocus-plugin-aws-ce` retrieves actual AWS costs through Cost Explorer for
FinFocus. The plugin name is `aws-ce`, and its protocol dependency is
`finfocus-spec v0.7.5`. It advertises actual costs only. Projected cost requests
return gRPC `Unimplemented` with task reference CE-6.10.

## Quick start

Install Go 1.27.1 and use an AWS identity with Cost Explorer read permissions.
Build and install from source:

```bash
git clone https://github.com/rshade/finfocus-plugin-aws-ce.git
cd finfocus-plugin-aws-ce
make build
make install-local
export AWS_PROFILE=default
export AWS_REGION=us-east-1
./bin/finfocus-plugin-aws-ce --port 0
```

Startup prints `PORT=<assigned-port>` to standard output. The plugin writes structured JSON logs
to standard error. Stop the process with Ctrl+C. The `--port` flag overrides the environment,
including an explicit `--port 0` for an automatic port. Invalid command-line ports and batch
settings stop startup with a nonzero exit code.

`make install-local` copies the binary to
`~/.finfocus/plugins/aws-ce/<manifest-version>/`. `make install` is an alias.
Set `FINFOCUS_HOME` to choose another registry directory. Releases contain
Linux, Darwin, and Windows archives plus `checksums.txt`. Releases omit container images. See the [quick-start guide](docs/QUICKSTART.md) for authentication,
core commands, and troubleshooting.

## Credentials and configuration

A request with no host-supplied credentials uses the AWS SDK default credential
chain, including AWS profiles, environment variables, and workload credentials.
Set `AWS_REGION`, for example `us-east-1`, or configure a region in your profile.
Cost Explorer uses a global billing endpoint. This setting doesn't choose a
separate regional billing dataset.

For temporary credentials, supply `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
and `AWS_SESSION_TOKEN` through your normal credential manager. Don't paste
credential values into source files or logs.

| Setting | Purpose | Default |
| --- | --- | --- |
| `FINFOCUS_PLUGIN_PORT` | TCP port, unless overridden by `--port` | Automatic |
| `FINFOCUS_LOG_LEVEL` | `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` | `info` |
| `FINFOCUS_LOG_FILE` | Absolute log filename. Unset uses stderr | Unset |
| `FINFOCUS_AWS_CE_MAX_BATCH_SIZE` | Batch limit, integer from 1 to 1000 | 100 |
| `FINFOCUS_AWS_CE_BATCH_WORKERS` | Concurrent batch workers, integer from 1 to 50 | 10 |
| `FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE` | CE request attempt budget per minute | Unlimited when unset or `0` |

For a log file in the current directory, set
`FINFOCUS_LOG_FILE="$(pwd)/aws-ce.log"`. Empty batch settings use defaults.
Invalid values stop startup and name the setting. Each custom request attempt,
including discovery, pagination and retries, counts toward the request budget.
The CE client turns off SDK retries and uses one budgeted retry loop. Requests
safely share default client initialization. A failed initialization has a
one-second retry delay.

The host may supply `access_key_id` and `secret_access_key` together, optional
`session_token` with that pair, and optional `role_arn` to assume an IAM role.
A role alone uses the default AWS chain for its STS source. Names are
case-insensitive. Unsupported names, incomplete pairs, whitespace-only values
and malformed role ARNs return `InvalidArgument` with
`ERROR_CODE_INVALID_CREDENTIALS`. Invalid credentials never fall back to process
credentials. Credential-bearing requests bypass the shared cache, and the plugin
excludes their values from logs and returned errors. See [AWS credentials](docs/CREDENTIALS.md).

## Actual cost behavior

An EC2 instance id consists of `i-` plus 8 to 17 lowercase hex characters.
An instance id supplied as a bare id or instance ARN uses `GetCostAndUsageWithResources` and filters
`RESOURCE_ID` by the bare id. This requires the account's resource-level
Cost Explorer opt-in and covers the last 14 days. A non-EC2 ARN returns an
explicit unsupported-resource error. Unknown bare resource ids also return an
error. To query totals, set `resource.provider` to `aws` and choose:

| `resource.resource_type` | `resource.id` | Query |
| --- | --- | --- |
| `aws:account` | Any non-empty account query label | All services visible to the credentials |
| `aws:service` | Exact AWS Cost Explorer service name | That service only |

The legacy `resource_id` value `aws-account-total` also selects account totals.
The SDK still requires a non-empty `resource_id` when using a descriptor.
Service and account queries cover the last 14 months.

Queries use Coordinated Universal Time dates, `DAILY` granularity, and an exclusive end date. The
plugin discovers reservation and Savings Plan ids with paginated
`GetDimensionValues` calls. It queries each commitment through a dimension
filter, then queries the remaining charges with those ids excluded. Each
charge belongs to one partition. Commitment partitions use `AmortizedCost`
when AWS returns it. Other charges use `UnblendedCost`. This also applies to
EC2 resource queries. Commitment fields stay unset when AWS supplies no id.
The plugin doesn't invent commitment quantity, status, or invoice details.

Service and resource totals can contain incompatible usage types. The public
remote procedure call omits usage amounts and units for these queries. It preserves the monetary
cost. The client only requests usage quantities when a single `USAGE_TYPE`
filter establishes a common unit. The plugin ignores `GetActualCostRequest.tags`
in v0.1.0.

Cost Explorer has a **24-hour data lag** or longer. Resource-level data may lag
up to 48 hours. Missing data returns an explicit error. Recent or estimated
results stay in the cache for 15 minutes. Closed historical results stay for 24 hours.
Each calculator instance keeps its default cache in memory.
The plugin doesn't reuse disk entries across sessions because the default
credential chain doesn't establish a verified billing identity. Cache keys
include the linked-account filter and the normalized Coordinated Universal Time API dates.
Each CE request costs $0.01 in real use. A query without a cache hit needs at least two
commitment discovery requests and one cost request. Each discovered commitment
and each additional page adds requests. The request budget bounds this work.

## Spec compatibility and FOCUS

On actual-cost requests, the descriptor identity takes precedence over legacy
identifiers. The identity can be an id or ARN. A descriptor without either identity falls back to `resource_id`
and `arn`. The SDK still requires `resource_id` on every request.

Pass `billing_account_id` to receive FOCUS records. The plugin copies that
caller value verbatim. Without it, costs remain available with `focus_record`
unset. The plugin never invents an account. Currency appears in `focus_record.billing_currency`.
When a FOCUS record is present, its `extended_columns` include:

| Key | Value |
| --- | --- |
| `data_source` | `AWS Cost Explorer` |
| `granularity` | `DAILY` |
| `metric` | Metric used, normally `UnblendedCost` |
| `estimated` | `true` if AWS marks any contributing period as an estimate |
| `lookback` | Query limit: `14_days` or `14_months` |
| `amount_decimal` | Exact decimal sum before protocol number conversion |
| `group_key` | Cost Explorer group key |
| `currency` | Currency reported by Cost Explorer |

There is no response-level metadata map. FOCUS and these columns are absent
without a billing account.

## Development and offline testing

```bash
make develop
make test
make test-integration
make lint
make vuln
```

`make develop` fetches Go modules and prepares the build directory.
`make test-integration` runs real plugin processes with local fake CE
endpoints and verifies protocol conformance. All Go test recipes use `-count=1`.
Releases contain binary archives and checksums. The owner removed Docker support and its
Makefile target.

Set `FINFOCUS_E2E=true` to run the subprocess E2E test against a local fake CE
endpoint. Legacy `finfocus_E2E` is a fallback only when the uppercase name is
unset. Explicit uppercase `false` turns off the fallback. Tests inject synthetic
credentials and use local fake endpoints. The ordinary suite never queries
a live AWS service. Contract fixtures prove parsing,
paging, totals, dates, and failure handling. [Live AWS checks](docs/AWS-VERIFICATION.md)
require an explicit build tag and environment flag, and a Pulumi environment
with Cost Explorer read permission. FinFocus core E2E remains outside this run.

The vulnerability check reports the known `GO-2026-6443` finding in
gRPC v1.84.0. The plugin retains the stable dependency. This verification environment
has no fixed stable release. The frozen `specs/` history also has pre-existing
Markdown lint errors. Documentation checks cover changed files separately.

## License and support

Licensed under [Apache 2.0](LICENSE). Report problems in the
[repository issue tracker](https://github.com/rshade/finfocus-plugin-aws-ce/issues).
Release-token maintenance and development rules are in [CLAUDE.md](CLAUDE.md).
