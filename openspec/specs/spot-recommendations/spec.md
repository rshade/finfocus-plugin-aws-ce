# spot-recommendations Specification

## Purpose

Emit opt-in move-to-Spot recommendations from `GetRecommendations` for EC2
target resources. Each recommendation carries a dynamic pricing category, an
optional interruption risk score, a computed monthly impact and a stable id,
and a lookup failure never fails the call.

## Requirements

### Requirement: Emit move-to-Spot recommendations behind the opt-in

When `FINFOCUS_AWS_CE_SPOT_COMPARISON=true`, `GetRecommendations` SHALL emit one
recommendation per EC2 target resource that has an instance type and an
availability zone, computed from the Spot quote used by `GetProjectedCost`. With
the opt-in off, or without such a target, the response SHALL be identical to the
response before this capability existed.

#### Scenario: Opt-in on with an eligible target

- **WHEN** the opt-in is on and a target is an EC2 instance with instance type and zone
- **THEN** the response includes a recommendation for that resource with action type MIGRATE and category COST

#### Scenario: Opt-in off

- **WHEN** the opt-in is unset or not `true`
- **THEN** no Spot recommendation is emitted and no Spot AWS call is made

#### Scenario: Ineligible target

- **WHEN** a target is not EC2, or lacks an instance type or zone
- **THEN** no Spot recommendation is emitted for it and the request does not fail

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
inputs: source, resource id, region, availability zone, instance type, operating
system and action type. Prices and risk SHALL NOT feed the id. Every emitted Spot
recommendation SHALL pass `pluginsdk.ValidateRecommendation`; a recommendation
that fails SHALL be dropped with an error log instead of failing the response.

#### Scenario: Price moves between calls

- **WHEN** the Spot price changes between two calls for the same instance
- **THEN** the id is unchanged

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
