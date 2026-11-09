# Proposal

## Why

Most Pulumi programs place `aws:ec2/instance:Instance` through `subnetId` and
never set `availabilityZone`. The subnet carries the zone, so the plugin skips the
instance: `wantsSpotComparison` needs an AZ. A preview-only create therefore gets
no Spot quote and no move-to-Spot recommendation, silently. Issue #101 asks for a
region-level fallback in the plugin, because pricing knowledge belongs here and
finfocus core must not invent a zone.

## What Changes

- An EC2 target with an instance type and a region, but no availability zone, is
  eligible for the Spot comparison (`GetProjectedCost`) and for the move-to-Spot
  recommendation. A target with an explicit zone is unchanged.
- A region-level quote is the median of the latest Spot price in each zone that
  reported one in the last hour. The lookup is one `DescribeSpotPriceHistory`
  call filtered by instance type and product, with no zone filter. Zones are not
  enumerated, so no new IAM action is needed.
- Region-level quotes are labeled: `spot_az_basis` is `region`,
  `spot_availability_zone` is `region-median`, and `spot_zone_count` is the number
  of zones in the median. The description and reasoning say so too.
- A region-level recommendation id substitutes the literal `region` for the zone.
  Explicit-zone ids are byte-identical to v0.1.2.
- A target skipped for any eligibility reason logs at debug level with the reason.

## Capabilities

### Modified Capabilities

- `spot-recommendations`: eligibility, id rule, and the region-level quote.

## Impact

- Code: `internal/client/spot.go`, `internal/client/ondemand.go` (cache helper
  signature only), `internal/client/client.go`, `internal/pricing/spot.go`,
  `internal/pricing/spot_recommendations.go`, `internal/pricing/recommendations.go`
  (no logic change expected beyond the shared eligibility helper).
- Docs: `README.md`, `docs/recommendations.md`, `docs/api.md`,
  `docs/configuration.md`, `CLAUDE.md`.
- AWS calls: the same `ec2:DescribeSpotPriceHistory` action, without the zone
  filter. No new IAM actions.
- Out of scope: picking the cheapest zone, core changes (option 2 in #101),
  rightsizing and Savings Plans code, and CHANGELOG (Release Please).
