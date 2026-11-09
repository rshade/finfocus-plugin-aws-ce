# Proposal

## Why

Source isolation (#100) returns the sources that succeeded when one selected
source fails. Recommendation page tokens are SDK offsets into the full,
sorted result (`pluginsdk.PaginateRecommendations`), and
`GetRecommendationsResponse` has no field that names a failed source. A source
that fails for one page and answers for the next changes the length of that
list, so the offset silently skips or repeats rows. Review of PR #102 raised
this.

## What Changes

- When any selected source fails, the RPC returns the remaining rows only if
  the request has no page token and the result fits on one page (no next-page
  token). Otherwise it fails with the status of the first failure, as it did
  before source isolation.
- A call with no failed source paginates exactly as before.
- `docs/recommendations.md`: the Errors table separates per-source failures
  (skipped unless every source fails) from whole-call failures (throttling,
  budget, cancellation, deadline, mixed currencies, savings overflow,
  projection), and the Pagination section explains the rule.

## Impact

- Code: `internal/pricing/recommendations.go` (`recommendationSourceRun`).
- Tests: `internal/pricing/recommendations_isolation_test.go`.
- No response fields or token semantics are added.
