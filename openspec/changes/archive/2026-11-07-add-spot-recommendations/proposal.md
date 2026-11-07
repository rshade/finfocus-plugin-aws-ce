# Proposal

## Why

Core FinFocus (rshade/finfocus#1839, epic #367 Spot Market Advisor) renders
whatever typed spot fields a plugin sends and holds no spot knowledge itself.
aws-ce already quotes Spot for `GetProjectedCost` behind
`FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, but `GetRecommendations` cannot tell a
caller "move this on-demand instance to Spot". finfocus-spec v0.7.11
(rshade/finfocus-spec#663) added `pricing_category` and
`spot_interruption_risk_score` to `Recommendation`, so the plugin can now say so.

## What Changes

- Bump `github.com/rshade/finfocus-spec` from v0.7.10 to v0.7.11.
- When the Spot opt-in is on, `GetRecommendations` emits one move-to-Spot
  recommendation for each EC2 target resource that has an instance type and an
  availability zone. It reuses the quote path in `spot.go` (Spot price, on-demand
  price, Spot Instance Advisor risk).
- The recommendation sets `pricing_category = FOCUS_PRICING_CATEGORY_DYNAMIC`, sets
  `spot_interruption_risk_score` only when the advisor gives a real figure, and
  fills `RecommendationImpact` with current cost, projected cost, savings and
  savings percentage.
- `Recommendation.id` is a deterministic hash, so the same suggestion keeps its
  dismissal id across calls.
- Every emitted Spot recommendation passes `pluginsdk.ValidateRecommendation`.
- Non-Spot recommendations never carry either field. Default output (opt-in off)
  is byte-for-byte unchanged.
- **Contract amendment:** the rule "the plugin doesn't generate recommendations"
  gains one exception. This opt-in Spot path computes a recommendation from live
  prices instead of copying AWS Cost Explorer output. The exception is documented
  in `docs/recommendations.md`, `CLAUDE.md` and the specs.
- Heavy documentation: a new Spot recommendations guide section, a field-by-field
  reference, worked examples, limitations, IAM and API-reference updates.

## Capabilities

### New Capabilities

- `spot-recommendations`: opt-in move-to-Spot recommendations for EC2 targets,
  with pricing category, interruption risk, impact, stable ids and validation.

### Modified Capabilities

- `reservation-recommendations`: the "complete-source" and "proxy only" wording
  must allow the one computed Spot source without changing reservation behavior.

## Impact

- Code: `go.mod`/`go.sum`, `internal/pricing/recommendations.go`, a new
  `internal/pricing/spot_recommendations.go`, reuse of `spot.go` helpers.
- Docs: `docs/recommendations.md`, `docs/api.md`, `docs/configuration.md`,
  `docs/permissions.md`, `README.md`, `CLAUDE.md`.
- AWS calls: the same EC2 `DescribeSpotPriceHistory`, Price List and advisor feed
  already used by the Spot quote; no new IAM actions.
- Out of scope: ranking by risk (core never reorders), picking the cheapest zone,
  non-EC2 Spot, and purchasing or mutating anything.
