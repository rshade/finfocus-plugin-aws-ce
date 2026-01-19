# Issue: Feature - Savings Plans Recommendations (#108)

**Status:** Researching
**Type:** Feature
**Priority:** Medium

## User Story
As a FinOps lead, I want to see AWS-generated Savings Plans purchase recommendations to cover my steady-state usage.

## Technical Thesis
Implement the `GetRecommendations` RPC (scoped to `SavingsPlans`) by proxying `costexplorer.GetSavingsPlansPurchaseRecommendation`.

### API Mapping
*   **RPC:** `GetRecommendations` (Filter: `RECOMMENDATION_TYPE_SAVINGS_PLAN`)
*   **AWS API:** `costexplorer.GetSavingsPlansPurchaseRecommendation`
*   **Inputs:** Requires `LookbackPeriodInDays` (7, 30, 60) and `SavingsPlansType` (Compute, EC2, SageMaker). We should expose these via the request or default to standard values (e.g., 30 days, Compute).

## Boundary Guardrails
1.  **No Financial Advice:** We rely entirely on AWS's calculated "ROI" and "Break-even months".
2.  **Parameters:** If the User doesn't specify a lookback period, default to AWS defaults (usually 7 or 30 days).
3.  **Complexity:** Handle the complex nested structure of `SavingsPlansPurchaseRecommendation` (which includes hourly usage details) by flattening it to the summary level for v1.

## Acceptance Criteria
- [ ] Returns Compute Savings Plans recommendations.
- [ ] Returns EC2 Instance Savings Plans recommendations.
- [ ] Maps "Estimated Monthly Savings" and "Upfront Cost" correctly.
