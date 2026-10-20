# Verify AWS Cost Explorer access

Ordinary tests use local fake endpoints. Live checks require the `awslive`
build tag, `FINFOCUS_AWS_LIVE=true`, and a configured FinFocus core executable.
The Makefile target supplies the build tag after checking the environment gate.
Use the existing Pulumi Environments, Secrets, and Configuration environment to
supply AWS credentials to child processes. Tests exclude credentials from output.

## Run a live comparison

Build the pinned core executable, then run the comparison with authorized credentials:

```bash
make core-install
pulumi env run tailscale-phase-2/aws-oidc -- env \
  FINFOCUS_AWS_LIVE=true AWS_EC2_METADATA_DISABLED=true AWS_MAX_ATTEMPTS=1 \
  make e2e-live
```

Set `FINFOCUS_CORE_BINARY` to use an existing core executable instead.
Don't use `pulumi env open` to print credentials. The check creates no cloud
resources and changes neither the Pulumi environment nor IAM policies.

The check uses seven days of daily costs ending two Coordinated Universal Time
days ago. It scopes the comparison to one service, defaulting to
Amazon Simple Storage Service. Set `FINFOCUS_AWS_LIVE_SERVICE` to select another
exact Cost Explorer service name. Two `GetDimensionValues` calls first establish
that this service has no Reserved Instance or Savings Plan IDs in the selected
period. A scope with commitments cannot use this comparison: it must fail
explicitly rather than mix incompatible metrics.

For an eligible scope, the baseline requests one `GetCostAndUsage` page using
`UnblendedCost`. The direct plugin call and real core command then retrieve costs
for the same service and period. Checks cover non-empty results, exact decimal
and protocol totals, currency, billing identity, dates, and estimated status
where exposed. The direct protocol checks also verify discovery metadata,
support declarations, invalid-request errors, and clean shutdown.

## Request limits and interpretation

The baseline stops if AWS requires another page. The direct plugin and core
queries each permit at most three Cost Explorer attempts, including retries:
two commitment-discovery requests and one cost request. Together with the two
initial discovery requests and one baseline request, this permits at most nine
Cost Explorer attempts. The check turns off SDK retries. Each direct plugin
query and core process has a 45-second deadline, below the one-minute request
budget reset. The Go test target has a two-minute timeout. Identity requests
are separate. Cost Explorer requests are billable.

Empty results, unexpected commitments, pagination, authorization failures, or
exhausted request budgets do not establish live cost accuracy. The bounded scope
verifies the residual Unblended-cost path and real core integration. It does not
verify live commitment allocation, EC2 resource costs, or every account setup.
Offline fixtures separately exercise commitment partitions and their arithmetic.

## AWS permissions

The existing AWS role needs the following read permissions:

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

`GetDimensionValues` supports commitment discovery; `GetCostAndUsage` supplies
service costs. `GetCostAndUsageWithResources` supports EC2 queries, which are
outside this comparison and also require resource data opt-in. See the
[AWS permission reference](https://docs.aws.amazon.com/service-authorization/latest/reference/list_ce.html).
The plugin needs no IAM write permission or forecasting action. Missing
permission is an explicit verification blocker, not a passing live comparison.

## Enable the manual workflow

Run the live workflow manually from `main` and select its `run_live` input.
It remains off until maintainers configure these repository variables:

| Variable | Required value |
| --- | --- |
| `FINFOCUS_AWS_LIVE_ENABLED` | `true` to permit live execution |
| `PULUMI_ORGANIZATION` | Organization containing the existing environment |
| `PULUMI_TEAM` | Team authorized to obtain the Pulumi OIDC token |
| `PULUMI_ESC_ENVIRONMENT` | Fully qualified name of the existing environment |

The workflow uses `pulumi/auth-actions` for a team token through OIDC, then
`pulumi/esc-action` to inject the environment into the test process. Configure
this access before enabling the workflow. No long-lived AWS keys belong in
GitHub secrets. Fork pull requests receive no live credentials; ordinary continuous integration
runs the fake-endpoint core test and official conformance suite.

## Check the harness without AWS

```bash
go test -count=1 ./test/live
go test -count=1 -tags awslive ./test/live
make core-install
make e2e
make conformance
```

Leave `FINFOCUS_AWS_LIVE` unset for the two Go commands. The tests verify
comparison failures and request limits using synthetic data. `make e2e` runs
real core against a fake endpoint; it never proves live AWS access.

## Recorded verification

On 2026-10-06, `tailscale-phase-2/aws-oidc` supplied an AWS identity. The initial
role policy denied `GetCostAndUsage`, and the plugin returned `PermissionDenied`.
After the owner granted Cost Explorer read permission, a rerun matched nine
service totals for 2026-09-27 through 2026-10-04, including exact decimal sums and
period metadata. This result predates the current commitment-discovery path.
It does not verify EC2 resource data, core integration, or another account.

The 2026-10-19 foundation work implements the offline core test, conformance
integration, and gated live harness. The owner chose offline implementation
because authentication was unavailable. No live AWS calls ran during that
continuation, and there is no new live result. A future authorized run must
verify the current path before marking live acceptance complete.
