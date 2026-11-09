# spot-recommendations Specification

## Purpose

Emit opt-in move-to-Spot recommendations from `GetRecommendations` for EC2
target resources. Each recommendation carries a dynamic pricing category, an
optional interruption risk score, a computed monthly impact and a stable id,
and a lookup failure never fails the call.

## Requirements

### Requirement: Emit move-to-Spot recommendations behind the opt-in

When `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, `GetRecommendations` SHALL emit one
recommendation per EC2 target resource that has an instance type and either an
availability zone or a region, computed from the Spot quote used by
`GetProjectedCost`. A target with a zone SHALL use that zone. A target with only a
region SHALL use the region-level quote. With the opt-in off, or without such a
target, the response SHALL be identical to the response before this capability
existed.

#### Scenario: Opt-in on with an eligible target

- **WHEN** the opt-in is on and a target is an EC2 instance with instance type and zone
- **THEN** the response includes a recommendation for that resource with action type MIGRATE and category COST

#### Scenario: Opt-in on with a region-only target

- **WHEN** the opt-in is on and a target is an EC2 instance with instance type and region but no zone
- **THEN** the response includes a region-level recommendation for that resource

#### Scenario: Opt-in off

- **WHEN** the opt-in is unset or not `true`
- **THEN** no Spot recommendation is emitted and no Spot AWS call is made

#### Scenario: Ineligible target

- **WHEN** a target is not EC2, lacks an instance type, or has neither a zone nor a region
- **THEN** no Spot recommendation is emitted for it, the request does not fail, and a debug log names the reason

### Requirement: Tag Spot recommendations with category and interruption risk

A Spot recommendation SHALL set `pricing_category` to
`FOCUS_PRICING_CATEGORY_DYNAMIC`. It SHALL set `spot_interruption_risk_score` in
[0, 1] only when the Spot Instance Advisor supplies a bucket the plugin can map,
and SHALL otherwise leave it at 0.0, which means unknown. The plugin SHALL NOT
emit either field on any recommendation that is not a Spot move.

#### Scenario: Known risk

- **WHEN** the advisor reports bucket `<5%` for the instance type
- **THEN** the score is 0.05 and the category is DYNAMIC

#### Scenario: Unknown risk

- **WHEN** the advisor feed fails, lacks the instance type, or reports the `>20%` bucket
- **THEN** the score is 0.0 and the category is still DYNAMIC
- **AND** the advisor label, when present, appears in metadata

#### Scenario: Non-Spot recommendation

- **WHEN** a rightsizing, Savings Plans, reservation or anomaly recommendation is returned
- **THEN** its category is UNSPECIFIED and its score is 0.0

### Requirement: Fill the impact for the Spot target

A Spot recommendation SHALL fill `RecommendationImpact` with `current_cost` (the
on-demand monthly cost), `projected_cost` (the Spot monthly cost),
`estimated_savings` and `savings_percentage`, in USD, using the same hour count
as `GetProjectedCost`. When the price list has no on-demand row, the plugin SHALL
NOT emit a recommendation, because savings cannot be computed.

#### Scenario: Savings computed

- **WHEN** Spot is 0.03 USD per hour and on-demand is 0.10 USD per hour over 730 hours
- **THEN** current cost is 73.0, projected cost is 21.9, savings 51.1 and percentage 70.0

#### Scenario: No on-demand price

- **WHEN** the Price List API returns no row for the instance type
- **THEN** no recommendation is emitted for that target and a warning is logged

#### Scenario: Spot costs more than on-demand

- **WHEN** the Spot price is at or above the on-demand price
- **THEN** no recommendation is emitted

### Requirement: Keep Spot recommendation ids stable and valid

The recommendation id SHALL be a deterministic hash of account-independent
inputs: source, resource id, region, location, instance type, operating system and
action type. Prices, risk, and zone count SHALL NOT feed the id. Every emitted
Spot recommendation SHALL pass `pluginsdk.ValidateRecommendation`; a recommendation
that fails SHALL be dropped with an error log instead of failing the response.

#### Scenario: Price moves between calls

- **WHEN** the Spot price changes between two calls for the same instance
- **THEN** the id is unchanged

#### Scenario: Region-level quote with a changing zone set

- **WHEN** the zone count or the median changes between two calls for a region-level target
- **THEN** the id is unchanged

#### Scenario: Location key

- **WHEN** the id is computed for a region-level quote
- **THEN** the location is the literal `region` in place of a zone, and for an explicit-zone quote the location is the zone

#### Scenario: Explicit zone keeps its id

- **WHEN** a target names an availability zone
- **THEN** the id equals the hash produced before this change, using the zone as the location

#### Scenario: Validation

- **WHEN** a Spot recommendation is built with category DYNAMIC and any risk in [0, 1]
- **THEN** `ValidateRecommendation` returns no error

#### Scenario: Zero risk with another category

- **WHEN** any recommendation is produced with risk 0.0
- **THEN** its category is never set to a non-DYNAMIC value together with a nonzero risk, and 0.0 is never sent with STANDARD to imply safety

### Requirement: Fail soft on Spot data

A Spot lookup failure for one target SHALL NOT fail the whole
`GetRecommendations` call or suppress Cost Explorer recommendations; the target
is skipped with a logged warning. Spot results SHALL merge with other sources
before shared filtering, sorting and pagination.

#### Scenario: Spot API throttled

- **WHEN** `DescribeSpotPriceHistory` fails for one target
- **THEN** other recommendations are returned and that target has none

### Requirement: Quote a region-level target at the median zone price

When a target has an instance type and a region but no availability zone, the
plugin SHALL query `DescribeSpotPriceHistory` for that instance type and product
without a zone filter, over the last hour. It SHALL keep the newest price for each
zone that reported one, and quote the median of those prices. It SHALL NOT quote
the cheapest zone, and it SHALL NOT count a zone with no row in the window.

#### Scenario: Odd zone count

- **WHEN** the latest prices in three zones are 0.03, 0.05 and 0.04 USD per hour
- **THEN** the quote is 0.04 and the zone count is 3

#### Scenario: Even zone count

- **WHEN** the latest prices in four zones are 0.02, 0.03, 0.05 and 0.06 USD per hour
- **THEN** the quote is 0.04 and the zone count is 4

#### Scenario: Equal timestamps

- **WHEN** two rows in the same zone share a timestamp and carry different prices
- **THEN** the higher price is kept, whatever the response order

#### Scenario: No price in any zone

- **WHEN** no zone reports a price in the window
- **THEN** the lookup returns NotFound and the target is skipped

#### Scenario: Page cap reached

- **WHEN** the history still has a next token after five pages
- **THEN** the lookup fails with the existing page-limit error and no quote is returned

### Requirement: Label region-level quotes in every output

A region-level quote SHALL set metadata `spot_az_basis` to `region`,
`spot_availability_zone` to `region-median`, and `spot_zone_count` to the number
of zones in the median. Its description and reasoning SHALL say the price is a
region-level median. An explicit-zone quote SHALL keep its existing metadata and
SHALL NOT set `spot_az_basis`. The same labels SHALL apply to the
`GetProjectedCost` response and to the recommendation.

#### Scenario: Region-level projection

- **WHEN** `GetProjectedCost` quotes a region-level target
- **THEN** the response metadata includes `spot_az_basis` set to `region` and `spot_zone_count`

#### Scenario: Explicit-zone projection

- **WHEN** `GetProjectedCost` quotes a target with an explicit zone
- **THEN** the response metadata has no `spot_az_basis` key and its other keys are unchanged
