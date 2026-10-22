## Intent and success criteria

Build issue #29's historical AWS emissions report for sustainability leads.
The user explicitly requires a configuration/environment opt-in. Success means
a caller can enable the feature and obtain AWS-reported monthly aggregates
without resource attribution, duplicate totals, lost provenance or fabricated
zero values. Ordinary cost users must not gain a Sustainability dependency.

This is an architectural feature: the existing ImpactMetric contains only kind,
value and unit; resource cost rows do not represent the necessary report scope.

## Existing behavior

`internal/pricing/calculator.go` implements resource/service actual costs,
forecasts, budgets and recommendations. `settings.go` advertises plugin settings
that FinFocus resolves to environment variables. The existing FinFocus carbon
plugin provides offline projected operational EC2 estimates through projected
cost transport. Actual-cost impact metrics already propagate to core, but this
does not supply an aggregate historical reporting interface.

## Approaches considered

1. Attach AWS totals to existing actual-cost rows. Rejected: daily resource rows
   do not match monthly account/service emissions and would risk duplicating
   or allocating totals without evidence.
2. Extend the offline carbon estimator to fetch AWS reports. Rejected: introduces
   credentials and networking into an offline projection plugin, and still
   requires an aggregate host contract.
3. Add optional historical reporting to aws-ce with an explicit SDK/host
   contract. Selected: reuses AWS operational conventions while keeping
   projections and historical inventory separate.

## Opt-in configuration

Advertise `carbon_enabled` as BOOL, default `false`, mapped to
`FINFOCUS_AWS_CARBON_ENABLED`. FinFocus resolves project/global plugin config
using its existing precedence; the plugin does not create a second config-file
parser. Accept case-insensitive true/false with surrounding whitespace; unset
or blank means false. Reject other nonempty values at startup, naming the
setting. A process restart applies configuration changes.

Examples of the two inputs:

```yaml
plugins:
  aws-ce:
    carbon_enabled: true
```

```sh
export FINFOCUS_AWS_CARBON_ENABLED=true
```

When disabled, discovery omits the new historical-carbon capability, the report
RPC returns FailedPrecondition before credential extraction or client creation,
and no Sustainability calls occur. The descriptor remains advertised so users
can discover and enable the setting. Merely enabling the setting performs no
AWS access; only an explicit historical report does.

Do not advertise the existing projected `CARBON` capability from aws-ce: core
uses that capability for projected enrichment. The new capability is distinct.

## Protocol and SDK contract

Add an optional `GetCarbonEmissions` RPC and `HistoricalCarbonProvider` SDK
interface with a distinct `HISTORICAL_CARBON` capability. Register through the
standard SDK server in gRPC and Connect modes. Existing plugins do not need to
implement it; absence returns Unimplemented. The new report is additive and
does not modify GetActualCost or GetProjectedCost.

Request fields:

- Required start and end timestamps, inclusive start/exclusive end, both first
  day of a month at 00:00:00 UTC; end must follow start.
- Optional account IDs, region codes and service codes (exact AWS dimension
  values), with OR within a dimension and AND across dimensions.
- Accounting method: market-based or location-based. Unspecified selects
  market-based; responses always identify the resolved method.
- Emissions scope: total, scope 1, scope 2 or scope 3. Unspecified selects total.
- Page size, default 50 and maximum 1000; negative values are invalid.
- Opaque continuation token, bound to the same normalized query and credential
  context. It must not expose credentials or AWS error messages.

Each result retains:

- Provider `aws` and source `aws-sustainability`.
- Usage account ID, region and service as returned by AWS; missing required
  grouping dimensions are malformed data, not invented identities.
- AWS's original monthly start/end, resolved accounting method, scope,
  `ModelVersion`, normalized gCO2e value and unit, and retrieval timestamp.
- A stable identity derived from provider, account, region, service, period,
  method and scope. Model version is provenance, not a second additive identity.

The response includes records and next-page token. Empty records mean no
available data for that page/query, not zero emissions. A denied request is an
error. The host may describe data as unpublished based on the documented
publication schedule but must not assert a reason for an empty AWS result.
Zero is reported only for an explicitly returned zero. AWS values are estimates,
and rounding may make a returned zero differ from physical zero.

Validate requests and responses through SDK helpers. Reject NaN, infinity,
negative emissions, unknown units, malformed periods and missing provenance.
Do not silently coerce malformed upstream values into missing or zero.

## AWS client

Add the official `service/sustainability` dependency and a focused injectable
client wrapper. Pin the Sustainability endpoint region to `us-east-1`; reporting
region filters select usage regions and do not select the endpoint. Preserve
the existing credential-chain and per-request assume-role behavior.

Call GetEstimatedCarbonEmissions with MONTHLY and GroupBy
USAGE_ACCOUNT_ID, REGION and SERVICE. Request exactly one AWS emission type for
the selected method/scope; never request overlapping total and scope rows as
additive peers. Scope 1 is shared between accounting methods but the report
still identifies the selected method.

Convert MTCO2e to gCO2e by multiplying by 1,000,000, checking finiteness. Retain
the AWS ModelVersion and original reporting period. Do not extrapolate, prorate
partial months, allocate by spend, synthesize resource IDs or fill missing months
with zero.

Use bounded retries, cancellation and the existing attempt-budget hook. Limit
each report walk to 100 AWS pages, reject repeated continuation tokens and fail
instead of returning a silently truncated successful report. Pagination state
must be integrity-protected and query/credential-bound. Never retain caller
credentials or share a caller's report through a process-wide cache. Initial
implementation does not cache carbon data, permitting AWS model revisions to
appear on the next query.

Build the complete requested report in the host before printing output. An AWS
page failure fails the report atomically. Error messages and logs use sanitized
codes and fixed operation text; credentials and raw AWS errors remain private.
Monetary calls run independently and acquire no new permission dependency.

## FinFocus report flow

Add a dedicated `finfocus carbon actual` command with required `--start` and
`--end` UTC date flags, optional account/region/service filters, `--method`,
`--scope`, and table/JSON output. Default method is market-based and scope total;
both are displayed explicitly. Select aws-ce using the historical capability,
not the projected-carbon enrichment path. If no enabled provider is available,
return an actionable instruction to enable carbon_enabled.

Walk report pages with the query unchanged. Reject duplicate stable identities
and mixed methods/scopes before computing totals. Sum only disjoint
account/region/service/month records within the selected method and scope.
Show the original periods, source and model versions; JSON retains all report
fields. Do not merge these rows into resource costs or projected carbon totals.

## Implementation boundaries

- finfocus-spec: additive messages/RPC/capability, generated SDKs, validators,
  optional server adapters and compatibility/conformance checks.
- aws-ce: settings validation/discovery, AWS client, report handler, sanitized
  errors and offline contract fixtures; README/configuration/API/IAM docs.
- finfocus: typed pluginhost client, capability routing, report command,
  provenance-preserving table/JSON output and process-level tests.

No purchases, resource mutations, console scraping, S3 export setup, estimated
resource allocation or changes to the offline carbon plugin are included.

## Verification and acceptance

1. Disabled/default/false configurations produce no carbon capability or AWS
   access; true enables explicit report requests; invalid config fails startup.
2. Exact fixture requests prove endpoint, MONTHLY grouping, filters, credentials
   and selected emission type. Every normalized value has method, scope,
   original period and model provenance.
3. SDK JSON-to-gRPC/Connect and core process tests prove each aggregate appears
   once, pagination is bounded, and rendered/JSON totals equal independent
   fixture sums without mixing estimates or accounting methods.
4. Missing data, explicit zero, access denial, throttling, cancellation,
   malformed rows, repeated tokens and partial-page failure remain distinct.
5. Existing cost/projection tests pass with the feature disabled and enabled;
   ordinary monetary requests never invoke Sustainability.
6. Run each changed repository's required offline tests, generation checks and
   lint. Run no real AWS verification without an explicit request.

## References

- https://github.com/rshade/finfocus-plugin-aws-ce/issues/29
- https://aws.amazon.com/about-aws/whats-new/2026/03/aws-launches-sustainability-console/
- https://docs.aws.amazon.com/sustainability/latest/APIReference/API_GetEstimatedCarbonEmissions.html
- https://docs.aws.amazon.com/sustainability/latest/userguide/getting-started.html
- https://docs.aws.amazon.com/service-authorization/latest/reference/list_sustainability.html
- https://github.com/rshade/finfocus/blob/main/plugins/carbon/README.md
