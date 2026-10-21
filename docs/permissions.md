# AWS permissions

This page is the single reference for the IAM permissions, account settings, and
network access that each `aws-ce` feature needs. Grant only the features you use.
For credential sources and host-supplied credentials, see
[AWS credentials](CREDENTIALS.md).

## Permissions by feature

| Feature | IAM actions | Also needs |
| --- | --- | --- |
| Account and service actual costs | `ce:GetCostAndUsage`, `ce:GetDimensionValues` | Cost Explorer enabled |
| EC2 and other resource actual costs | `ce:GetCostAndUsageWithResources`, `ce:GetDimensionValues` | [Resource-level data opt-in](#account-settings) |
| Account and service projected costs | `ce:GetCostForecast`, `ce:GetCostAndUsage` | Cost Explorer enabled |
| EC2 Spot comparison (opt-in) | `ec2:DescribeSpotPriceHistory`, `pricing:GetProducts` | Outbound HTTPS to `spot-bid-advisor.s3.amazonaws.com` |
| Budgets | `budgets:ViewBudget` | Process credentials only. See [Budgets](#budgets) |
| Anomalies | `ce:GetAnomalies` | Anomaly monitors in the account |
| EC2 rightsizing recommendations | `ce:GetRightsizingRecommendation` | [Rightsizing enabled](#account-settings) |
| Savings Plans recommendations | `ce:GetSavingsPlansPurchaseRecommendation` | An existing AWS recommendation |
| Non-EC2 reservation recommendations | `ce:GetReservationPurchaseRecommendation` | RDS, Redshift, ElastiCache, or OpenSearch usage |
| Assume a host-supplied `role_arn` | `sts:AssumeRole` on the source identity | A [trust policy](#assume-a-role) on the target role |

`GetRecommendations` with no filter queries rightsizing, both Savings Plans types,
four reservation services, and anomalies. It fails as a whole if any one of them is
denied, so grant all four recommendation actions together or narrow the request
with a [resource type filter](recommendations.md#choose-sources). `GetCallerIdentity`
in STS needs no IAM permission.

Cost Explorer, Budgets, and Price List are billing services, so their IAM actions
accept only `"Resource": "*"`.

## Full policy

This policy covers every feature. Remove statements you don't need.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CostExplorerActuals",
      "Effect": "Allow",
      "Action": [
        "ce:GetCostAndUsage",
        "ce:GetCostAndUsageWithResources",
        "ce:GetDimensionValues",
        "ce:GetCostForecast"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CostExplorerRecommendations",
      "Effect": "Allow",
      "Action": [
        "ce:GetAnomalies",
        "ce:GetRightsizingRecommendation",
        "ce:GetSavingsPlansPurchaseRecommendation",
        "ce:GetReservationPurchaseRecommendation"
      ],
      "Resource": "*"
    },
    {
      "Sid": "Budgets",
      "Effect": "Allow",
      "Action": "budgets:ViewBudget",
      "Resource": "*"
    },
    {
      "Sid": "SpotComparison",
      "Effect": "Allow",
      "Action": [
        "ec2:DescribeSpotPriceHistory",
        "pricing:GetProducts"
      ],
      "Resource": "*"
    }
  ]
}
```

For a read-only costs installation, keep only the `CostExplorerActuals` statement.

## Assume a role

Set `role_arn` in the host-supplied credentials when the plugin should query
through a role in the billing (payer) account.

1. **Source identity.** The identity that starts the plugin, or the host-supplied
   key pair, needs permission to assume the role:

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Action": "sts:AssumeRole",
         "Resource": "arn:aws:iam::111122223333:role/finfocus-aws-ce"
       }
     ]
   }
   ```

2. **Target role permissions.** Attach the [full policy](#full-policy), or the
   subset you need, to the role.
3. **Target role trust policy.** The role must trust the source identity:

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Effect": "Allow",
         "Principal": { "AWS": "arn:aws:iam::444455556666:role/finfocus-runner" },
         "Action": "sts:AssumeRole"
       }
     ]
   }
   ```

Replace the account ids and role names with your own. A role alone, without keys,
uses the default AWS credential chain as the STS source.

## Budgets

`GetBudgets` always uses the process credential chain in `us-east-1`. It ignores
host-supplied keys and `role_arn`. A role-assumed cost query can therefore succeed
while budgets fail with `PermissionDenied`: grant `budgets:ViewBudget` to the
process identity.

The account comes from `FINFOCUS_AWS_ACCOUNT_ID`. When that is empty, the plugin
calls STS `GetCallerIdentity` with the process credentials.

## Account settings

IAM permission isn't enough for some features. Make these settings in the
management (payer) account. AWS Cost Management marks parts of the console as
legacy, so confirm the labels in your console.

| Feature | Setting | Where |
| --- | --- | --- |
| Resource actual costs | Daily resource-level data for EC2 or all services | Billing and Cost Management, Cost Management preferences, granular data options |
| Rightsizing recommendations | Rightsizing | Billing and Cost Management, Cost Management preferences, General tab |
| Anomalies | At least one anomaly monitor | Billing and Cost Management, Cost Anomaly Detection |

- **Resource-level data.** AWS can take up to 48 hours to apply the change and
  blocks further changes for 48 hours after saving. It covers the last 14 days
  only. Days before AWS starts collecting have no data, and the plugin returns
  `NotFound` for them instead of a zero cost.
- **Rightsizing.** AWS can take up to 24 hours to produce recommendations. Only a
  regular or management account can enable it.
- **Savings Plans recommendations.** AWS must already hold a recommendation. The
  plugin never asks AWS to generate one.

## Network access

| Destination | Used by | Notes |
| --- | --- | --- |
| Cost Explorer endpoint | All cost, forecast, anomaly, and recommendation calls | Global service. Region doesn't select a dataset |
| `budgets.amazonaws.com` | Budgets | `us-east-1` |
| STS endpoint | Role assumption, budget account lookup | |
| Price List endpoint in `us-east-1` | Spot comparison | On-demand hourly rate |
| EC2 endpoint in the instance's region | Spot comparison | Spot price history |
| `spot-bid-advisor.s3.amazonaws.com` | Spot comparison | Public JSON feed for the interruption score. Needs no credentials |

The Spot advisor feed is the only non-AWS-API call. If egress blocks it, the Spot
comparison can't return an interruption score.

## Permission errors

A permission error names the AWS operation that was denied, for example
`retrieving costs failed: AccessDeniedException (Cost Explorer GetRightsizingRecommendation)`.
Match the operation against the preceding table. See
[AWS credentials](CREDENTIALS.md#failure-handling) for how AWS errors map to gRPC
status codes.
