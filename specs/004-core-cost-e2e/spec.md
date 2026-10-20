# Feature specification: actual-cost foundation and end-to-end testing

**Issue:** #11, with official conformance tracked in #31

**Created:** 2026-01-21

**Scope revised:** 2026-10-19

**Status:** implementation and offline verification, with live verification outstanding

## Approved scope

Complete the current actual-cost foundation so later budgets, forecasting,
anomaly detection, and recommendations can build on verified plugin behavior.
The current contract in [README.md](../../README.md) and
[CONTEXT.md](../../CONTEXT.md) governs this work.

This revision replaces the original requirements for arbitrary dimension grouping,
monthly queries, tag filtering, persistent cache reuse, and manual cache
invalidation. These features are outside this change. Forecasting, pricing,
reservation coverage operations, and resource management also remain outside scope.

## Current actual-cost contract

- Explicit account queries return service totals visible to the credentials.
  Explicit service queries filter by the exact Cost Explorer service name.
- EC2 instance queries use `GetCostAndUsageWithResources`, require account
  opt-in, and cover the last 14 days. Service totals cover 14 months.
- Queries use daily granularity and normalized Coordinated Universal Time dates
  with an exclusive end date. Request tags remain ignored and documented.
- Commitment discovery produces disjoint Reserved Instance, Savings Plan, and
  residual partitions. Commitment partitions use amortized cost when supplied;
  residual charges use Unblended cost. AWS remains the source of monetary values.
- Caller-supplied `billing_account_id` enables FOCUS output. EC2 resource IDs
  must remain resource identities, separate from the service name. Invalid
  FOCUS input must return an error rather than an incomplete successful record.
- Missing costs return an explicit error. Estimated data appears in FOCUS
  extensions when a billing account is supplied.
- Default caching remains in memory within one calculator session. Recent or
  estimated results expire after 15 minutes; closed historical results after
  24 hours. Per-request credentials bypass shared caching.

## User stories and acceptance criteria

### Retrieve correctly translated actual costs

FinFocus users need actual AWS costs with correct identities and metadata.

1. Valid account and service queries preserve exact decimal totals, currency,
   period boundaries, commitment attribution, and estimated status.
2. An EC2 response uses the known compute service name, preserves the instance
   ID in the FOCUS resource field and group key, and remains correct on cache hits.
3. Missing currency or group identity cannot produce a successful incomplete
   FOCUS record.
4. Invalid requests, unavailable data, permission failures, and exhausted
   request budgets return meaningful protocol errors.

### Exercise the real FinFocus executable offline

Maintainers need a test of the core executable discovering and invoking
this plugin. It must use controlled costs without AWS credentials.

1. `make core-install` builds an explicitly pinned core revision.
   `FINFOCUS_CORE_VERSION` can override the pin, and `FINFOCUS_CORE_BINARY` can
   select an existing executable. Tests never download an executable themselves.
2. `make e2e` and `make test-e2e` run the real core against an isolated plugin
   registry and local fake Cost Explorer endpoint with synthetic credentials.
3. Assertions check non-empty results, identity, adapter, currency, dates,
   expected totals, and source breakdown. Command success alone is insufficient.
4. Missing plugin data and AWS errors cannot pass as successful cost retrieval.
5. Pull-request continuous integration runs the offline core test automatically.

### Run official conformance

Maintainers need the official SDK suite to verify the declared capabilities.

1. `make conformance` runs the official Standard suite through the plugin process.
2. The suite checks advertised capabilities and the correct unsupported status
   for operations outside the actual-only contract.
3. The adapter passes real responses through. It must not invent pricing,
   discard mandatory failures, or rewrite suite results.
4. Any upstream suite correction is consumed through a reproducible revision;
   committed local filesystem replacements are prohibited.

### Verify bounded live AWS access when authorized

Maintainers need an explicitly enabled comparison against live AWS data.

1. `make e2e-live` requires `FINFOCUS_AWS_LIVE=true`, the `awslive` build tag,
   a configured core executable, and credentials from the existing Pulumi
   Environments, Secrets, and Configuration environment.
2. A scoped service query defaults to Amazon Simple Storage Service and covers
   at most seven days. Two discovery calls must establish that the selected
   scope has no Reserved Instance or Savings Plan IDs before comparing an
   independent Unblended-cost baseline with direct plugin and core results.
3. The baseline is limited to one page. Direct plugin and core calls each have
   at most three Cost Explorer attempts. The total limit is nine Cost Explorer
   attempts. Each direct query and core process has a 45-second deadline and
   creates no cloud resources.
4. A manual workflow runs only from `main` with `run_live` selected and the
   enable variable set. It obtains credentials only after maintainers configure
   its Pulumi OIDC settings. Fork pull requests receive no credentials.
5. Missing credentials, authorization, billing data, or supported comparison
   scope must be reported explicitly, never counted as successful live evidence.

## Completion evidence

Offline tests can establish protocol behavior, core integration, conformance,
and failure handling. Live verification is a separate acceptance result.
The owner requested offline continuation for the 2026-10-19 implementation
session. That continuation made no live AWS calls.
The older service comparison predates the current commitment-discovery path
and does not establish completion of the new live check. Record future results
and their scope in [AWS verification](../../docs/AWS-VERIFICATION.md).
