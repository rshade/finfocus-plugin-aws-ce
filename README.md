# FinFocus AWS Cost Explorer plugin

`finfocus-plugin-aws-ce` retrieves actual AWS costs through Cost Explorer for
FinFocus. The plugin name is `aws-ce`, and it implements the `finfocus-spec`
v0.7.8 protocol. It advertises actual costs, Cost Explorer forecasts, and optimization recommendations.
`GetProjectedCost` answers for `aws:account` and `aws:service` only. It
returns the projected total for the current Coordinated Universal Time
calendar month: the amortized cost already billed before today plus the Cost
Explorer forecast from today to the last day of the month. Cost Explorer cannot
forecast a single resource, so any other resource gets `InvalidArgument`
instead of a service or account total. The plugin doesn't calculate its own
forecast.

An opt-in setting changes that for one case: with
`FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, an EC2 instance that carries an
instance type and an availability zone gets a Spot price quote. See
[Spot comparison](#spot-comparison).

## Quick start

Install Go 1.27.2 and use an AWS identity with Cost Explorer read permissions.
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

## Documentation

- [API reference](docs/api.md): request fields, query scopes, responses, and errors.
- [Configuration reference](docs/configuration.md): environment variables, flags,
  credentials, batch limits, and cache behavior.
- [Deployment guide](docs/deployment.md): source and release installation,
  AWS permissions, verification, and troubleshooting.

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
| `FINFOCUS_AWS_CE_SPOT_COMPARISON` | `true` answers EC2 instance projections with a Spot quote | `false` |
| `FINFOCUS_HEALTH_ENDPOINT` | Serve SDK `/healthz` on the plugin port. `true` also enables Connect | `false` |
| `FINFOCUS_WEB_ENABLED` | Serve Connect and gRPC-Web beside gRPC on the plugin port | `false` |
| `FINFOCUS_CORS_ALLOWED_ORIGINS` | Comma-separated browser origins. Empty allows none | Empty |
| `FINFOCUS_CORS_ALLOW_CREDENTIALS` | Allow credentialed cross-origin requests | `false` |
| `FINFOCUS_CORS_ALLOWED_HEADERS` | Comma-separated allowed request headers. Empty uses the SDK set | SDK default |
| `FINFOCUS_AWS_ACCOUNT_ID` | Account for AWS Budgets. Empty uses STS `GetCallerIdentity` | Empty |

`FINFOCUS_HEALTH_ENDPOINT=true` turns Connect on even when
`FINFOCUS_WEB_ENABLED=false`. The SDK serves `/healthz` only in Connect mode,
and only on the plugin listener. There is no separate health port. Connect
mode leaves the gRPC trace header unread, so leave both off unless you need
HTTP health checks or a Connect client. `true` and `false` are the only
accepted values. Any other value stops startup and names the setting.
A wildcard origin together with `FINFOCUS_CORS_ALLOW_CREDENTIALS=true`
stops startup. The SDK rejects that combination. An empty entry in a
comma-separated list does too.

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

## Spot comparison

The Spot comparison is off by default. Set `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`
to turn it on. With it on, `GetProjectedCost` for an EC2 instance with an
instance type and an availability zone returns the current Spot price, with the
on-demand rate and an interruption score alongside it. With it off, the same
request returns `InvalidArgument`, and FinFocus core falls back to the next
plugin, such as `aws-public`, which prices the instance at its on-demand rate.

Leave it off when another plugin prices your instances. A Spot quote is not the
price of an on-demand instance. FinFocus core uses the first plugin that
answers a projected-cost request, and at equal priority it asks `aws-ce` before
`aws-public`. With the comparison on, the Spot price can replace the on-demand
list price in a stack total, and cost drift then reads a false difference.
Turn it on for a run that asks what Spot would cost, ideally with routing
that sends only those requests here. The routing example below comes from the
FinFocus core documentation. It hasn't run against this plugin.

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

## AWS account prerequisites

Some features need a setting in the AWS account as well as IAM permission.
Without it AWS returns an access error even when the policy is correct. You make every
setting here in the management (payer) account. Steps follow the AWS
Cost Management documentation, parts of which AWS marks as legacy, so confirm
the labels in your console.

| Feature | Setting | Where |
| --- | --- | --- |
| EC2 instance and other resource costs | Resource-level data at daily granularity | Billing and Cost Management, Cost Management preferences, granular data options |
| Rightsizing recommendations | Rightsizing | Billing and Cost Management, Cost Management preferences, General tab |

- **Resource-level data.** Select resource-level data at daily granularity for
  EC2 or for all services, then save. AWS can take up to 48 hours to apply the
  change and blocks further changes for 48 hours after saving. It covers the
  last 14 days only. Days before AWS starts collecting have no data, and
  the plugin returns `NotFound` for them instead of a zero cost.
- **Rightsizing.** Check Rightsizing and save. AWS can take up to 24 hours to
  produce recommendations. Only a regular or management account can enable it.
- **Savings Plans recommendations.** AWS must already hold a recommendation
  for the account. The plugin never asks AWS to generate one.

The [permissions table](docs/CREDENTIALS.md#permissions-and-failure-handling)
lists the IAM action for each feature. A permission error names the AWS
operation that was denied, for example
`retrieving costs failed: AccessDeniedException (Cost Explorer GetRightsizingRecommendation)`.

## Actual cost behavior

An EC2 instance id consists of `i-` plus 8 to 17 lowercase hex characters.
An instance id supplied as a bare id or instance ARN uses `GetCostAndUsageWithResources` and filters
`RESOURCE_ID` by the bare id. The filter never sends the full EC2 ARN.
This requires the account's resource-level
Cost Explorer opt-in and covers the last 14 days. A non-EC2 ARN uses the same
operation. The plugin maps `s3`, `rds`, and `lambda` to Cost Explorer service
names and sends `RESOURCE_ID` values for both the ARN and the bare name, the
text after the first `:` or `/` in the ARN resource. The plugin sends other
service codes as written. No live check has verified that id form for each
service, and a missing row returns `NotFound`. A bare id that's not an EC2
instance id still needs an ARN. To query totals, set `resource.provider` to
`aws` and choose:

| `resource.resource_type` | `resource.id` | Query |
| --- | --- | --- |
| `aws:account` | Any non-empty account query label | All services visible to the credentials |
| `aws:service` | Exact AWS Cost Explorer service name | That service only |

The legacy `resource_id` value `aws-account-total` also selects account totals.

`Supports` also answers when the host sends a resource type and no id or ARN, as the FinFocus
engine does. The engine accepts any non-empty AWS type. `aws:account` and `aws:service` are totals.
Other types, including non-EC2 resources, report unverified resource-level cost for that
service. Those queries need the resource-level opt-in and cover 14 days. `GetActualCost` still needs an
EC2 instance id, a non-EC2 ARN, or a total label.
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
and `arn`. When a descriptor ID is a Pulumi Uniform Resource Name (URN), the plugin uses a non-empty
`resource.tags["pulumi:cloudId"]` as its cloud identity. Ordinary descriptor IDs
retain precedence over that metadata. This identity metadata doesn't enable
Cost Explorer tag filtering. The SDK still requires `resource_id` on every request.

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

See the [contributor guide](CONTRIBUTING.md) for development setup, code style,
testing, and the pull request process.

```bash
make develop
make test
make test-integration
make lint
make vuln
```

`make develop` fetches Go modules and prepares the build directory.
`make test-integration` launches plugin processes against local fake Cost Explorer
endpoints. All Go test recipes use `-count=1`. Releases contain binary archives
and checksums. Docker support remains outside scope.

Run the real FinFocus core test and official SDK conformance suite with:

```bash
make core-install
make e2e
make conformance
```

`make core-install` builds the pinned core revision. Override
`FINFOCUS_CORE_VERSION` to build another revision, or set `FINFOCUS_CORE_BINARY`
to an existing executable. `make e2e`, also available as `make test-e2e`, uses
an isolated registry, synthetic credentials, and a local fake AWS endpoint.
It asserts actual cost results from core. A successful exit with no data fails.
`make conformance` runs the official Standard suite against the plugin's
advertised capabilities against offline AWS fixtures. The pinned suite doesn't
yet exercise recommendations, so a companion remote procedure call check covers
that capability.
Ordinary tests never query live AWS services.

The SDK dependency pins `v0.7.9-0.20261020013049-bc3a94391af7`, which includes
the [upstream capability-aware conformance fix](https://github.com/rshade/finfocus-spec/pull/652).
The suite validates advertised capabilities and requires `Unimplemented` for
unsupported methods. It retains mandatory checks without fabricated responses.

[Live AWS verification](docs/AWS-VERIFICATION.md) has a separate `make e2e-live`
target, explicit build and environment gates, and bounded AWS requests. It uses
the existing Pulumi environment for credentials. The current foundation has
no new live verification result. The recorded earlier check predates commitment
discovery. Monthly grouping, tag filtering, and persistent cache reuse remain
outside this change.

The vulnerability check reports the known `GO-2026-6443` finding in
gRPC v1.84.0. The plugin retains the stable dependency. This verification environment
has no fixed stable release. Markdown checks cover the repository, including specifications. Run Vale on
each document you edit, as described in the contributor guide.

## License and support

Licensed under [Apache 2.0](LICENSE). Report problems in the
[repository issue tracker](https://github.com/rshade/finfocus-plugin-aws-ce/issues).
Release-token maintenance and development rules are in [CLAUDE.md](CLAUDE.md).

## Optimization recommendations

`GetRecommendations` reads existing AWS Cost Explorer recommendations. The AWS
identity needs `ce:GetRightsizingRecommendation` and
`ce:GetSavingsPlansPurchaseRecommendation`, in addition to the permissions that
other remote procedure calls need. You must enable Cost Explorer, and the account
must already have eligible usage and recommendation datasets. Enable rightsizing
recommendations in AWS and allow AWS time to prepare its dataset. Savings Plans
recommendations must already exist. This plugin never starts recommendation
generation, purchases commitments, or modifies resources. Payer visibility and
AWS account permissions determine which recommendations are available.

An unfiltered request queries EC2 rightsizing and both Compute and EC2 Instance
Savings Plans, plus account-scoped anomalies from the last 90 days. The AWS
identity also needs `ce:GetAnomalies`. Use category `COST` to query only
optimization opportunities. Use category `ANOMALY` or action `INVESTIGATE` to
query only anomalies. Target-scoped requests query optimization opportunities
only. Requests for anomalies alone reject target resources and pass AWS page
tokens through. Unfiltered requests collect up to 100 anomaly pages before
shared filtering, sorting, and pagination. The shared source `aws-cost-explorer`
includes both optimization opportunities and anomalies.

Anomalies retain actual and expected spend, with unexpected spend in metadata
`total_impact`. They don't contribute estimated savings, and projection-period
conversions don't change their impact. The plugin doesn't detect or score
anomalies.

Rightsizing uses `AmazonEC2`, benefits considered, and same-instance-family
targets. The first modify target that AWS flags as `DefaultTargetInstance`
becomes a recommendation. If AWS flags no target, the first target is the
deterministic fallback. The plugin doesn't add alternative targets together.
Savings Plans use payer scope, `THIRTY_DAYS`, `ONE_YEAR`, and `NO_UPFRONT` for
both `COMPUTE_SP` and `EC2_INSTANCE_SP`. Resource tags and request fields can't
override these parameters. AWS supplies savings estimates. Priority is neutral
`MEDIUM`, and confidence, return on investment, and break-even metrics are
absent. AWS may omit the account ID on payer-scoped Savings Plans details. The
plugin then uses resource `account:payer:savings-plan` and omits `account_id`
metadata.

For a running gRPC plugin with reflection enabled, request examples are:

```bash
grpcurl -plaintext -d '{}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"actionType":"RECOMMENDATION_ACTION_TYPE_RIGHTSIZE","accountId":"123456789012","region":"us-east-1"},"projectionPeriod":"annual","pageSize":50}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"actionType":"RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT"}}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
```

Action and category filters select applicable sources before AWS access. EC2
resource filters accept `ec2`, `ec2:Instance`, and `aws:ec2/instance:Instance`.
Savings Plans use `savingsplans:Account`. The plugin sends account filters to
AWS. Region filters apply exactly to mapped records using canonical AWS region
codes. The plugin doesn't send rightsizing `REGION` filters to AWS, because AWS
requires region display names there. Target resource types restrict the selected
AWS sources before credential initialization. Other filters apply to mapped
records. Target resources require provider and type. A supplied stock keeping unit, region,
tags, ID, or ARN constrains the match. A request accepts at most 100 targets.
Recommendation metadata keeps the account identity as `account_id`.

Savings Plans are account-level purchase opportunities, not purchased plan ARNs
or existing EC2 instances. Compute plans without a region can't match regional
or EC2 instance targets. Missing confidence or generation timestamps can't
satisfy filters requiring those fields. The plugin honors excluded
recommendation IDs, but it has no persistent dismissal store.

The default projection is `monthly`. `daily` divides AWS monthly savings by 30,
and `annual` multiplies them by 12. These are display conversions, not new AWS
forecasts. Minimum savings uses the selected period. Original monthly savings
and the conversion convention remain in metadata. Savings across rightsizing
and commitments can overlap. Page totals are potential opportunities, not
guaranteed additive savings.

Pagination returns 50 records per page by default, and the plugin clamps
`pageSize` to a maximum of 1,000 records. Pass the returned `nextPageToken`
unchanged on the next otherwise identical request. Sorting has an ID
tie-breaker. Offset pagination queries AWS again and provides no snapshot
consistency when AWS regenerates data. Summaries describe the returned page
only. Separately, each AWS query follows up to 100 AWS result pages and rejects
repeated tokens. Retries consume the configured request budget. The plugin
doesn't cache results across requests, and per-request credentials and
assume-role settings apply to every selected source.

Empty datasets return a successful empty response. Invalid filters, projections,
targets, and tokens return `InvalidArgument` before AWS access. Permission,
throttling, cancellation, and deadline failures use the existing AWS error
mapping. Unavailable datasets return an availability or precondition error.
Malformed financial data and mixed currencies return `DataLoss`. Any selected
source failure fails the entire call, with no partial result.

Live verification requires explicit opt-in and existing AWS datasets:

```bash
make build
FINFOCUS_AWS_LIVE=true go test -tags awslive ./test/live -run '^TestAWSRecommendations$' -v -count=1
```

This reads real AWS data and may incur AWS API charges. The test validates
returned records and explicitly reports empty datasets. It doesn't require the
core binary. Ordinary tests and tagged runs without `FINFOCUS_AWS_LIVE=true`
never contact AWS for this test. On 2026-10-20, `TestAWSRecommendations` with
`tailscale-phase-2/aws-oidc` received zero EC2 rightsizing rows. Compute and
EC2 Instance Savings Plans each returned a recommendation object with no plan
type and no details. A blank plan type with no details is an empty result. After
that mapping change, the same live check passed with empty datasets for
rightsizing, `TERMINATE`, and Savings Plans.
