# Feature specification: AWS optimization recommendations

**Issue:** #13

**Created:** 2026-10-19

**Status:** approved and implemented; `make test`, `make lint`, and `make build` passed on 2026-10-19. Offline HTTP, RPC, and conformance coverage passed. Opt-in live verification is available but was not run.

## Purpose and scope

A cloud architect or FinOps practitioner can view AWS-generated optimization
recommendations through FinFocus without leaving the plugin workflow. Issue #13
includes both EC2 rightsizing and Savings Plans purchase recommendations. This
scope supersedes the narrower split in `issue_drafts/013-optimization-rightsizing.md`
and `issue_drafts/108-savings-plans-recommendations.md`.

Implement `pluginsdk.RecommendationsProvider` using the pinned FinFocus SDK and
the existing Cost Explorer client. Advertise recommendations in `GetPluginInfo`,
`Supports`, and the supported RPC metadata once the handler is implemented.

The plugin reads recommendations; it does not purchase commitments, change
instances, or start Savings Plans recommendation generation. RDS rightsizing,
Reserved Instances, SageMaker and Database Savings Plans, custom recommendation
scoring, and persistent dismissal storage are outside this feature.

## User story: view optimization recommendations

As an AWS cloud architect, I want to retrieve rightsizing and Savings Plans
recommendations with their AWS savings estimates so I can assess opportunities
before making infrastructure or commitment decisions.

Acceptance criteria:

1. An unfiltered request retrieves EC2 rightsizing and both Compute and EC2
   Instance Savings Plans recommendations visible to the request credentials.
2. EC2 terminate and modify recommendations produce distinct action types and
   the corresponding SDK action detail. Modify targets remain alternatives:
   select the first AWS-ranked target rather than counting every alternative
   as independent savings for the same instance.
3. Savings Plans records describe the proposed commitment and AWS estimates,
   including plan type, hourly commitment, upfront amount, term, and payment
   option when supplied by AWS.
4. Results support SDK filtering, target resource scope, exclusions, sorting,
   pagination, and a summary for the returned page.
5. No matching recommendations is a successful empty response. Failed AWS
   retrieval or invalid financial data cannot become a successful empty result.

## AWS requests and defaults

Use `GetRightsizingRecommendation` with service `AmazonEC2`, benefits considered,
and same-instance-family targets. This API supports EC2 only.

Use `GetSavingsPlansPurchaseRecommendation` once for `COMPUTE_SP` and once for
`EC2_INSTANCE_SP`, with `THIRTY_DAYS`, `ONE_YEAR`, and `NO_UPFRONT`. Use payer
scope for recommendations visible to the caller and apply an explicit account
filter when requested. Record these choices in recommendation metadata.
The RPC has no dedicated fields for overriding these purchase parameters;
this version does not overload resource tags to introduce such configuration.

Only call sources that can satisfy explicit category/action filters. Push
account filters to AWS where supported, and region filters for rightsizing
where supported. Apply remaining selection after mapping: Savings Plans filter
expressions have different restrictions from rightsizing expressions.

Follow the `NextPageToken` from each API, with the existing 100-page safety cap per
query. Detect repeated tokens. Every AWS attempt must pass through the existing
request budget and retry policy, including context cancellation. Exhaustion
or failure returns an error; do not return an incomplete successful result.

## SDK response mapping

Use `Recommendation` and its action-detail types, rather than cost FOCUS records.
The pinned SDK uses category/action filters, not the recommendation-type enumeration values
mentioned in the old issue drafts.

| AWS data | FinFocus representation |
| --- | --- |
| Rightsizing `CurrentInstance.ResourceId` | Resource ID; provider `aws`; EC2 instance resource type |
| Account, region, instance type, and tags | Corresponding resource fields |
| `TERMINATE` and terminate detail | Terminate action/detail; cost optimization category |
| `MODIFY` and first target instance | Rightsize action/detail with current and target instance types |
| Savings Plans recommendation detail | Purchase commitment action/detail; cost optimization category |
| AWS recommendation identifier | Stable source-prefixed recommendation ID |
| Missing AWS identifier | Deterministic digest of account, source, resource, action, and target/plan identity |
| Estimated monthly savings and currency | Recommendation impact with monthly projection by default |
| Generation timestamp, when available | `created_at` |
| Source | `aws-cost-explorer` |

Savings Plans are account-level purchase opportunities. Their resource identity
must identify the account/plan opportunity, not invent an existing instance or
purchased Savings Plan ARN. Retain regional/instance-family detail for EC2
Instance plans when AWS provides it. A Compute plan without a region does not
match an explicit regional or instance target filter.

Parse AWS monetary decimal strings with explicit error handling. Reject missing
required savings/currency, malformed values, negative savings, NaN, infinity,
and mixed currencies when a combined summary cannot represent them. Preserve
AWS financial estimates; do not calculate utilization, confidence, ROI, or
break-even values. Leave unsupported optional metrics absent.

Monthly projection is the default. Support `daily` by dividing monthly estimates
by 30 and `annual` by multiplying by 12; document these as display conversions,
not additional AWS forecasts. Reject other projection periods. Apply savings
thresholds in the selected projection period. Preserve the original monthly
estimate and projection convention in metadata.

Set a documented neutral priority because AWS does not provide a matching
FinFocus urgency rating. Leave confidence absent. Use AWS generation time when
available; never substitute the retrieval time as recommendation creation time.

## Validation, selection, and pagination

Reject nil requests. Use `pluginsdk.ValidateRecommendationFilter` before AWS
access, supplementing it with finite numeric checks and validation of enum,
projection, and target resource fields. Allow at most 100 target resources;
validate each resource with SDK helpers where available.

Use SDK helpers for recommendation validation, filtering, visibility,
sorting, pagination, and page summary calculation. Target matching follows the
pinned Protocol Buffers contract: provider and resource type are required matches;
specified SKU, region, and tags match exactly. Honor supplied resource IDs/ARNs
as additional identity constraints. Do not broaden unsupported target scopes.

Provider/source/resource filters that cannot match produce an empty response
without unnecessary AWS calls. Missing optional AWS metadata cannot satisfy
filters requiring that metadata.

Retrieve and map applicable AWS pages, then apply projection, selection,
exclusions, deterministic sorting, and SDK pagination. Use ID as a stable
tie-breaker. The default page size is 50 and the maximum is 1000, following SDK
behavior. Return `InvalidArgument` for invalid page tokens. Offset pagination
queries AWS again and does not promise snapshot consistency if AWS regenerates
recommendations between calls. Page summaries cover returned records only;
aggregate savings are potential opportunities and may overlap across rightsizing
and commitment recommendations.

## Architecture and errors

Add focused AWS retrieval methods in `internal/client` and mapping/handler code
in `internal/pricing`. Reuse existing credential selection, including per-request
credentials and assume-role behavior. Do not retain caller credentials or cache
recommendations across requests. Use narrow recommendation API interfaces to
avoid requiring unrelated cost-client mocks to implement new operations.

Use `pluginsdk.LogOperation` and the existing trace logger. Reuse AWS error
mapping for permission failures, throttling, cancellation, and deadlines. Map
bad client input to `InvalidArgument`, invalid upstream payloads to an appropriate
data/internal error, and unavailable recommendation datasets to an explicit
availability/precondition error. Do not include credentials or raw AWS payloads
in client-visible error messages or logs. Any selected-source failure fails
the RPC because the response contract has no partial-failure indicator.

## Verification and documentation

Add unit tests for terminate/modify mapping, target alternatives, both Savings
Plans types, defaults, identity stability, financial parsing, projections,
empty data, all supported filters, exclusions, target matching, sorting, and
page-only summaries. Exercise AWS pagination, repeated tokens, page caps,
request budgets, retries, cancellation, permission failures, and source failures.
Verify per-request credentials and capability discovery over the SDK test server.

Run `make test`, `make lint`, and `make build`. Use an offline HTTP AWS fixture
to verify serialized requests and real AWS SDK response decoding. Provide an
opt-in live verification path for accounts with existing recommendation data;
report live checks as pending unless actually exercised.

Update README permissions, defaults, limitations, examples, and live verification
instructions. Update `AGENTS.md`, `GEMINI.md`, and `CLAUDE.md` consistently.
Reconcile the local issue drafts with the approved combined scope.

## References

- [Issue #13](https://github.com/rshade/finfocus-plugin-aws-ce/issues/13)
- [AWS rightsizing API](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetRightsizingRecommendation.html)
- [AWS Savings Plans API](https://docs.aws.amazon.com/aws-cost-management/latest/APIReference/API_GetSavingsPlansPurchaseRecommendation.html)
- Pinned SDK: `github.com/rshade/finfocus-spec v0.7.9-0.20261020013049-bc3a94391af7`.
