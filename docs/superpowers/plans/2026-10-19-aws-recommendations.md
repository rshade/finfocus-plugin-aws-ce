# AWS Optimization Recommendations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose AWS EC2 rightsizing and Compute/EC2 Instance Savings Plans recommendations through `GetRecommendations`.

**Architecture:** Extend the existing Cost Explorer wrapper through a narrow optional recommendation interface. Keep AWS retrieval, response mapping, and RPC orchestration in focused files. Reuse credential selection, retries, request budgets, SDK helpers, and protocol errors.

**Tech Stack:** Repository Go toolchain, AWS SDK v2 Cost Explorer, pinned FinFocus SDK, zerolog, gRPC, testify.

**Spec:** `specs/005-aws-ce-recommendations-plugin/spec.md`

## Global Constraints

- Preserve pinned dependencies; `go.mod` currently requires Go 1.27.2.
- Both EC2 rightsizing and Savings Plans are in issue #13.
- Savings Plans inputs: `THIRTY_DAYS`, `ONE_YEAR`, `NO_UPFRONT`; query `COMPUTE_SP` and `EC2_INSTANCE_SP`.
- Rightsizing inputs: `AmazonEC2`, benefits considered, same-instance-family targets.
- Use AWS financial estimates; daily/monthly/annual projections use monthly/30, monthly, monthly*12.
- SDK validation, filtering, visibility, sorting, pagination, and summary helpers are required where available.
- Maximum 100 AWS pages per query; detect repeated tokens; charge every retry against the request budget.
- No cross-request recommendation cache, credential retention, resource changes, purchases, or generation requests.
- Return an error if any selected source fails; summaries describe the returned page only.

## Review Focus

- Multiple AWS resize targets for one instance must not double-count alternative savings (Task 2).
- Identical instance IDs in different accounts must produce distinct recommendation IDs (Task 2).
- Compute plans without regional identity must not leak into regional/instance-target results (Task 3).
- Per-request credentials and assume-role settings must reach every selected source without affecting later requests (Task 4).
- Malformed tokens and invalid filters must fail before credential initialization or AWS access (Task 3).

## File Structure

- Create `internal/client/recommendations.go` and `recommendations_test.go`: narrow AWS API, retrieval, retries, pagination.
- Create `internal/pricing/recommendation_mapping.go` and `recommendation_mapping_test.go`: AWS-to-protobuf mapping and stable IDs.
- Create `internal/pricing/recommendations.go` and `recommendations_test.go`: validation, source selection, projections, filters, pagination, summaries.
- Modify `internal/pricing/calculator.go`: provider assertion and capability discovery.
- Extend existing capability tests; add `internal/pricing/recommendations_wire_test.go` for RPC/credential behavior.
- Create `internal/client/recommendations_contract_test.go`: offline HTTP serialization and decoding coverage.
- Create `test/live/recommendations_test.go`: explicitly gated live verification.
- Update README, the approved spec status, local issue drafts, and root agent guidance files.

### Task 1: Retrieve recommendations through the existing AWS client

**Interfaces:**

- Define `RecommendationsAPI` with AWS SDK signatures for `GetRightsizingRecommendation` and `GetSavingsPlansPurchaseRecommendation`.
- Produce `(*Client).ListRightsizingRecommendations(ctx context.Context, input costexplorer.GetRightsizingRecommendationInput) ([]costexplorer.GetRightsizingRecommendationOutput, error)`.
- Produce `(*Client).ListSavingsPlansRecommendations(ctx context.Context, input costexplorer.GetSavingsPlansPurchaseRecommendationInput) ([]costexplorer.GetSavingsPlansPurchaseRecommendationOutput, error)`.
- Use a type assertion on the existing `ceClient`; do not widen `CostExplorerAPI`.

- [ ] Write table tests `TestListRightsizingRecommendations` and `TestListSavingsPlansRecommendations` with mocks capturing inputs: two pages return both outputs; empty nil output is an error; repeated token and page 100 with continuation fail; canceled context prevents AWS access; each retry consumes a page hook; permission failures do not retry; retryable failures eventually return results.
- [ ] Run `go test ./internal/client -run 'TestList(Rightsizing|SavingsPlans)Recommendations' -count=1`; confirm failure from missing methods.
- [ ] Implement the two retrieval methods using `WithRetry`, existing retry classification, context checks, `runPageHook` inside each attempt, token tracking, and safe wrapped errors. Copy input before changing its token.
- [ ] Run the targeted tests and `go test ./internal/client -count=1`; require pass.
- [ ] Commit only Task 1 files with `feat(client): retrieve AWS optimization recommendations`.

### Task 2: Map AWS recommendations without fabricating metrics

**Interfaces:**

- Consume the AWS outputs from Task 1.
- Produce `mapRightsizingRecommendations(pages []costexplorer.GetRightsizingRecommendationOutput) ([]*pbc.Recommendation, error)`.
- Produce `mapSavingsPlansRecommendations(pages []costexplorer.GetSavingsPlansPurchaseRecommendationOutput) ([]*pbc.Recommendation, error)`.
- Produce `projectRecommendationSavings(recs []*pbc.Recommendation, period string) error`.

- [ ] Add `TestMapRightsizingRecommendations`: terminate and modify action/detail, current identity/tags, first AWS-ranked target only, generation time, absent optional metrics, stable IDs across retrievals, distinct IDs across accounts. Assert `require.Len(t, recs, 1)` for a modify record with two targets and `require.Equal(t, 12.5, recs[0].GetImpact().GetEstimatedSavings())` for first-target monthly savings `12.50`.
- [ ] Add `TestMapSavingsPlansRecommendations`: both plan types, account-level identity, currency from AWS summary, AWS-provided ID or deterministic fallback, hourly commitment/upfront/term/payment metadata, optional region, no invented confidence or ROI. Assert stable ID when AWS detail order changes.
- [ ] Add table-driven money/projection tests: monthly `300` maps to daily `10` and annual `3600`; missing, negative, nonnumeric, `NaN`, infinity, and overflow fail; absent currency fails. Validate mapped records with `pluginsdk.ValidateRecommendation`.
- [ ] Run `go test ./internal/pricing -run 'TestMap(Rightsizing|SavingsPlans)Recommendations|TestRecommendationMoney|TestRecommendationProjection' -count=1`; confirm missing implementation failure.
- [ ] Implement mapping in `recommendation_mapping.go`: cost optimization category; terminate, rightsize, and purchase-commitment actions with typed details; source `aws-cost-explorer`; neutral medium priority; deterministic digest IDs scoped by account/source/resource/action/target. Use UTC generation timestamps only when AWS supplies them. Retain original monthly amount and projection convention in metadata.
- [ ] Run the targeted tests; require pass.
- [ ] Commit with `feat(pricing): map AWS optimization recommendations`.

### Task 3: Implement validated recommendation selection and response pages

**Interfaces:**

- Consume Task 1 methods via `clientForCall`, and Task 2 mapping/projection functions.
- Produce `(*Calculator).GetRecommendations(ctx context.Context, req *pbc.GetRecommendationsRequest) (*pbc.GetRecommendationsResponse, error)`.
- Keep auxiliary validation and matching functions private to `recommendations.go`.

- [ ] Add `TestRecommendationsValidation` for nil request, malformed token, unknown enum, invalid projection, nonfinite thresholds, over 100 targets, and invalid descriptors. Set `openClient` to a function that fails the test if invoked; all invalid requests must return `InvalidArgument` first.
- [ ] Add `TestRecommendationsSelection` with fake AWS sources: unfiltered calls all three queries; action/category/provider/source filters suppress irrelevant queries; region/SKU/tags/account/ID/priority/minimum savings/age/confidence filters select correctly; missing metadata cannot satisfy a filter; excluded IDs always disappear; dismissal flag alone has no effect.
- [ ] Add `TestRecommendationsTargets`: AND with selection filters; exact optional fields; account-level Compute plan must not match an EC2 instance target or explicit region; matching IDs/ARNs constrain identity.
- [ ] Add `TestRecommendationsPagination`: deterministic order with ID tie-breaks; default 50/max 1000 through SDK helper; no duplicates for unchanged AWS data; summaries count/sum only current page; mixed currencies fail before summary; empty result succeeds; minimum savings applies after projection.
- [ ] Run `go test ./internal/pricing -run '^TestRecommendations' -count=1`; confirm failure from missing handler.
- [ ] Implement validation before client initialization, conservative source selection, supported AWS filter pushdown, mapping/projection, SDK visibility/filter/sort/pagination/summary helpers, strict target matching, and safe error translation. Apply `withCostBudget` before retrieval. Log operations through the trace logger; never log raw upstream messages or credentials.
- [ ] Add source failure tests: selected-source failure returns no partial response; data unavailable maps explicitly; cancellation/deadline, local rate budget, permission, throttling, invalid payload, and generic errors have safe statuses.
- [ ] Run `go test ./internal/pricing -count=1`; require pass.
- [ ] Commit with `feat(pricing): serve filtered AWS recommendations`.

### Task 4: Verify credentials, protocol discovery, and AWS serialization

**Interfaces:** Consume the complete handler; no new public production interface.

- [ ] Add wire tests with `pluginsdk.NewTestServer`: serialized requests retrieve real mapped results; recommendation capability appears in info and typed/legacy support metadata; per-request static credentials and assume-role configuration reach every selected source; later requests use default credentials. Assert no credential material in responses or logs on failure.
- [ ] Add offline `httptest` cases using the real AWS SDK and synthetic credentials. Assert AWS action headers, rightsizing service/configuration, both Savings Plans types, exact purchase defaults, account filters, continuation tokens, and decoded nested response fields.
- [ ] Run targeted wire/contract tests; confirm discovery assertions fail before advertisement changes.
- [ ] Add `pluginsdk.RecommendationsProvider` compile assertion, recommendations capability in `GetPluginInfo`/`Supports`, and `GetRecommendations` in `supportedRPCs`. Update existing capability count/legacy assertions and any integration expectations affected by this supported RPC.
- [ ] Run `go test ./internal/client ./internal/pricing ./test/integration/... ./test/conformance/... -count=1`; require pass, fixing fixture expectations without weakening existing assertions.
- [ ] Commit with `feat(plugin): advertise and verify recommendations over RPC`.

### Task 5: Document and verify the complete feature

**Interfaces:** Existing live-test opt-in convention and developer commands.

- [ ] Add an `awslive` test gated by `FINFOCUS_AWS_LIVE=true` that exercises the recommendation RPC against real AWS; validate any returned records and report empty datasets explicitly. It must not generate recommendations, purchase plans, or mutate resources. Ordinary test runs never contact AWS.
- [ ] Document `ce:GetRightsizingRecommendation` and `ce:GetSavingsPlansPurchaseRecommendation`, dataset prerequisites, the exact defaults, query/action examples, source selection, monthly/30 and monthly*12 conversions, account-level Compute plan limitations, overlapping savings, offset pagination limitations, and live-test invocation in README.
- [ ] Update root `AGENTS.md`, `GEMINI.md`, and `CLAUDE.md` (create missing files only with concise project guidance), and reconcile the two recommendation issue drafts to combined issue #13. Mark spec implementation status accurately after verification.
- [ ] Run `gofmt` on changed Go files and `git diff --check`.
- [ ] Run `make test`, `make lint`, and `make build`; require successful exit status from each. Record any environmental blocker precisely. Run additional e2e checks only if discovery changes affect those fixtures.
- [ ] Review the whole diff against the approved specification, focusing on the five Review Focus cases. Use the required execution-workflow review process; fix material findings and rerun affected tests.
- [ ] Commit with `docs: document and verify AWS recommendations`; report commands/results and whether live verification actually ran. Do not mark live verification complete unless executed.
