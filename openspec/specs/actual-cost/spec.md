# Actual cost

## Purpose

Describe tested actual-cost identifier handling, request construction, and time-range checks.

## Requirements

### Requirement: Resource identity for actual costs

The plugin SHALL derive an EC2 instance ID from a valid ARN or a legacy bare
instance ID when constructing a `GetActualCost` resource query. When both
identifiers differ, the ARN-derived ID SHALL take precedence. Covered by `TestGetActualCost_WithArn`,
`TestGetActualCost_BackwardCompatibility`, and
`TestGetActualCost_MismatchIdentifiers` in `internal/pricing/calculator_test.go`.

#### Scenario: ARN and legacy callers

- **WHEN** a caller requests recent actual cost with a valid EC2 ARN or bare instance ID
- **THEN** the locally constructed `GetCostAndUsageWithResources` input contains the corresponding `RESOURCE_ID` filter

### Requirement: Resource query retention limit

The plugin SHALL reject a bare EC2 instance cost query older than 14 days before
calling Cost Explorer. Covered by `TestGetActualCost_BareInstanceID` in
`internal/pricing/calculator_test.go`.

#### Scenario: Old instance cost query

- **WHEN** the caller requests an instance period older than 14 days
- **THEN** the plugin returns `InvalidArgument` without making an AWS call
