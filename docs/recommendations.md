# Optimization recommendations

`GetRecommendations` reads recommendations and anomalies that AWS Cost Explorer
already holds. It never starts recommendation generation, purchases commitments,
or modifies resources. The plugin doesn't detect or score anomalies.

The plugin doesn't generate recommendations either, with one exception. When you
set `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, it computes a move-to-Spot
recommendation for each EC2 target resource from live Spot and on-demand prices.
That computed source has the source name `aws-spot-pricing`, so you can tell it
apart from Cost Explorer output. See [Spot recommendations](#spot-recommendations).

For permissions and account settings, see [AWS permissions](permissions.md). For
request and response fields, see the [API reference](api.md#getrecommendations).

## Sources

| Source | AWS operation | IAM action |
| --- | --- | --- |
| EC2 rightsizing | `GetRightsizingRecommendation` | `ce:GetRightsizingRecommendation` |
| Compute Savings Plans and EC2 Instance Savings Plans | `GetSavingsPlansPurchaseRecommendation` | `ce:GetSavingsPlansPurchaseRecommendation` |
| RDS, Redshift, ElastiCache, and OpenSearch reservations | `GetReservationPurchaseRecommendation` | `ce:GetReservationPurchaseRecommendation` |
| Anomalies from the last 90 days | `GetAnomalies` | `ce:GetAnomalies` |
| Move to Spot, opt-in, target resources only | `DescribeSpotPriceHistory`, `GetProducts` | `ec2:DescribeSpotPriceHistory`, `pricing:GetProducts` |

The Cost Explorer sources share the source name `aws-cost-explorer`. The Spot
source uses `aws-spot-pricing`. An unfiltered request queries every Cost Explorer
source. If any selected Cost Explorer source fails, the whole call fails with no
partial result. The Spot source behaves differently: it fails soft for each target.

You must enable Cost Explorer, and the account must already have eligible usage and
recommendation datasets. Enable rightsizing in AWS and allow time for AWS to
prepare its dataset. Savings Plans recommendations must already exist. Payer
visibility and account permissions decide which recommendations are available.

## Choose sources

Narrow a request to avoid IAM permissions and datasets you don't use.

| To query | Set |
| --- | --- |
| Optimization opportunities only | Category `COST` |
| Anomalies only | Category `ANOMALY` or action `INVESTIGATE` |
| Commitment purchases only | Action `RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT` |
| One source | A resource type filter from the table below |

| Resource type filter | Source |
| --- | --- |
| `ec2`, `ec2:Instance`, `aws:ec2/instance:Instance` | EC2 rightsizing |
| `savingsplans:Account` | Both Savings Plans types |
| `rds:Account` | RDS reservations |
| `redshift:Account` | Redshift reservations |
| `elasticache:Account` | ElastiCache reservations |
| `es:Account` | OpenSearch reservations. `opensearch` also maps here |

Short service names and the AWS service codes in the next section are accepted as
aliases. `PURCHASE_COMMITMENT` without a resource type queries both Savings Plans
types and all four reservation services.

Target-scoped requests query optimization opportunities only. With the Spot opt-in on,
they also return a Spot recommendation for each eligible EC2 target, unless the
filter already excludes it (another provider, category, action type other than
`MIGRATE`, or resource type other than `ec2:Instance`), in which case no Spot
AWS calls are made. Requests for
anomalies alone reject target resources and pass AWS page tokens through.
Unfiltered requests collect up to 100 anomaly pages before shared filtering,
sorting, and pagination.

## How each source is queried

These parameters are fixed. Resource tags and request fields can't override them.

**Rightsizing.** Service `AmazonEC2`, benefits considered, and same-instance-family
targets. The plugin copies `TERMINATE` and `MODIFY` savings from AWS. For a modify
recommendation, the first target that AWS flags as `DefaultTargetInstance` becomes
the recommendation. If AWS flags none, the first target is the deterministic
fallback. The plugin doesn't add alternative targets together.

**Savings Plans.** Payer scope, `THIRTY_DAYS` lookback, `ONE_YEAR` term, and
`NO_UPFRONT` payment, for both `COMPUTE_SP` and `EC2_INSTANCE_SP`. AWS supplies the
savings estimate. Priority is the neutral `MEDIUM`, and confidence, return on
investment, and break-even metrics are absent. AWS may omit the account ID on
payer-scoped details. The plugin then uses resource `account:payer:savings-plan`
and omits `account_id` metadata. A blank plan type with no details is an empty
result.

**Reservations.** Payer scope, `THIRTY_DAYS`, `ONE_YEAR`, and `NO_UPFRONT`. EC2
reservations are excluded, because EC2 commitment opportunities come from Savings
Plans. MemoryDB and DynamoDB reservations are out of scope. Requests use AWS
billing service names, and `metadata.service` identifies `AmazonRDS`,
`AmazonRedshift`, `AmazonElastiCache`, or `AmazonES`. OpenSearch uses the legacy
billing service name `Amazon Elasticsearch Service` and `ESInstanceDetails`.

Each reservation detail becomes a `PURCHASE_COMMITMENT` action with commitment type
`reserved_instance` and AWS's recommended quantity. Monthly savings and currency
come from AWS. Upfront cost, recurring monthly cost, break-even months (when AWS
supplies them), and service-specific instance details stay in metadata. The plugin
doesn't calculate these values. Resource IDs such as
`account:123456789012:reserved-instance:AmazonRDS` identify account purchase
opportunities. They don't identify an existing database, cluster, or purchased
reservation. Payer-scoped rows without an account identity are labeled `payer`, and
`account_id` is omitted.

**Anomalies.** Each row uses category `ANOMALY` and action `INVESTIGATE`. The plugin
retains actual and expected spend, and puts the unexpected spend in metadata
`total_impact`. Anomalies contribute no estimated savings, and projection-period
conversions don't change their impact. A confidence score is set only when the AWS
score is already between 0 and 1.

## Spot recommendations

A Spot recommendation tells you to move an on-demand EC2 instance to Spot capacity. It
is computed, not copied from AWS. The plugin quotes the current Spot price, the
on-demand price, and the Spot Instance Advisor interruption bucket, then reports
the difference.

### When the plugin emits one

All of these must hold:

- `FINFOCUS_AWS_CE_SPOT_COMPARISON` is `true`. It is off by default, so a Spot
  quote never replaces an on-demand price you didn't ask about. With it off, the
  response is the same as it was before this feature, and the plugin makes no Spot
  AWS call.
- The request lists the instance in `targetResources`. The plugin has no inventory
  and can't discover instances. For a request with no targets, Cost Explorer
  rightsizing is the source.
- The target is an EC2 instance (`ec2`, `aws:ec2/instance:Instance`, or
  `ec2:Instance`) with an instance type and an availability zone. The plugin reads
  `attributes` first, then `sku`, then tags, the same order as `GetProjectedCost`.
- The region is set, and the operating system is Linux, Windows, RHEL, or SUSE.
  Linux is the default.
- The Price List API has an on-demand row, and the Spot price is below it.

A target that fails any condition produces no recommendation and doesn't fail the
request. An unusable target is logged as a warning.

### Worked example

Request. The target is an instance in `us-east-1a`:

```json
{
  "targetResources": [
    {
      "provider": "aws",
      "resourceType": "aws:ec2/instance:Instance",
      "id": "i-0abc1234def567890",
      "region": "us-east-1",
      "attributes": {
        "instanceType": "m5.large",
        "availabilityZone": "us-east-1a"
      }
    }
  ]
}
```

Suppose Spot costs 0.03 USD per hour, on-demand costs 0.10 USD per hour, and the
advisor reports the `<5%` bucket. Response, trimmed to the relevant fields:

```json
{
  "recommendations": [
    {
      "id": "aws-spot-pricing:<64 hexadecimal characters>",
      "category": "RECOMMENDATION_CATEGORY_COST",
      "actionType": "RECOMMENDATION_ACTION_TYPE_MIGRATE",
      "priority": "RECOMMENDATION_PRIORITY_MEDIUM",
      "source": "aws-spot-pricing",
      "resource": {
        "id": "i-0abc1234def567890",
        "provider": "aws",
        "resourceType": "ec2:Instance",
        "sku": "m5.large",
        "region": "us-east-1"
      },
      "impact": {
        "estimatedSavings": 51.1,
        "currency": "USD",
        "projectionPeriod": "monthly",
        "currentCost": 73,
        "projectedCost": 21.9,
        "savingsPercentage": 70
      },
      "pricingCategory": "FOCUS_PRICING_CATEGORY_DYNAMIC",
      "spotInterruptionRiskScore": 0.05,
      "metadata": {
        "spot_availability_zone": "us-east-1a",
        "spot_product_description": "Linux/UNIX",
        "spot_hourly_price": "0.03",
        "on_demand_hourly_price": "0.1",
        "spot_interruption_label": "<5%",
        "monthly_savings": "51.1"
      }
    }
  ]
}
```

### Field reference

| Field | Value on a Spot recommendation |
| --- | --- |
| `source` | `aws-spot-pricing` |
| `category` | `COST` |
| `actionType` | `MIGRATE` |
| `priority` | `MEDIUM`. The plugin doesn't rank urgency |
| `resource.id` | The target `id`, or the instance ID in the target ARN when `id` is empty |
| `resource.resourceType` | `ec2:Instance`, so it matches the same target filters as rightsizing |
| `resource.sku` | The instance type |
| `resource.tags` | The target's tags |
| `pricingCategory` | `DYNAMIC`, meaning the recommended target state is Spot capacity |
| `spotInterruptionRiskScore` | 0.05, 0.10, 0.15, or 0.20 when the advisor reports a mappable bucket. 0.0 means unknown |
| `impact.currentCost` | The on-demand monthly cost |
| `impact.projectedCost` | The Spot monthly cost |
| `impact.estimatedSavings` | `currentCost - projectedCost` for the selected projection period |
| `impact.savingsPercentage` | `estimatedSavings / currentCost * 100`, so 70 means 70 percent |
| `metadata.account_id` | The account from the target ARN, when the target has one |
| `metadata.spot_interruption_label` | The advisor label, such as `<5%` or `>20%`, when the advisor has one |

Only the Spot builder sets `pricingCategory` and `spotInterruptionRiskScore`.
Rightsizing, Savings Plans, reservation, and anomaly recommendations always leave
`pricingCategory` unspecified and the score at 0.0.

### Impact formula

The plugin uses the same hour count as `GetProjectedCost` for a request with no
usage profile: 730 hours per month.

```text
current cost   = on-demand hourly price x 730
projected cost = Spot hourly price x 730
savings        = current cost - projected cost
percentage     = savings / current cost x 100
```

With Spot at 0.03 and on-demand at 0.10: current 73.0, projected 21.9, savings
51.1, percentage 70.0. The plugin rounds each figure to six decimal places so
floating-point noise doesn't reach the response. `projectionPeriod` rescales
`estimatedSavings` like any other recommendation: `daily` divides the monthly
savings by 30 and `annual` multiplies them by 12. `currentCost` and
`projectedCost` stay monthly.

### Interruption risk

The Spot Instance Advisor reports a bucket for the instance type, region, and
operating system. The plugin maps it like this:

| Advisor bucket | `spotInterruptionRiskScore` | `spot_interruption_label` |
| --- | --- | --- |
| `<5%` | 0.05 | `<5%` |
| `5-10%` | 0.10 | `5-10%` |
| `10-15%` | 0.15 | `10-15%` |
| `15-20%` | 0.20 | `15-20%` |
| `>20%` | 0.0 | `>20%` |
| Feed unavailable or instance type missing | 0.0 | Absent |

A score of 0.0 means the risk is unknown, not that the risk is zero. The SDK uses
0.0 for unknown, so the plugin never sends 0.0 to suggest safety. The `>20%` bucket
has no single value, so the plugin keeps it out of the score and reports the label
in metadata. In every case the category stays `DYNAMIC`, because the
recommendation is still a move to Spot. A failure of the advisor feed never
suppresses the recommendation.

The plugin doesn't rank recommendations by risk. FinFocus core never reorders
them either. Read the score and the label, then decide.

### Stable IDs

The ID is `aws-spot-pricing:` followed by a SHA-256 hash of the source, resource
ID, region, availability zone, instance type, operating system, and action type.
Prices and risk are not inputs. A dismissal therefore survives a price change, and
the same suggestion keeps its ID across calls. A different zone, instance type, or
operating system produces a new ID. Duplicate targets produce one recommendation.

### Failure behavior

Spot recommendations fail soft. If a lookup fails for a target, such as a throttled
`DescribeSpotPriceHistory` or a missing price, the plugin logs a warning, and skips
that target. The call still returns the other targets and the Cost Explorer
recommendations. This differs from the Cost Explorer sources, where one failure
fails the whole call.

A target is also skipped when:

- the Price List API has no on-demand row, because the savings can't be computed;
- the Spot price isn't below the on-demand price;
- the recommendation fails `pluginsdk.ValidateRecommendation`, which also logs an
  error.

### Freshness

Spot lookups run one target at a time. They don't count against
`FINFOCUS_AWS_CE_MAX_REQUESTS_PER_MINUTE`, which covers Cost Explorer and Budgets
calls only. Spot prices change minute to minute. The plugin caches Spot and on-demand prices
for five minutes and the advisor document for one hour. `Recommendation` has no
expiry field, so treat the figures as a snapshot from the last few minutes, and
ask again before acting on them.

### Limitations

- Targets only. The plugin doesn't find instances for you.
- EC2 only. It doesn't recommend Spot for RDS, containers, or other services.
- One zone per target. It prices the zone you give and never picks the cheapest
  zone.
- Shared tenancy on-demand prices only. It doesn't derive the on-demand price from
  advisor data.
- It recommends the same instance type on Spot. It doesn't suggest a different
  family or size, and it doesn't check workload fitness. Spot capacity can be
  interrupted with two minutes of notice.
- Savings overlap with other recommendations. Spot savings and a Savings Plan or
  rightsizing saving for the same instance aren't additive.
- The plugin never buys, launches, or changes anything.

### Required permissions

The Spot source needs `ec2:DescribeSpotPriceHistory` and `pricing:GetProducts` in
addition to the Cost Explorer actions. It makes no new calls beyond those that
`GetProjectedCost` already makes. See [AWS permissions](permissions.md).

## Filters

The plugin sends account filters to AWS. Reservations use the `AccountId`
request field, with no positive dimension filter expressions. Region filters apply exactly
to the AWS region values in mapped records. The plugin doesn't send rightsizing
`REGION` filters to AWS, because AWS requires region display names there. Other
filters apply to mapped records.

- Target resources require a provider and a type. A supplied stock keeping unit,
  region, tags, ID, or ARN constrains the match. A request accepts at most 100
  targets.
- Target resource types restrict the selected AWS sources before credential
  initialization.
- Savings Plans are account-level purchase opportunities, not purchased plan ARNs or
  existing EC2 instances. Compute plans without a region can't match regional or
  EC2 instance targets.
- Missing confidence or generation timestamps can't satisfy filters that require
  those fields.
- The plugin honors excluded recommendation IDs, but it has no persistent dismissal
  store.
- Recommendation metadata keeps the account identity as `account_id`.

## Projection periods and totals

The default projection is `monthly`. `daily` divides AWS monthly savings by 30, and
`annual` multiplies them by 12. These are display conversions, not new AWS
forecasts. Minimum savings uses the selected period. Original monthly savings and
the conversion convention remain in metadata.

Savings across rightsizing and commitments can overlap. Page totals are potential
opportunities, not guaranteed additive savings.

## Pagination

Pagination returns 50 records per page by default, and the plugin clamps `pageSize`
to a maximum of 1,000. Pass the returned `nextPageToken` unchanged on the next
otherwise identical request. Sorting has an ID tie-breaker. Offset pagination
queries AWS again and gives no snapshot consistency when AWS regenerates data.
Summaries describe the returned page only.

Each AWS query follows up to 100 AWS result pages and rejects repeated tokens.
Retries consume the configured request budget. The plugin doesn't cache results
across requests, and per-request credentials and assume-role settings apply to every
selected source.

## Errors

| Condition | Result |
| --- | --- |
| Empty dataset | Successful empty response |
| Invalid filter, projection, target, or token | `InvalidArgument`, before any AWS access |
| Permission, throttling, cancellation, or deadline failure | The standard AWS error mapping |
| Unavailable dataset | An availability or precondition error |
| Malformed financial data or mixed currencies | `DataLoss` |
| Any selected Cost Explorer source fails | The whole call fails |
| The Spot lookup fails for a target | The target is skipped and the rest returns |

## Example requests

With a running plugin that has gRPC reflection enabled:

```bash
grpcurl -plaintext -d '{}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"actionType":"RECOMMENDATION_ACTION_TYPE_RIGHTSIZE","accountId":"123456789012","region":"us-east-1"},"projectionPeriod":"annual","pageSize":50}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"actionType":"RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT"}}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"resourceType":"rds:Account","accountId":"123456789012","region":"us-east-1"}}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
```

To check against a real account, see [live AWS verification](AWS-VERIFICATION.md).
