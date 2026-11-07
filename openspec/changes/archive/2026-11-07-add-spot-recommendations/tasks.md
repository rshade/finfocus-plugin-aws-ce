## 1. SDK bump

- [x] 1.1 Bump `finfocus-spec` to v0.7.11 (`go get`, `make deps`) and update `specVersion` if the SDK registry requires it.
  Verify: `go build ./... && go test ./internal/pricing/ -run 'Info|Spec'` passes.
  Break check: `GetPluginInfo` spec version and the conformance suite still agree.

## 2. Shared Spot quote

- [x] 2.1 Extract the lookup in `quoteSpot` into a reusable quote step in `internal/pricing/spot.go`.
  Verify: `go test ./internal/pricing/ -run Spot` passes unchanged.
  Break check: `GetProjectedCost` Spot response, metadata and warnings are byte-identical.

## 3. Spot recommendation builder (TDD)

- [x] 3.1 Write failing tests in `spot_recommendations_test.go`: DYNAMIC with known risk, unknown risk (score 0.0, still DYNAMIC), `>20%` bucket, no on-demand row, Spot >= on-demand, non-Spot rows carry neither field, id stable across price change, `ValidateRecommendation` passes.
  Verify: tests fail for the missing builder, then pass after 3.2.
  Break check: no existing recommendation test needs editing.
- [x] 3.2 Add `internal/pricing/spot_recommendations.go`: build recommendation, impact, metadata, stable id, validation gate.
  Verify: `go test ./internal/pricing/ -run SpotRecommendation -v` passes.
  Break check: category and score are set only inside this builder.

## 4. Wire into GetRecommendations

- [x] 4.1 Call the builder for eligible targets when the opt-in is on, fail soft per target, merge before shared filtering.
  Verify: wire test with opt-in on returns the Spot row; with it off the response equals the stored baseline.
  Break check: target-scoped requests still skip credential init when no source is selected; Cost Explorer failure semantics unchanged; pagination tokens stable.
- [x] 4.2 Update `settings.go` description and its test.
  Verify: `go test ./internal/pricing/ -run Settings` passes.
  Break check: `docs/configuration.md` table still matches settings.

## 5. Documentation

- [x] 5.1 Write the Spot recommendations section in `docs/recommendations.md` with worked JSON example, field table, impact formula, risk table, id stability, fail-soft, freshness, limitations, and the rewritten "never generates" statement.
- [x] 5.2 Update `docs/api.md`, `docs/configuration.md`, `docs/permissions.md`, `docs/deployment.md` policy, and README summary table.
- [x] 5.3 Amend `CLAUDE.md` (Spot paragraph and the never-generate rule).
  Verify: `markdownlint` and `mise run lint-prose` report no new findings on changed files.
  Break check: documented IAM actions equal the calls the code makes.

## 6. Verification

- [x] 6.1 Add an offline conformance/recommendation fixture covering a Spot target.
  Verify: `make conformance` passes.
  Break check: the awslive test stays untouched and unrun.
- [x] 6.2 Run `make fmt lint test test-integration build` and `openspec validate --all --strict`.
  Verify: all exit 0 (known flaky `TestMakeInstallLocal`/`TestPluginSubprocessGRPC` pass alone).
  Break check: `git diff --stat` touches nothing in `specs/` or `.specify/`.
