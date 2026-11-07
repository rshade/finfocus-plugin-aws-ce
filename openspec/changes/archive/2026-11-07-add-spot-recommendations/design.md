## Context

`GetRecommendations` merges mapped Cost Explorer results, then runs shared
strict matching, SDK filter, visibility, sort and pagination. `spot.go` already
builds a Spot quote for `GetProjectedCost` (`LatestSpotPrice`, `OnDemandHourly`,
advisor `Interruption`), with injectable interfaces and five-minute caches.
finfocus-spec v0.7.11 adds `pricing_category` (field 20) and
`spot_interruption_risk_score` (field 21) to `Recommendation`, and
`ValidateRecommendation` rejects a nonzero risk with a non-DYNAMIC category.

## Goals / Non-Goals

**Goals:** typed Spot recommendations for explicit EC2 targets, behind the
existing opt-in; stable ids; honest unknown risk; heavy documentation.

**Non-Goals:** discovering instances (the plugin has no inventory), ranking by
risk, cheapest-zone search, non-EC2 Spot, any purchase or mutation.

## Decisions

1. **Targets only.** Spot recommendations come from `req.TargetResources`. Without
   an inventory the plugin cannot enumerate EC2 instances, and Cost Explorer
   rightsizing already covers the untargeted case. Alternative (derive instances
   from rightsizing output) rejected: couples two sources and hides Spot behind
   AWS data availability.
2. **Refactor `quoteSpot` into a shared quote step.** Extract the price, on-demand
   and advisor lookup into `spotQuoteFor(ctx, logger, query, hours)` returning a
   struct; `quoteSpot` and the new builder both call it. The projected-cost
   response is unchanged, locked by existing `spot_test.go`.
3. **Action type MIGRATE, category COST, source `aws-cost-explorer`-distinct.**
   Use source `aws-spot-pricing` so consumers can tell computed recommendations
   from Cost Explorer ones. Resource info uses the target id, provider `aws`,
   resource type, region and SKU (instance type).
4. **Impact math.** `current_cost = on_demand_hourly * hours`, `projected_cost =
   spot_hourly * hours`, savings the difference, percentage `savings / current * 100`
   (the spec's percentage scale is checked against `ValidateRecommendationImpact`
   during apply). Hours come from `pluginsdk.DefaultMonthlyHours` and the request
   `projection_period` is applied by the existing `projectRecommendationSavings`
   so Spot rows scale like the others.
5. **Skip rather than fabricate.** No on-demand row, Spot >= on-demand, or a failed
   lookup means no recommendation. Savings cannot be honest otherwise.
6. **Risk.** Reuse advisor mapping: `0.05/0.10/0.15/0.20`; the `>20%` bucket and
   any failure leave 0.0 and keep the label in metadata. The builder is the only
   code that sets category and score, so non-Spot rows cannot carry them.
7. **Stable id.** `recommendationID(source, resource id, region, zone, instance
   type, os, action)`, the same hash helper the other sources use. Prices and risk
   are excluded so dismissals survive price moves.
8. **Fail soft.** Per-target errors log a warning and skip the target. This
   deliberately differs from the all-source failure of Cost Explorer sources, and
   is documented. Concurrency: targets run sequentially. The EC2, Price List
   and advisor calls don't go through the Cost Explorer pacer, so they don't count
   against the request budget; caches make repeats cheap.
9. **Validation gate.** Each built recommendation goes through
   `pluginsdk.ValidateRecommendation`; failure is dropped with an error log.

## Risks / Trade-offs

- Spot prices change minute to minute; the five-minute cache bounds staleness and
  `ExpiresAt` is not available on `Recommendation`, so docs state the freshness.
- Amending "never generates recommendations" weakens a documented guarantee. It
  is narrowed to one opt-in, labeled by a distinct source name.
- Spot comparison needs extra IAM (EC2, Pricing). Already documented for
  `GetProjectedCost`; recommendations reuse it, and docs say which RPC needs what.

## Documentation Plan

Docs are a first-class deliverable, not an afterthought:

- `docs/recommendations.md`: new "Spot recommendations" section: when emitted,
  eligible targets, a request/response worked example (JSON), field table,
  impact formula with numbers, risk mapping table, id stability, fail-soft
  behavior, freshness, limitations, and a rewritten opening that states the
  exception to "never generates".
- `docs/api.md`: new `Recommendation` fields and the Spot-only guarantees.
- `docs/configuration.md` and `internal/pricing/settings.go`: note that the
  opt-in now also governs recommendations.
- `docs/permissions.md`, README summary table, `docs/deployment.md` policy: show
  the Spot actions are required by `GetRecommendations` too.
- `CLAUDE.md`: amend the Spot and "never generate" paragraphs.
- Explanation of why 0.0 means unknown and why core never reorders by risk.

## Open Questions

- Confirm the `savings_percentage` scale (0-100 vs fraction) against SDK
  validation before writing tests.
