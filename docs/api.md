# API reference

The AWS Cost Explorer plugin serves the `finfocus.v1.CostSourceService` gRPC
interface on a local TCP port. It identifies itself as `aws-ce`, reports spec
version `v0.7.8`, and advertises actual costs, forecasts, and budgets. This
reference describes the current source checkout. See [deployment](deployment.md) to install and run it,
and [configuration](configuration.md) for settings and credentials.

Field names below use Protocol Buffers spelling. JSON clients can also use the
corresponding JSON names, such as `resourceId`. Timestamps use the Protocol
Buffers `Timestamp` type, represented as date-time strings in JSON,
for example `2026-10-01T00:00:00Z`.

## Discovery

### Name

`Name(NameRequest) → NameResponse` accepts an empty request and returns
`name: "aws-ce"`. It doesn't contact AWS.

### GetPluginInfo

`GetPluginInfo(GetPluginInfoRequest) → GetPluginInfoResponse` accepts an empty
request and returns discovery metadata without contacting AWS.

| Response field | Type | Value or meaning |
| --- | --- | --- |
| `name` | string | `aws-ce` |
| `version` | string | Build version. Release builds inject the release version |
| `spec_version` | string | `v0.7.8` |
| `providers` | repeated string | `aws` |
| `capabilities` | repeated `PluginCapability` | Actual costs, forecasts, budgets, and recommendations |
| `metadata` | map of strings | `supported_rpcs` lists `GetActualCost,GetProjectedCost,GetBudgets,GetRecommendations,Supports,GetPluginInfo,BatchCost` |
| `settings` | repeated `PluginSetting` | Plugin settings, environment names, types, and applicable defaults |

The [pinned SDK revision](../go.mod) can differ from the reported protocol
version. AWS credential-chain settings aren't part of `settings`.

### Supports

`Supports(SupportsRequest) → SupportsResponse` checks whether the plugin can
query a resource. The request contains one `resource` descriptor.

| Descriptor field | Type | Behavior |
| --- | --- | --- |
| `provider` | string | Must be exactly `aws` |
| `resource_type` | string | Selects type-only support or an explicit total scope |
| `id` | string | AWS identity or query label |
| `arn` | string | Optional Amazon Resource Name (ARN) |
| `tags` | map of strings | Can carry `pulumi:cloudId` for a Pulumi identity |
| `region`, `sku`, `attributes` | string, string, structured value | Don't select or filter costs |

Without an id or ARN, type checks accept any non-empty type for provider `aws`.
The plugin recognizes `aws:account` and `aws:service` as totals, ignoring case.
Other types report resource-level requirements, with non-EC2 support unverified.
With an identity, the same query planning rules as `GetActualCost` apply. Use the exact lowercase total
types when requesting costs.

| Response field | Type | Behavior |
| --- | --- | --- |
| `supported` | `bool` | Whether the descriptor has a supported query |
| `reason` | string | Rejection reason, resource opt-in requirements, or unverified support |
| `capabilities_enum` | repeated `PluginCapability` | Actual costs, forecasts, budgets, and recommendations |
| `capabilities` | map of `bool` values | Legacy field. Use `capabilities_enum` |
| `supported_metrics` | repeated `MetricKind` | Empty. No sustainability metrics |

Unsupported resources return `supported: false`, not a failed remote procedure
call. This check doesn't contact AWS or confirm permissions, resource existence,
Cost Explorer opt-in, or available billing data.

## GetActualCost

`GetActualCost(GetActualCostRequest) → GetActualCostResponse` retrieves historical
costs visible to the request's AWS credentials.

### Request fields

| Field | Type | Requirement and behavior |
| --- | --- | --- |
| `resource_id` | string | Required, even when `resource` contains an identity |
| `resource` | `ResourceDescriptor` | Optional descriptor. `provider` must be `aws` when present |
| `arn` | string | Optional legacy ARN, used when the descriptor has no identity |
| `start` | `Timestamp` | Required inclusive start. Subject to the query's lookback limit |
| `end` | `Timestamp` | Required exclusive end, strictly after `start` |
| `billing_account_id` | string | Optional caller-supplied account for FOCUS output. Never a cost filter |
| `tags` | map of strings | Ignored. Doesn't enable Cost Explorer tag filtering |
| `dry_run` | `bool` | Ignored by this handler. Setting it doesn't prevent AWS calls |
| `page_size`, `page_token` | int32, string | Ignored. This handler returns all aggregated results |

### Identity and query selection

If the descriptor has an id or ARN, its identity takes precedence over the
legacy fields. Otherwise, the plugin uses `resource_id` and `arn`. For a
descriptor id beginning with `urn:pulumi:`, a non-empty
`resource.tags["pulumi:cloudId"]` supplies the cloud identity. Ordinary descriptor
ids retain precedence over that metadata.

A valid EC2 instance ARN takes precedence over a conflicting id and supplies
the linked-account filter. A malformed ARN can fall back to a usable EC2 id.
A non-EC2 ARN selects a resource query using both the full ARN and its bare
resource name as `RESOURCE_ID` values. The bare name is the text after the first
`:` or `/` in the resource component. The plugin maps `s3`, `rds`, and `lambda`
to Cost Explorer service names and sends other service codes unchanged. These
resource identity forms have no live verification. Missing AWS rows return
`NotFound`, even when `Supports` accepts the resource.

| Query | Identity | AWS operation | Start-time limit |
| --- | --- | --- | --- |
| EC2 instance | Bare `i-` id with 8 to 17 lowercase hex characters, or EC2 instance ARN | `GetCostAndUsageWithResources` | Last 14 days |
| Other AWS resource | Non-EC2 ARN | `GetCostAndUsageWithResources` | Last 14 days |
| Account totals | Descriptor type `aws:account`, any non-empty id, no ARN | `GetCostAndUsage` | Last 14 months |
| Service totals | Descriptor type `aws:service`, exact Cost Explorer service name as id, no ARN | `GetCostAndUsage` | Last 14 months |
| Legacy account totals | `resource_id: "aws-account-total"`, no ARN | `GetCostAndUsage` | Last 14 months |

Service and account totals return one row per service, and per commitment when
one applies, for the whole requested period. They don't return one row per day.
A five-day account query across eight services returns eight rows.

Account totals cover all services visible to the credentials. The account query
label and `billing_account_id` don't restrict that scope. For service totals,
use a name such as `Amazon Simple Storage Service`, not `s3`.

The following JSON illustrates a service query. Replace the timestamps with a
period inside the last 14 months and use your billing account id if you need
FOCUS records:

```json
{
  "resource_id": "Amazon Simple Storage Service",
  "resource": {
    "provider": "aws",
    "resource_type": "aws:service",
    "id": "Amazon Simple Storage Service"
  },
  "start": "2026-10-01T00:00:00Z",
  "end": "2026-10-03T00:00:00Z",
  "billing_account_id": "123456789012"
}
```

### Dates and cost aggregation

Queries use `DAILY` granularity and Coordinated Universal Time (UTC). The plugin
rounds the start down to its UTC calendar day. An end exactly at UTC midnight
stays on that date. A later time rounds up to the next midnight. Use midnight
timestamps to request exact date boundaries.

The plugin discovers reservation and Savings Plan ids through paginated
`GetDimensionValues` calls. It queries disjoint commitment and residual
partitions, using `AmortizedCost` for commitments when available and
`UnblendedCost` for residual charges. It consumes AWS pagination internally.

Results aggregate the query period by service or resource group and commitment
identity. They aren't one result per day. The plugin sums decimal amounts before
converting to protocol numbers. It rejects mixed currencies, missing amounts,
and amounts it can't parse. Usage amounts and units remain unset because these
queries can combine incompatible usage types.

### Response fields

| Field | Type | Behavior |
| --- | --- | --- |
| `results` | repeated `ActualCostResult` | Aggregated cost records |
| `fallback_hint` | `FallbackHint` | `FALLBACK_HINT_NONE` on success |
| `next_page_token` | string | Empty. No client pagination |
| `total_count` | int32 | Unset. Use the length of `results` |
| `dry_run_result` | `DryRunResponse` | Unset |

Each `ActualCostResult` contains:

| Field | Type | Behavior |
| --- | --- | --- |
| `timestamp` | `Timestamp` | Earliest contributing period start |
| `cost` | double | Aggregated monetary amount |
| `source` | string | `aws-ce` |
| `usage_amount`, `usage_unit` | double, string | Zero and empty for these queries |
| `expires_at` | `Timestamp` | Expiry based on the cache policy |
| `focus_record` | `FocusCostRecord` | Present only when the caller supplies `billing_account_id` |

### FOCUS records

The plugin uses the SDK's FOCUS 1.2 builder and copies `billing_account_id`
verbatim. It doesn't discover or invent a billing account. Without that field,
costs still return but currency and other FOCUS metadata aren't available as
separate top-level result fields. With FOCUS output, currency appears in
`focus_record.billing_currency`.

FOCUS records retain EC2 resource identity separately from service identity.
Commitment fields use ids supplied by AWS. The plugin leaves unsupported
commitment quantities, status, and invoice details unset. Its `extended_columns`
include:

| Key | Meaning |
| --- | --- |
| `data_source` | `AWS Cost Explorer` |
| `granularity` | `DAILY` |
| `metric` | Metric used for the result |
| `estimated` | Whether AWS marked any contributing period as estimated |
| `lookback` | `14_days` or `14_months` |
| `amount_decimal` | Exact decimal sum before conversion to double |
| `group_key` | Cost Explorer group identity |
| `currency` | Currency from AWS |

There is no response-level metadata map. A FOCUS translation failure returns an
error instead of an incomplete successful record.

## GetProjectedCost

`GetProjectedCost(GetProjectedCostRequest) → GetProjectedCostResponse` projects
the current UTC calendar month for an account or a service. When
`FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, it also compares Spot with on-demand for
one EC2 instance. The request
requires `resource` with provider `aws` and non-empty `resource_type`.
`dry_run: true` returns `InvalidArgument`.

For an account or a service, the plugin reads two things from Cost Explorer and adds them:

1. Month-to-date `AmortizedCost` from the first of the month up to, but not
   including, today, with `MONTHLY` granularity. This call is skipped on the
   first day of the month.
2. A `GetCostForecast` from today to the first day of the next month, with
   `MONTHLY` granularity, `AMORTIZED_COST`, and an 80 percent prediction
   interval request.

The caller doesn't select dates through this method. `AmortizedCost` spreads
Savings Plan and reservation fees over the usage they cover. `UnblendedCost`
would show the fee as its own charge and covered usage at the on-demand rate.

| Resource | Behavior |
| --- | --- |
| Type `aws:account` | All costs visible to the credentials |
| Type `aws:service` with a non-empty id | Exact Cost Explorer service name in `id` |
| EC2 instance with an instance type and an availability zone | Spot comparison described below when `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, otherwise declined |
| Any other resource, or `aws:service` without an id | Declined with `InvalidArgument` and `ERROR_CODE_INVALID_RESOURCE` |

Cost Explorer cannot forecast one resource, and resource-level data covers 14
days and needs an opt-in on the payer account. An instance id or ARN doesn't
restrict a forecast, so the plugin declines such requests rather than return a
service or account total as that resource's price. Use a plugin that prices
resources from list prices for single resources.

### Spot comparison

The comparison is off by default. With `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`,
an EC2 instance request that includes an instance type and an availability zone
returns a Spot quote instead of a forecast. With it off, the request gets
`InvalidArgument` like any other single resource. The instance type comes from
`attributes.instanceType` or `attributes.instance_type`, then `sku`, then tags.
The zone comes from `attributes.availabilityZone` or `attributes.availability_zone`,
then tags. `attributes` win when the same input is also on a tag. The SDK still
requires `sku` on the request. The operating
system comes from the same sources and defaults to Linux. The request must include `region`.
`dry_run: true` still returns `InvalidArgument`.

The plugin filters `ec2:DescribeSpotPriceHistory` by that instance type, product
description, and availability zone. Linux uses `Linux/UNIX`. The selected
`unit_price` is the hourly Spot price and `pricing_category` is `DYNAMIC`.
`cost_per_month` uses the SDK monthly hours for the request usage profile,
730 hours unless the profile says otherwise.

`pricing:GetProducts` supplies the shared-tenancy on-demand hourly rate for the
same instance type, region, and operating system. When that rate exists,
`on_demand_equivalent_monthly_cost` uses the same hour count, and a `STANDARD`
`price_options` entry named `On-Demand` carries it. `savings_fraction` is
`(spot unit price - on-demand unit price) / spot unit price`. A missing
on-demand product leaves both fields unset. A present zero is a real price.

The Spot response is a quote for a Spot instance, not the cost of an on-demand
instance. FinFocus core takes the first plugin that answers, and at equal
priority `aws-ce` precedes `aws-public`. See
[Spot comparison](../README.md#spot-comparison)
for the routing setting that keeps list prices for projected costs.

Interruption frequency comes from the public Spot advisor feed. Closed buckets
map to `spot_interruption_risk_score` values 0.05, 0.10, 0.15, and 0.20.
The open bucket `>20%` leaves the score at 0 and sets metadata
`spot_interruption_label`. A feed failure still returns the prices and leaves
the score at 0. `FINFOCUS_TEST_MODE=true` skips the feed. Metadata also records `spot_availability_zone` and
`spot_product_description`. `expires_at` is five minutes ahead. The plugin caches
Spot prices and on-demand rates for five minutes, and caches the advisor document
for one hour. Missing Spot history returns `NotFound`.

| Response field | Meaning |
| --- | --- |
| `cost_per_month` | Month-to-date amortized cost plus the forecast mean for the rest of the month |
| `currency` | Currency from the forecast response |
| `prediction_interval_lower`, `prediction_interval_upper` | AWS bounds, only when exactly one row supplies both, plus month-to-date cost |
| `confidence_level` | Requested interval level when that single row supplies bounds |

Several rows leave prediction bounds unset because the plugin doesn't combine
intervals. The handler doesn't use the actual-cost cache. It shares the Cost
Explorer request budget and error mapping and makes at most two requests.
Missing forecast rows return `NotFound`.

## `GetBudgets`

`GetBudgets(GetBudgetsRequest) → GetBudgetsResponse` reads AWS Budgets through
`DescribeBudgets` on its own endpoint. It doesn't create budgets or send alerts.

| Request field | Behavior |
| --- | --- |
| `filter.providers` | Empty or containing exact `aws` selects AWS budgets. Other lists return an empty response |
| `filter.regions`, `filter.resource_types`, `filter.tags` | Ignored |
| `include_status` | Include spend and health when AWS supplies calculated spend |

The account comes from `FINFOCUS_AWS_ACCOUNT_ID`, or STS `GetCallerIdentity`
when that setting is empty. The Budgets and STS clients use the process AWS
credential chain and `us-east-1`. This handler doesn't consume host-supplied
per-request credentials or use the actual-cost cache.

The response has `budgets` and `summary`. Each budget copies its name into `id`
and `name`, sets `source` to `aws-budgets`, and includes `amount.limit`,
`amount.currency`, and the daily, monthly, quarterly, or annual `period`.
With status requested and available, it includes current and forecast spend,
percentages of the limit, currency, and health:

| Health | Rule |
| --- | --- |
| `EXCEEDED` | Current spend exceeds a positive limit |
| `CRITICAL` | Percentage used is at least 90 |
| `WARNING` | Percentage used is at least 75 |
| `OK` | Other cases |

`summary.total_budgets` counts all returned budgets. Health counters only count
budgets with status. The handler doesn't populate budget thresholds or scope
metadata. It consumes AWS pagination internally, with a maximum of 100 pages.
Each page shares the process request budget. Account lookup failure returns
`FailedPrecondition`. Budget API failures use the AWS error mapping, while a
local budget limit or other generic failure returns `Internal`.

## `BatchCost`

`BatchCost(BatchCostRequest) → BatchCostResponse` uses the SDK's concurrent
adapter to call the plugin once per resource.

| Request field | Type | Behavior |
| --- | --- | --- |
| `resources` | repeated `ResourceDescriptor` | Ordered inputs. Empty returns an empty result list |
| `query_type` | `CostQueryType` | Select `COST_QUERY_TYPE_ACTUAL` or `COST_QUERY_TYPE_PROJECTED`. Unspecified selects unsupported estimates |
| `start`, `end` | `Timestamp` | Shared actual-cost period, with the same date limits as individual queries |
| `dry_run` | `bool` | Returns SDK field mappings without AWS calls. Mappings report unsupported fields |

The default limit is 100 resources with 10 workers. See
[batch settings](configuration.md#plugin-settings) for configurable limits.
Oversized batches, invalid descriptors, and invalid actual-cost periods fail
the whole call with `InvalidArgument`.

The response contains `results` in input order and `max_batch_size`. Each result
echoes its `resource` and contains either `cost_data.actual_cost`, `cost_data.projected_cost`, or an `error`
with a gRPC code, message, and flag indicating whether retrying might succeed.
Inspect every result even when the batch call succeeds.

For actual costs, the pinned SDK adapter forwards a legacy id, ARN, tags, and dates. It doesn't
forward the resource descriptor or a billing account id to `GetActualCost`.
Use bare EC2 instance ids, resource ARNs, or the legacy `aws-account-total`
id for batch queries. Use individual `GetActualCost` calls for explicit service
scope, descriptor-based identity resolution, or FOCUS records. A batch's
`resource_type: "aws:account"` alone doesn't enable account totals. Projected
batches forward the complete descriptor and use the forecast scope described
for `GetProjectedCost`. Batch requests don't retrieve budgets.

## GetRecommendations

`GetRecommendations` returns optimization opportunities and account-scoped
anomalies by default. Category `COST` selects rightsizing and Savings Plans.
Category `ANOMALY` or action `INVESTIGATE` selects anomalies. The shared source
`aws-cost-explorer` includes both. Target-scoped requests query optimization
opportunities only. See [recommendation details](../README.md#optimization-recommendations).

Anomaly requests call Cost Explorer `GetAnomalies` for the last 90 days.
Each row uses category `ANOMALY` and action `INVESTIGATE`. The plugin doesn't
score or detect anomalies. Requests for anomalies alone pass the AWS page token
through. Combined requests collect at most 100 anomaly pages and use shared
filtering, sorting, and SDK pagination. Target resources on requests for
anomalies alone return `InvalidArgument` because the call is account scoped. The plugin copies AWS
`TotalImpact` to metadata `total_impact` and leaves `EstimatedSavings` unset,
because that figure records unexpected spend, not a savings estimate. The
plugin sets a confidence score only when the AWS current score is already
between 0 and 1.

## Errors and unsupported operations

A permission or validation failure from AWS carries the AWS error code and the
service and operation that failed, such as
`retrieving costs failed: AccessDeniedException (Cost Explorer GetRightsizingRecommendation)`.
The text never includes the AWS error message, which can hold a role ARN.

Single-resource failures return a gRPC status. Some also attach an `ErrorDetail`
with a protocol error code. Don't assume every failure includes that detail.

| gRPC status | Protocol detail, when attached | Cause |
| --- | --- | --- |
| `InvalidArgument` | `ERROR_CODE_INVALID_RESOURCE` | Missing identity, unsupported provider, or resource |
| `InvalidArgument` | `ERROR_CODE_INVALID_TIME_RANGE` | Missing or reversed dates, exceeded lookback |
| `InvalidArgument` | `ERROR_CODE_INVALID_CREDENTIALS` | Invalid host-supplied credential keys or values |
| `InvalidArgument` | None | AWS request validation failure |
| `PermissionDenied` | `ERROR_CODE_PERMISSION_DENIED` | AWS denies access |
| `Unauthenticated` | `ERROR_CODE_INVALID_CREDENTIALS` | Expired or invalid AWS session |
| `ResourceExhausted` | `ERROR_CODE_RATE_LIMITED` | Local request budget or AWS rate limit |
| `NotFound` | `ERROR_CODE_RESOURCE_NOT_FOUND` | Missing costs or AWS data unavailable |
| `FailedPrecondition` | None | Mixed currencies, or missing region for host-supplied credentials |
| `Internal` | None | Default-client initialization, unexpected AWS failure, amount parsing, or FOCUS translation failure |
| `Unimplemented` | None | Unsupported operation |

`GetPricingSpec` and `EstimateCost` don't supply costs.
Recommendation dismissal and history aren't supported. Direct `DryRun`
returns `InvalidArgument` for forecasts. The SDK returns `Unimplemented` for
operations this plugin doesn't implement.
`ResolveResourceTypes` has no configured mapping registry. Use `Supports` to
check the plugin's accepted inputs. Cost parsing isn't registered.

Missing data is an error, not a successful zero-cost response. Resource queries need
resource-level opt-in. Allow for billing-data delay when choosing dates, and see
[troubleshooting](deployment.md#troubleshooting) for corrective actions.

## Implementation references

- [Request handlers and query selection](../internal/pricing/calculator.go)
- [FOCUS translation](../internal/pricing/aggregate.go)
- [Credential validation and AWS errors](../internal/pricing/credentials.go)
- [Protocol and integration tests](../test/integration/gRPC_test.go)
- [Forecast scope and response mapping](../internal/pricing/projected.go)
- [Budget mapping](../internal/pricing/budgets.go)
- [Anomaly mapping](../internal/pricing/anomalies.go)
