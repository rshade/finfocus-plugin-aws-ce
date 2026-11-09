# Proposal

## Why

Issue #100 (rshade/finfocus-plugin-aws-ce) reports that `GetRecommendations`
returns nothing for any account with Cost Explorer rightsizing data. Cost
Explorer returns `RightsizingType` as `"Modify"` and `"Terminate"`, but the SDK
constants are upper case, so every real item fails the exact-equality switch in
`mapRightsizingRecommendations`. That error aborts the whole call with
`DataLoss`, and the opt-in Spot source (which needs no Cost Explorer data) is
never reached. Any failure in one source has the same effect, including a
missing Cost Explorer client when only Spot could answer.

## What Changes

- Match `RightsizingType` case-insensitively, so `"Modify"` and `"Terminate"`
  map to rightsizing and terminate recommendations as Cost Explorer returns them.
- Skip a rightsizing item that cannot be mapped, with a warning log. The other
  items on the page are still returned.
- Isolate the selected sources (rightsizing, each Savings Plans plan type, each
  reservation service, anomalies, and Spot). A source that fails is logged and
  dropped, and the RPC returns the sources that succeeded.
- When every selected source fails, the RPC still fails with the status the
  first failure maps to, so callers never receive an empty success caused by an
  outage.
- Keep hard failure for request validation, cancellation, deadline, and request
  budget exhaustion (`ResourceExhausted`), and for request-scoped credential
  errors returned as status codes.
- A Spot source with eligible targets needs an EC2/Pricing client only when its
  quote seams are not overridden. A missing Cost Explorer client does not stop the
  Spot source.

## Capabilities

### New Capabilities

- `rightsizing-recommendations`: case-insensitive mapping of Cost Explorer
  rightsizing items and per-item skipping.

### Modified Capabilities

- `reservation-recommendations`: the "complete-source" requirement changes from
  all-or-nothing to per-source isolation with an all-failed error.

## Impact

- Code: `internal/pricing/recommendations.go`, `internal/pricing/recommendation_mapping.go`,
  and their tests and offline fixtures.
- Docs: `docs/recommendations.md` (partial results and skipped items).
- AWS calls: unchanged. The same APIs are called. Failing sources no longer
  suppress the rest, so a request can make calls it previously abandoned.
- Out of scope: the availability-zone fallback (#101), changing the anomaly-only
  path, changing request validation, and changing the mixed-currency check.
