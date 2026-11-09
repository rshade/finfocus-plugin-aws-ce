# Design

## Eligibility

A target is an EC2 instance when its type is `ec2`, `aws:ec2`, or contains
`ec2/instance`. It is eligible when it has an instance type (attributes, then sku,
then tags) and either a zone or a region. `spotIneligibleReason` names the first
failed condition, and `wantsSpotComparison` is true when no reason applies. An
explicit zone with no region still reaches `parseSpotQuery`, which returns the
same `InvalidArgument` as before.

Eligible targets with an unsupported operating system or an invalid type are
rejected by `parseSpotQuery`. Recommendations log that at debug level.

## Region-level price rule

The client issues `DescribeSpotPriceHistory` for the instance type and product,
over the last hour, without a zone filter. Each row names its zone. For every zone
the plugin keeps the newest row. On equal timestamps it keeps the higher price, so
the result does not depend on response order. The quote is the median of those
per-zone prices:

- an odd count takes the middle price;
- an even count takes the arithmetic mean of the two middle prices;
- the prices are sorted before selection, so map order never matters.

Why the median and not the cheapest zone:

- The instance lands in the zone its subnet sits in, which the plugin cannot see.
  The cheapest zone is the most optimistic guess and can understate every
  realistic placement.
- One unusually cheap or dear zone moves the median by at most half the gap to
  its neighbours; it cannot dominate the figure.
- `CLAUDE.md` already records "don't pick the cheapest zone" for Spot pricing.

Zones with no row in the window are not counted, and the count is reported in
`spot_zone_count`. Pagination keeps the existing five-page cap (500 rows). Because
the query spans every zone, hitting the cap returns the existing
`spot quote page limit reached` error rather than a median over partial data.

## Quote labeling

Region-level quotes carry `spot_az_basis: region`, `spot_availability_zone:
region-median`, and `spot_zone_count`. Explicit-zone responses keep their existing
metadata and add nothing, so their output is unchanged. A missing `spot_az_basis`
therefore means the zone was explicit.

## Recommendation id

The id hashes source, resource id, region, the location key, instance type,
product, and action. The location key is the zone for an explicit-zone quote and
the literal `region` for a region-level quote. Price, risk, and zone count are not
inputs, so they never change the id.

Dismissal behaviour:

- A region-level dismissal survives price, risk, and zone-count changes.
- When the instance later reports a real zone, the id changes. A dismissal on the
  region-level row does not carry over, and the zone-specific suggestion appears as
  a new row. This is intentional: the zone-specific price is a different
  suggestion.
- The reverse holds too: a zone-level dismissal does not hide the region-level row.

## Logging

Each skipped EC2 target logs one debug line with `resource_id` and `reason`. The
reasons are `no instance type`, `no region or availability zone`, and the parse
error for an invalid type, zone, or unsupported operating system. Failed lookups
keep their existing warning.
