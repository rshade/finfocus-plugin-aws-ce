## 1. Rightsizing mapping

- [x] 1.1 Add tests for title-case `Modify` and `Terminate`, upper-case input, and an unknown type that is skipped while other items are returned. Verify: `go test ./internal/pricing/ -run "CaseInsensitive|SkipsUnmappable"` passes.
- [x] 1.2 Normalize `RightsizingType` with `strings.ToUpper` and skip unmappable items with a warning log. Verify: `go test ./internal/pricing/ -run Rightsizing` passes.

## 2. Source isolation

- [x] 2.1 Add tests for partial success (rightsizing or Savings Plans failure), Spot surviving a failed or missing Cost Explorer client, Spot-only selection with no client, and the all-failed status. Verify: `go test ./internal/pricing/ -run "SpotSurvives|SpotOnly|AllSelected|HardFailures|RequestCredential"` passes.
- [x] 2.2 Restructure `GetRecommendations` into isolated sources with hard-failure rules and the all-failed rule. Verify: `go test ./internal/pricing/` passes, including the existing budget, cancellation, and pagination tests.
- [x] 2.3 Update the tests whose assertions encode all-or-nothing behavior (`TestRecommendationsPartialSourceFailure`, `TestRecommendationsCombinedAnomalyPageGuards` repeat case). Verify: each change is explained in the PR body.

## 3. Fixtures and docs

- [x] 3.1 Change offline rightsizing fixtures to title case (`Terminate`) in `internal/client`, `internal/pricing`, and `test/conformance`.
- [x] 3.2 Add the partial-result and skipped-item notes to `docs/recommendations.md`.

## 4. Verification

- [x] 4.1 Run `make test`, `make lint`, and `go vet ./...`. Verify: all pass.
