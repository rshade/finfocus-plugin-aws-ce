# Historical AWS Carbon Reporting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Deliver opt-in AWS historical emissions reports through FinFocus,
with advertised settings and accurate monthly aggregate provenance.

**Architecture:** Add a historical-carbon RPC and distinct capability to the
shared SDK. Implement it in aws-ce through the official Sustainability client.
Core invokes it through a dedicated report flow, independently of projected
carbon enrichment and monetary resource queries.

**Tech Stack:** Go, Protocol Buffers, gRPC/Connect, AWS SDK Go v2, Cobra, zerolog,
existing FinFocus settings delivery and SDK validators.

**Spec:** [Approved design](design.md) and
[requirements](specs/historical-carbon/spec.md).

## Global Constraints

- Opt-in: `plugins.aws-ce.carbon_enabled` / `FINFOCUS_AWS_CARBON_ENABLED`, default false.
- Advertise the setting through `GetPluginInfo.settings`, including while disabled.
- Advertise historical carbon only when enabled; never advertise projected CARBON.
- Monthly account/region/service aggregates; whole UTC months; no resource allocation.
- Exactly one accounting method and emissions scope per query.
- MTCO2e to gCO2e conversion factor: 1000000.
- Preserve period, method, scope, model version, dimensions, source and retrieval time.
- No caller-report cache; no credential retention or raw AWS error disclosure.
- Maximum 100 AWS pages; bounded retries and cancellation; no partial successful reports.
- Use offline fixtures. No real AWS verification, publication, pushes or merges.
- Keep frozen `specs/` and `.specify/` history in aws-ce intact.
- Preserve unrelated existing edits in core (`CLAUDE.md` and Terraform design work).
- Do not leave local filesystem dependency replacements in releasable module files.

## Review Focus

1. A disabled plugin must expose its configuration descriptor without gaining a reporting capability or touching credentials.
2. Multi-account or revised AWS data must not duplicate identities, leak reports, or appear as measured resource emissions.
3. A changed filter, method, page size or credential context must invalidate a continuation token.
4. Missing, explicit zero, permission denial and malformed upstream data must remain distinguishable.
5. A later page failure must leave CLI stdout empty and preserve unrelated monetary operations.

## Dependency and integration strategy

Implementation touches the sibling repositories `finfocus-spec` and `finfocus`
as well as aws-ce. Create feature branches or isolated worktrees before product
changes; carry only this feature's existing OpenSpec files. Do not stage or reset
unrelated work. For local cross-repository verification use an ignored/external
Go workspace referencing the three checkouts. Record the upstream SDK dependency
requirement; publishing a coordinated SDK release is a later integration action,
not permission to leave an absolute-path replacement in go.mod.

## Task 1: Optional SDK historical-report contract

**Files, relative to finfocus-spec:**

- Create `proto/finfocus/v1/carbon.proto` with the request/record/response and enums.
- Modify `proto/finfocus/v1/costsource.proto` to add GetCarbonEmissions.
- Modify `proto/finfocus/v1/enums.proto` to add HISTORICAL_CARBON at the next free number.
- Generate Go/Connect/TypeScript outputs with the repository's pinned generators.
- Create `sdk/go/pluginsdk/carbon.go` and `carbon_test.go` for validation and defaults.
- Modify `sdk/go/pluginsdk/sdk.go`, `plugin_info.go`, `capability_compat.go`,
  and the existing Connect adapter for optional dispatch and capability support.
- Update registry/schema/TypeScript capability mappings where existing capabilities are enumerated.

**Interfaces:**

- Produces `HistoricalCarbonProvider.GetCarbonEmissions(context.Context, *pbc.GetCarbonEmissionsRequest) (*pbc.GetCarbonEmissionsResponse, error)`.
- Produces `ValidateGetCarbonEmissionsRequest` and `ValidateGetCarbonEmissionsResponse`.
- Request fields: start, end, account_ids, regions, services, method, scope, page_size, page_token.
- Record fields: id, provider, source, account_id, region, service, start, end, method, scope, model_version, value, unit, retrieved_at.
- Method values: unspecified, market-based, location-based. Scope values: unspecified, total, scope-1, scope-2, scope-3.
- Response fields: records and next_page_token. Defaults: market-based, total, page size 50; maximum 1000.

- [ ] Write descriptor/validator/optional-provider tests before production helpers: unavailable provider returns Unimplemented; valid monthly bounds pass; partial months, invalid timestamps, negative page size and unknown enum values fail.
- [ ] Run those tests and confirm failure from the missing contract/helpers.
- [ ] Add the additive proto definitions, generate using `make generate`, implement defaults, validators and gRPC/Connect dispatch following existing optional RPC patterns.
- [ ] Test record provenance, exact zero, finite nonnegative grams, valid monthly periods and missing required grouping/model fields.
- [ ] Verify default/explicit capability handling, gRPC/Connect round trips and old providers; no historical capability may imply projected carbon.
- [ ] Run `go test ./...`, buf lint, generation consistency, schema/example validation, and repository lint. Review the diff before any commit.

## Task 2: Opt-in setting and RPC discovery

**Files, relative to aws-ce:**

- Modify `internal/pricing/settings.go`, `calculator.go`, `plugin_info_test.go`.
- Modify `cmd/plugin/config.go`, `main.go`, `config_test.go`, `config_doc_test.go`.
- Create `internal/pricing/carbon_settings_test.go`.
- Update `docs/configuration.md` and README configuration examples.

**Interfaces:**

- Consumes Task 1 HISTORICAL_CARBON and GetCarbonEmissions contract.
- Produces `CarbonEnabledEnv = "FINFOCUS_AWS_CARBON_ENABLED"`.
- Produces startup validation using the existing strict boolean parsing convention.
- Calculator stores the validated boolean; the descriptor is always present.

- [ ] Add a real SDK gRPC GetPluginInfo test asserting exactly one carbon_enabled descriptor: BOOL, env FINFOCUS_AWS_CARBON_ENABLED, default false, and a description explaining historical aggregate reporting.
- [ ] Add unset/false/true/blank/whitespace/case/invalid startup tests; run and observe failures.
- [ ] Implement parsing and calculator wiring, then conditionally append HISTORICAL_CARBON and GetCarbonEmissions to discovery. Never append projected CARBON.
- [ ] Add a disabled handler test proving FailedPrecondition occurs before credentials/client creation; discovery must perform no AWS calls.
- [ ] Run `go test ./internal/pricing ./cmd/plugin`; verify config-doc checks include the new advertised variable.

## Task 3: AWS Sustainability reader and normalization

**Files, relative to aws-ce:**

- Create `internal/client/carbon.go`, `carbon_test.go`, and offline JSON fixtures under `internal/client/testdata/carbon/`.
- Modify `go.mod` and `go.sum` for the official service/sustainability dependency.
- Reuse `internal/client/client.go` loadAWSConfig, page hooks, pacing and retry conventions.

**Interfaces:**

- Consumes `client.Config` and Task 1 request fields mapped to AWS input.
- Produces `SustainabilityAPI.GetEstimatedCarbonEmissions` using official SDK types.
- Produces `NewSustainabilityClient(context.Context, Config)` and a single-page reader with attempt-budget enforcement.
- Produces validated monthly records normalized from AWS EstimatedCarbonEmissions.

- [ ] Add an HTTP fixture test asserting POST body, MONTHLY, all three grouping dimensions, exact filters and one selected emissions type; run it failing before implementation.
- [ ] Add official SDK dependency and implement the injectable reader, forcing the Sustainability endpoint to us-east-1 without changing usage-region filters.
- [ ] Verify credential chain, request keys, session token and assume-role behavior with local fake servers; every HTTP retry consumes the existing attempt budget.
- [ ] Test all method/scope combinations, 1000000 conversion, explicit zero, absent results, malformed units/periods/model/dimensions, negative/nonfinite values and conversion overflow.
- [ ] Verify cancellation, bounded transient/throttle retry, nil output, denied access and sanitized failures.
- [ ] Run `go test ./internal/client` and targeted race tests; no fixture may contact AWS.

## Task 4: Report handler and protected pagination

**Files, relative to aws-ce:**

- Create `internal/pricing/carbon.go`, `carbon_pagination.go`, and corresponding tests.
- Extend calculator state only for opt-in, an injectable reader and process-local token signing key.
- Reuse `credentials.go` credential extraction/config and safe AWS error mapping.
- Add process/SDK contract tests under `test/integration/`.

**Interfaces:**

- Consumes Tasks 1-3 contract, opt-in and AWS page reader.
- Produces Calculator.GetCarbonEmissions with SDK request/response validation.
- Produces integrity-protected opaque cursors carrying page count, AWS token and normalized query/credential binding, never credential values.

- [ ] Write tests for invalid query rejection before AWS, disabled-before-credentials behavior, and a valid fixture RPC report; observe failures.
- [ ] Implement request-scoped client creation and operation logging with the existing SDK helpers.
- [ ] Implement monthly grouping, selected-method normalization, stable record identity and AWS model provenance.
- [ ] Write token tests for tampering, oversize input, wrong query/method/page size, wrong caller, restart and expired state; then implement bounded integrity checking.
- [ ] Enforce 100-page limits across continuations, repeated-token rejection and response validation. Return explicit gRPC status for permission/auth/throttling/cancellation failures without raw AWS text.
- [ ] Test multiple callers and concurrent walks under the race detector; no report or credential cache is shared.
- [ ] Exercise real AWS JSON through SDK deserialization and gRPC/Connect; run `make test`, `make lint`, `make build`, and offline conformance.

## Task 5: FinFocus report command and web parity

**Files, relative to finfocus:**

- Create `internal/pluginhost/carbon.go` and tests; extend capability mappings.
- Create `internal/carbonreport/report.go` and tests for the shared report operation.
- Create `internal/cli/carbon_actual.go` and tests; register the carbon command in `internal/cli/root.go`.
- Add `internal/webui/handlers_carbon.go`, handler tests and registration using existing authenticated server routing.
- Add docs under `docs/src/content/docs/plugins/` and CLI reference material.
- Add a core-to-real-plugin test with an isolated registry and fake Sustainability server.

**Interfaces:**

- Consumes Task 1 typed client and Task 4 capability/RPC behavior.
- Produces a shared Query operation accepting the typed request and provider client, walking pages atomically and returning validated records plus total grams.
- Produces `finfocus carbon actual --start YYYY-MM-DD --end YYYY-MM-DD --method market-based|location-based --scope total|1|2|3 --output table|json`, with optional account/region/service filters.
- Produces an authenticated POST `/api/carbon/actual/query` hook using the same shared Query operation, without a separate calculation implementation.

- [ ] Write tests for command discovery, required month bounds, method/scope parsing, disabled-provider guidance and typed pluginhost dispatch; run failing before implementation.
- [ ] Implement pluginhost capability routing without admitting historical sources into projected-carbon enrichment.
- [ ] Implement shared report walk: no stdout until all pages pass, reject repeated tokens, duplicate stable identities, mixed methods/scopes and invalid response/query matches.
- [ ] Test two accounts, two services, multiple months and model revisions against independent expected sums; empty reports carry no invented zeros.
- [ ] Render provider/source, dimensions, original periods, method, scope, units and model provenance in JSON and table output. Ensure output errors are returned.
- [ ] Add the authenticated web hook and test unauthorized access, input validation and parity with CLI report results. Record a UI-only follow-up without sending external messages unless separately authorized.
- [ ] Run a process-level core-to-plugin fixture test proving both config and environment opt-in delivery, descriptor discovery, pagination and exact totals. Ordinary monetary calls must make no Sustainability requests with opt-in true or false.
- [ ] Run `go test ./...`, required lint, docs lint, module validation and build. Report pre-existing failures by name and keep unrelated files untouched.

## Task 6: Review, documentation and OpenSpec archive

**Files:** Existing feature files across the three repositories; aws-ce README,
`docs/api.md`, `docs/configuration.md`, and this OpenSpec change.

- [ ] Document exact config/env examples, discovery descriptors, explicit permission, monthly publication lag, estimate terminology, accounting methods, zero/missing behavior and separate projected-plugin scope.
- [ ] Verify the SDK dependency integration is reproducible without local replacements; disclose any release-order prerequisite.
- [ ] Run the required offline test/lint/build commands once per final changed repository and record results.
- [ ] Obtain a fresh whole-change review focused on credentials, tokens, duplicate attribution and reporting semantics; fix material findings with regression tests.
- [ ] Mark tasks complete and archive the OpenSpec change only after delivered behavior and verification match the approved design.
- [ ] Leave pushes, merges, publishing and live AWS verification for explicit authorization.
