# FOCUS 1.3 transition, issue 104

**Status:** researching
**Type:** chore and compliance
**Priority:** medium

## User story

A FinOps practitioner needs FOCUS 1.3 commitment details to analyze effective rates for Reserved Instances and Savings Plans.

## Technical approach

Audit the `finfocus-spec` FOCUS 1.3 columns against AWS Cost Explorer data availability.

### Scope

- **New columns:** `CommitmentDiscountCategory`, `CommitmentDiscountId`, `CommitmentDiscountName`, and `CommitmentDiscountType`.
- **AWS mapping:**
  - `ReservationARN` to `CommitmentDiscountId` for Reserved Instances.
  - `SavingsPlanARN` to `CommitmentDiscountId` for Savings Plans.
  - `Savings Plan` or `Reserved Instance` to `CommitmentDiscountType`.

## Constraints

1. Populate fields only when `GetCostAndUsage` returns them, such as in `GroupByKey` or `ResultsByTime`.
2. Rely on the presence of `ReservationARN` to identify a Reserved Instance discount. Don't infer it from the price.

## Research tasks

- [ ] Check whether `GetCostAndUsage` returns `ReservationARN` and `SavingsPlanARN` when grouping by `Service`. Grouping by `RESERVATION_ID` or a similar field might be necessary.
- [ ] Check how these groupings affect the number of response rows.
