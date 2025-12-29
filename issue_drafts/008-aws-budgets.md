# Issue: Feature - AWS Budgets Support (#8)

**Status:** Planned
**Type:** Feature
**Priority:** High

## User Story
As an engineering manager, I want to view the status of my AWS Budgets alongside my actual costs, so that I can see if my current spending is tracking against my defined limits.

## Technical Thesis
Implement the `getbudgets` RPC (defined in `pulumicost-spec` v0.5.0) by proxying the AWS Budgets API.

### API Mapping
*   **RPC:** `GetBudgets(GetBudgetsRequest)`
*   **AWS API:** `budgets.DescribeBudgets` (List) & `budgets.GetBudget` (Detail)
*   **Data Transformation:**
    *   `AWS BudgetLimit` -> `FocusBudget.Amount`
    *   `AWS CalculatedSpend` -> `FocusBudget.Actual`
    *   `AWS ForecastedSpend` -> `FocusBudget.Forecast`

## Boundary Guardrails (Hard Constraints)
1.  **Read-Only:** We never Create, Update, or Delete budgets.
2.  **No Alerting Logic:** We do not implement "If cost > budget, send email." That is the responsibility of the Core engine or AWS itself.
3.  **No "Remaining" Math:** If AWS provides the `CalculatedSpend`, we use it. We do not manually subtract `Actual` from `Limit` to determine `Remaining` to avoid rounding errors or misunderstanding of credit application.

## Acceptance Criteria
- [ ] `GetBudgets` lists all budgets for the configured account.
- [ ] Budget details include Limit, Actual Spend, and Forecasted Spend.
- [ ] Supports both "Cost" and "Usage" budget types (filtering appropriately if needed).
- [ ] Unit tests mock `DescribeBudgets` response.
