# Savings Plans recommendations, issue 108

**Status:** researching
**Type:** feature
**Priority:** medium

## User story

A FinOps lead needs AWS Savings Plans purchase recommendations to cover steady-state usage.

## Technical approach

Implement the `GetRecommendations` remote procedure call for `SavingsPlans` as a proxy for `costexplorer.GetSavingsPlansPurchaseRecommendation`.

### API mapping

- **Remote procedure call:** `GetRecommendations`, with the `RECOMMENDATION_TYPE_SAVINGS_PLAN` filter.
- **AWS API:** `costexplorer.GetSavingsPlansPurchaseRecommendation`.
- **Inputs:** `LookbackPeriodInDays`, which accepts 7, 30, or 60 days, and `SavingsPlansType`, which accepts Compute, EC2, or SageMaker. Expose these through the request or use standard defaults of 30 days and Compute.

## Constraints

1. Use only AWS calculations for return on investment and months to break even.
2. Use AWS defaults when the user omits a lookback period. Defaults usually span 7 or 30 days.
3. Flatten the nested `SavingsPlansPurchaseRecommendation` structure, including hourly usage details, to a summary for version 1.

## Acceptance criteria

- [ ] Return Compute Savings Plans recommendations.
- [ ] Return EC2 Instance Savings Plans recommendations.
- [ ] Map estimated monthly savings and upfront cost correctly.
