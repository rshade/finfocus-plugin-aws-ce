# FinFocus AWS Cost Explorer plugin

`finfocus-plugin-aws-ce` (plugin name `aws-ce`) retrieves **actual AWS costs** from
AWS Cost Explorer for [FinFocus](https://github.com/rshade/finfocus). It
implements the `finfocus-spec` v0.7.9 protocol over gRPC.

## What it does

| Capability | Behavior |
| --- | --- |
| Actual costs | Account totals, per-service totals, and per-resource costs, as FOCUS 1.2 records when you pass a billing account |
| Projected costs | Month-to-date amortized cost plus the Cost Explorer forecast, for `aws:account` and `aws:service` |
| Budgets | Reads AWS Budgets and reports spend and health status |
| Recommendations | EC2 rightsizing, Savings Plans, reservation purchases, and cost anomalies that AWS already holds |
| Batch costs | Fans one request out over many resources and shares Cost Explorer queries between them |
| Spot comparison | Opt-in. Quotes the Spot price for an EC2 instance, with an on-demand rate and interruption score |

The plugin reads data only. It never creates budgets, purchases commitments, or
changes resources, and it doesn't calculate its own forecasts. Cost Explorer can't
forecast a single resource, so `GetProjectedCost` returns `InvalidArgument` for
anything other than an account or service.

## Quick start

You need Go 1.27.2 and an AWS identity with Cost Explorer read permission. See
[AWS permissions](docs/permissions.md) for the exact IAM policy.

```bash
git clone https://github.com/rshade/finfocus-plugin-aws-ce.git
cd finfocus-plugin-aws-ce
make build
make install-local
export AWS_PROFILE=default
export AWS_REGION=us-east-1
./bin/finfocus-plugin-aws-ce --port 0
```

Startup prints `PORT=<assigned-port>` to standard output, and the plugin writes
structured JSON logs to standard error. Stop it with Ctrl+C.

`make install-local` copies the binary to
`~/.finfocus/plugins/aws-ce/<manifest-version>/`. Set `FINFOCUS_HOME` to use
another registry. Each release publishes Linux, macOS, and Windows archives with
`checksums.txt`. There is no container image. For release installation, see the
[deployment guide](docs/deployment.md), and for a first query, the
[quick-start guide](docs/QUICKSTART.md).

## AWS permissions at a glance

Grant only the features you use. The [permissions reference](docs/permissions.md)
has the full least-privilege policy, role assumption, account settings, and network
requirements.

| Feature | IAM actions |
| --- | --- |
| Account and service costs | `ce:GetCostAndUsage`, `ce:GetDimensionValues` |
| Resource costs | `ce:GetCostAndUsageWithResources`, `ce:GetDimensionValues` |
| Projected costs | `ce:GetCostForecast`, `ce:GetCostAndUsage` |
| Spot comparison | `ec2:DescribeSpotPriceHistory`, `pricing:GetProducts` |
| Budgets | `budgets:ViewBudget` |
| Anomalies | `ce:GetAnomalies` |
| Rightsizing | `ce:GetRightsizingRecommendation` |
| Savings Plans recommendations | `ce:GetSavingsPlansPurchaseRecommendation` |
| Reservation recommendations | `ce:GetReservationPurchaseRecommendation` |
| Assume a host-supplied role | `sts:AssumeRole` |

Some features also need a setting in the AWS management account:

- **Resource costs** need resource-level data at daily granularity. It covers the
  last 14 days.
- **Rightsizing** must be enabled in Cost Management preferences.
- **Savings Plans recommendations** must already exist in AWS. The plugin never asks
  AWS to generate them.

## Configuration

A request with no host-supplied credentials uses the AWS SDK default credential
chain: profiles, environment variables, SSO, and workload credentials. Set
`AWS_REGION` or a profile region. Cost Explorer is a global service, so the region
doesn't choose a billing dataset.

The plugin reads these settings. Variables are optional. See the
[configuration reference](docs/configuration.md) for formats, limits, and
interactions.

| Variable | Purpose | Default |
| --- | --- | --- |
| `FINFOCUS_PLUGIN_PORT` | TCP port, unless `--port` overrides it | Automatic |
| `FINFOCUS_LOG_LEVEL` | `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic` | `info` |
| `FINFOCUS_LOG_FILE` | Absolute log path. Unset uses standard error | Unset |
| `FINFOCUS_LOG_FILE_PERMISSIONS` | Octal creation mode for a new log file | `0644` |
| `FINFOCUS_AWS_CE_MAX_BATCH_SIZE` | Largest batch, 1 to 1000 | `100` |
| `FINFOCUS_AWS_CE_BATCH_WORKERS` | Concurrent batch workers, 1 to 50 | `50` |
| `FINFOCUS_AWS_CE_BATCH_WINDOW_MS` | How long resource lookups wait to share one query. `0` queries each alone | `50` |
| `FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE` | Cost Explorer and Budgets request budget per minute | Unlimited |
| `FINFOCUS_AWS_CE_MAX_WAIT_SECONDS` | Longest a request queues for the budget or a throttle cooldown. `0` fails at once | `90` |
| `FINFOCUS_AWS_CE_SPOT_COMPARISON` | `true` answers EC2 instance projections with a Spot quote | `false` |
| `FINFOCUS_AWS_ACCOUNT_ID` | Account for Budgets. Empty uses STS `GetCallerIdentity` | Empty |
| `FINFOCUS_HEALTH_ENDPOINT` | Serve `/healthz` on the plugin port. Also turns on Connect | `false` |
| `FINFOCUS_WEB_ENABLED` | Serve Connect and gRPC-Web beside gRPC | `false` |
| `FINFOCUS_CORS_ALLOWED_ORIGINS` | Comma-separated browser origins. Empty allows none | Empty |
| `FINFOCUS_CORS_ALLOW_CREDENTIALS` | Allow credentialed cross-origin requests | `false` |
| `FINFOCUS_CORS_ALLOWED_HEADERS` | Comma-separated allowed request headers | SDK default |

Invalid values stop startup and name the setting. `GetPluginInfo` advertises these
settings to the host.

The host can also supply `access_key_id`, `secret_access_key`, an optional
`session_token`, and an optional `role_arn` for each request. Invalid sets return
`InvalidArgument` with `ERROR_CODE_INVALID_CREDENTIALS`, and never fall back to
process credentials. See [AWS credentials](docs/CREDENTIALS.md).

## Spot comparison

The Spot comparison is off by default. With `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`,
`GetProjectedCost` for an EC2 instance with an instance type and an availability zone
returns the current Spot price, with the on-demand rate and an interruption score.
With it off, the same request returns `InvalidArgument`, and FinFocus core falls back
to the next plugin, such as `aws-public`, which prices the instance at its on-demand
rate.

Leave it off when another plugin prices your instances. FinFocus core uses the first
plugin that answers a projected-cost request, and at equal priority it asks `aws-ce`
before `aws-public`. With the comparison on, a Spot price can replace the on-demand
list price in a stack total, and cost drift then reads a false difference. Turn it
on for a run that asks what Spot would cost, ideally with routing that sends only
those requests here. This example comes from the FinFocus core documentation and
hasn't run against this plugin:

```yaml
routing:
  plugins:
    - name: aws-public
      features: [ProjectedCosts]
      priority: 20
    - name: aws-ce
      features: [ActualCosts, Recommendations]
      priority: 10
```

The comparison also fetches the public Spot Instance Advisor feed over HTTPS. See
[network access](docs/permissions.md#network-access).

## Things to know about the data

- **Billing delay.** Cost Explorer data lags by 24 hours or longer, and
  resource-level data by up to 48 hours. Missing data returns an explicit error, not
  a zero cost.
- **Query range.** Account and service queries cover the last 14 months. Resource
  queries cover the last 14 days.
- **Cost.** Each Cost Explorer request costs $0.01. A query with no cache hit needs
  at least two commitment-discovery requests and one cost request. Set
  `FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE` to bound spend.
- **Caching.** Results are cached in memory for each plugin process. Recent or
  estimated results expire after 15 minutes, and closed history after 24 hours.
  Requests with host-supplied credentials bypass the cache.
- **FOCUS.** Pass `billing_account_id` to receive FOCUS records. The plugin copies
  that value verbatim and never invents an account.
- **Tags.** The plugin ignores `GetActualCostRequest.tags`.

## Documentation

| Guide | Contents |
| --- | --- |
| [Quick start](docs/QUICKSTART.md) | Build, authenticate, and run a first query |
| [Deployment guide](docs/deployment.md) | Source and release installation, verification, and troubleshooting |
| [AWS permissions](docs/permissions.md) | IAM policy, role assumption, account settings, and network access |
| [AWS credentials](docs/CREDENTIALS.md) | Credential chain, host-supplied credentials, and error mapping |
| [Configuration reference](docs/configuration.md) | Environment variables, flags, limits, and cache behavior |
| [API reference](docs/api.md) | Request fields, query scopes, responses, and errors |
| [Recommendations](docs/recommendations.md) | Sources, filters, parameters, pagination, and failure semantics |
| [Live AWS verification](docs/AWS-VERIFICATION.md) | Opt-in checks against a real account |
| [Contributing](CONTRIBUTING.md) | Development setup, code style, and the pull request process |

## Development

```bash
make develop
make test
make test-integration
make lint
make vuln
```

`make test-integration` launches plugin processes against local fake Cost Explorer
endpoints. Ordinary tests never query live AWS. To run the real FinFocus core test
and the official SDK conformance suite:

```bash
make core-install
make e2e
make conformance
```

`make core-install` builds the pinned core revision. Override `FINFOCUS_CORE_VERSION`
to build another revision, or set `FINFOCUS_CORE_BINARY` to an existing executable.
See the [contributor guide](CONTRIBUTING.md) for the rest.

## License and support

Licensed under [Apache 2.0](LICENSE). Report problems in the
[issue tracker](https://github.com/rshade/finfocus-plugin-aws-ce/issues).
