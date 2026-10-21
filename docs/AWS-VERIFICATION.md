# Verify AWS Cost Explorer access

Ordinary tests use local fake endpoints. Live checks require the `awslive`
build tag, `FINFOCUS_AWS_LIVE=true`, and a configured FinFocus core executable.
The Makefile target supplies the build tag after checking the environment gate.
Supply AWS credentials to the test process through your normal credential
manager, such as an AWS profile or a secrets tool that exports environment
variables for one command. Tests exclude credentials from output.

## Run a live comparison

Build the pinned core executable, then run the comparison with authorized credentials:

```bash
make core-install
AWS_PROFILE=<profile> AWS_REGION=us-east-1 \
  FINFOCUS_AWS_LIVE=true AWS_EC2_METADATA_DISABLED=true AWS_MAX_ATTEMPTS=1 \
  make e2e-live
```

Set `FINFOCUS_CORE_BINARY` to use an existing core executable instead.
Don't print credentials to the terminal or logs. The check creates no cloud
resources and changes no IAM policies.

The check uses seven days of daily costs ending two Coordinated Universal Time
days ago. It scopes the comparison to one service, defaulting to
Amazon Simple Storage Service. Set `FINFOCUS_AWS_LIVE_SERVICE` to select another
exact Cost Explorer service name. Two `GetDimensionValues` calls first establish
that this service has no Reserved Instance or Savings Plan IDs in the selected
period. A scope with commitments can't use this comparison: it must fail
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
exhausted request budgets don't establish live cost accuracy. The bounded scope
verifies the residual Unblended-cost path and real core integration. It doesn't
verify live commitment allocation, EC2 resource costs, or every account setup.
Offline fixtures separately exercise commitment partitions and their arithmetic.

## AWS permissions

The comparison needs only `ce:GetDimensionValues` and `ce:GetCostAndUsage`. EC2
resource queries are outside the comparison, and also need
`ce:GetCostAndUsageWithResources` and the resource-data opt-in. The recommendations
check needs the recommendation actions. See [AWS permissions](permissions.md) for
the full table and policy.

The plugin needs no IAM write permission. Missing permission is an explicit
verification blocker, not a passing live comparison.

## Verify recommendations

The recommendations check reads real AWS data and may incur AWS API charges. It
needs `FINFOCUS_AWS_LIVE=true`, the `awslive` build tag, and existing AWS datasets:

```bash
make build
FINFOCUS_AWS_LIVE=true go test -tags awslive ./test/live -run '^TestAWSRecommendations$' -v -count=1
```

The test validates returned records and reports empty datasets explicitly. It
doesn't need the core binary. Ordinary tests, and tagged runs without
`FINFOCUS_AWS_LIVE=true`, never contact AWS for this test. An account with no
eligible usage returns successful empty datasets, which verifies API access and
billing service names but not populated mapping. Offline fixtures cover populated
mapping.

## Enable the manual workflow

Run the live workflow manually from `main` and select its `run_live` input.
It uses OpenID Connect (OIDC) through Pulumi ESC for authentication and remains
off until maintainers configure these repository variables:

| Variable | Required value |
| --- | --- |
| `FINFOCUS_AWS_LIVE_ENABLED` | `true` to permit live execution |
| `PULUMI_ORGANIZATION` | Organization containing the existing environment |
| `PULUMI_TEAM` | Team authorized to obtain the Pulumi OIDC token |
| `PULUMI_ESC_ENVIRONMENT` | Fully qualified name of the existing environment |

The workflow uses `pulumi/auth-actions` for a team token through OIDC, then
`pulumi/esc-action` to inject the environment into the test process. Configure
this access before enabling the workflow. No long-lived AWS keys belong in
GitHub secrets. Fork pull requests receive no live credentials. Ordinary continuous integration
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
real core against a fake endpoint. It never proves live AWS access.

## Verification status

The repository ships the offline core test, the conformance suite, and the gated
live harness. Offline tests establish protocol behavior, not live-account
permissions or billing-data availability. Before relying on a new account, run the
live comparison there and confirm the result matches the current
commitment-discovery path.
