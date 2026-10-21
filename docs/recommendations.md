# Optimization recommendations

`GetRecommendations` reads recommendations and anomalies that AWS Cost Explorer
already holds. It never starts recommendation generation, purchases commitments,
or modifies resources. The plugin doesn't generate recommendations, detect
anomalies, or score them.

For permissions and account settings, see [AWS permissions](permissions.md). For
request and response fields, see the [API reference](api.md#getrecommendations).

## Sources

| Source | AWS operation | IAM action |
| --- | --- | --- |
| EC2 rightsizing | `GetRightsizingRecommendation` | `ce:GetRightsizingRecommendation` |
| Compute Savings Plans and EC2 Instance Savings Plans | `GetSavingsPlansPurchaseRecommendation` | `ce:GetSavingsPlansPurchaseRecommendation` |
| RDS, Redshift, ElastiCache, and OpenSearch reservations | `GetReservationPurchaseRecommendation` | `ce:GetReservationPurchaseRecommendation` |
| Anomalies from the last 90 days | `GetAnomalies` | `ce:GetAnomalies` |

All sources share the source name `aws-cost-explorer`. An unfiltered request
queries every source. If any selected source fails, the whole call fails with no
partial result.

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

Target-scoped requests query optimization opportunities only. Requests for
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
| Any selected source fails | The whole call fails |

## Example requests

With a running plugin that has gRPC reflection enabled:

```bash
grpcurl -plaintext -d '{}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"actionType":"RECOMMENDATION_ACTION_TYPE_RIGHTSIZE","accountId":"123456789012","region":"us-east-1"},"projectionPeriod":"annual","pageSize":50}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"actionType":"RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT"}}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
grpcurl -plaintext -d '{"filter":{"resourceType":"rds:Account","accountId":"123456789012","region":"us-east-1"}}' localhost:PORT finfocus.v1.CostSourceService/GetRecommendations
```

To check against a real account, see [live AWS verification](AWS-VERIFICATION.md).
