# Verify AWS Cost Explorer access

Ordinary tests use local fake endpoints. Live checks require the `awslive`
build tag and `FINFOCUS_AWS_LIVE=true`; neither alone enables AWS calls.
Use a Pulumi ESC environment that supplies AWS credentials. Credentials stay
in the child process environment and are excluded from test output.

```bash
pulumi env run tailscale-phase-2/aws-oidc -- env \
  FINFOCUS_AWS_LIVE=true AWS_EC2_METADATA_DISABLED=true AWS_MAX_ATTEMPTS=1 \
  go test -count=1 -tags awslive -run '^TestAWSActualCost$' -v ./test/live
```

Replace the environment name when verifying another account. Do not use
`pulumi env open` to print credentials. The check creates no cloud resources
and does not change the Pulumi environment or IAM policies.

The check reads STS identity, then requests seven days of service costs ending
two UTC days ago. It compares one Cost Explorer page with a newly built plugin
served over gRPC. Daily groups are summed into one row per service for the
window. The comparison checks period boundaries, exact decimal sums, RPC
amounts, currency, caller billing account and Estimated. It also verifies Supports,
GetPluginInfo, invalid-request status and clean shutdown.

The baseline stops if another page is required. The plugin has a one-page
request budget and AWS SDK retries are disabled. Cost Explorer requests are
billable; this check can make two CE page attempts. Identity requests are
separate. A successful comparison requires grouped billing data in the
selected period; an empty or incomplete response does not prove cost accuracy.

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
        "ce:GetCostAndUsage",
        "ce:GetCostAndUsageWithResources"
      ],
      "Resource": "*"
    }
  ]
}
```

GetCostAndUsage is required by the service-cost comparison. The second action
supports EC2 resource-cost queries, which are outside this check and also
require resource data to be enabled. See the [AWS permission reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ce.html).
No IAM write permission or forecasting action is needed by the plugin.

If AWS returns AccessDeniedException, the check verifies the plugin returns
PermissionDenied, then reports BLOCKED-ON-INPUT and skips cost comparison.
This outcome proves the error mapping, not live cost amounts. Grant the read
permission to the role, allow policy propagation, and rerun the command.

Compile and test the comparison without live access:

```bash
go test -count=1 ./test/live
go test -count=1 -tags awslive ./test/live
```

With FINFOCUS_AWS_LIVE unset, both commands are offline. The unit tests reject
wrong amounts, periods, accounts, currency, estimates and missing or duplicate
rows. Synthetic comparison data is separate from live AWS evidence.

## Recorded verification

On 2026-10-06, `tailscale-phase-2/aws-oidc` supplied a valid AWS identity. The
initial role policy denied GetCostAndUsage, and the plugin returned
PermissionDenied. The owner granted Cost Explorer read permission. A rerun
then matched nine service totals for 2026-09-27 through 2026-10-04, including
exact decimal sums and period metadata. This result does not verify EC2
resource-level data, FinFocus core E2E or another account.
