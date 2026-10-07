# Verify AWS Cost Explorer access

Ordinary tests use local fake endpoints. Live checks require the `awslive`
build tag and `FINFOCUS_AWS_LIVE=true`. Neither alone enables AWS calls.
Use a Pulumi Environments, Secrets, and Configuration environment that supplies AWS credentials. Credentials stay
in the child process environment. The tests exclude them from output.

```bash
pulumi env run tailscale-phase-2/aws-oidc -- env \
  FINFOCUS_AWS_LIVE=true AWS_EC2_METADATA_DISABLED=true AWS_MAX_ATTEMPTS=1 \
  go test -count=1 -tags awslive -run '^TestAWSActualCost$' -v ./test/live
```

Replace the environment name when verifying another account. Don't use
`pulumi env open` to print credentials. The check creates no cloud resources
and doesn't change the Pulumi environment or IAM policies.

The check reads STS identity, then requests seven days of service costs ending
two Coordinated Universal Time days ago. It compares one independent Cost Explorer
`AmortizedCost` page with a newly built plugin served over gRPC. The comparison
sums daily baseline groups and plugin commitment partitions by service for the
window. It adds every partition rather than overwriting repeated service names.
The comparison checks period boundaries, exact decimal sums, protocol
amounts, currency, caller billing account, and `Estimated`. It also verifies Supports,
GetPluginInfo, invalid-request status and clean shutdown.

The baseline stops if the response requires another page. The plugin has a
20-attempt request budget to cover Reserved Instance and Savings Plan discovery,
their cost partitions, the residual cost query, and any pagination or plugin retries.
The check turns off AWS SDK retries. Cost Explorer requests are billable. This
check can make at most 21 CE attempts: one baseline and 20 plugin attempts within
the 40-second check deadline. Identity requests are separate. A successful comparison requires grouped billing data in the
selected period. An empty or incomplete response doesn't prove cost accuracy.

[AWS describes amortized costs](https://docs.aws.amazon.com/cost-management/latest/userguide/ce-advanced.html)
as spreading upfront and recurring commitment fees over their applicable period.
Comparing the plugin's mixed metrics with the global amortized baseline assumes
that discovery finds all relevant commitments and that residual charges have equal
Unblended and amortized amounts. The check tests that assumption against the
selected account and period. A mismatch fails the comparison. it doesn't switch
metrics or treat unmatched fees as successful verification. Offline partition
tests don't prove this equivalence for every AWS fee or account configuration.

## AWS permissions

Attach this policy to the IAM role used by the Pulumi environment:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "FinFocusCostExplorerReadOnly",
      "Effect": "Allow",
      "Action": [
        "ce:GetDimensionValues",
        "ce:GetCostAndUsage",
        "ce:GetCostAndUsageWithResources"
      ],
      "Resource": "*"
    }
  ]
}
```

The plugin requires `GetDimensionValues` for commitment discovery and
`GetCostAndUsage` for service costs. The resource action
supports EC2 resource-cost queries, which are outside this check and also
require resource data opt-in. See the [AWS permission reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ce.html).
The plugin needs no IAM write permission or forecasting action.

If AWS returns AccessDeniedException, the check verifies the plugin returns
PermissionDenied, then reports `BLOCKED-ON-INPUT` and skips cost comparison.
This outcome proves the error mapping, not live cost amounts. Grant the read
permission to the role, allow policy propagation, and rerun the command.

Compile and test the comparison without live access:

```bash
go test -count=1 ./test/live
go test -count=1 -tags awslive ./test/live
```

With FINFOCUS_AWS_LIVE unset, both commands are offline. The unit tests reject
wrong amounts, periods, accounts, currency, estimates, and missing or duplicate
partitions. They also verify that Reserved Instance, Savings Plan, and residual
rows for the same service contribute to both exact and protocol totals. Synthetic
comparison data is separate from live AWS evidence.

## Recorded verification

On 2026-10-06, `tailscale-phase-2/aws-oidc` supplied a valid AWS identity. The
initial role policy denied GetCostAndUsage, and the plugin returned
PermissionDenied. The owner granted Cost Explorer read permission. A rerun
then matched nine service totals for 2026-09-27 through 2026-10-04, including
exact decimal sums and period metadata. This result doesn't verify EC2
resource-level data, FinFocus core E2E, or another account.

The audit fixes add commitment discovery to actual-cost queries. The earlier
live result in this document predates that query path. Run the gated live check again with
the updated policy to verify the new path against AWS.
