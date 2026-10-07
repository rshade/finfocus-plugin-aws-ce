# AWS Budgets support, issue 8

**Status:** planned
**Type:** feature
**Priority:** high

## User story

An engineering manager needs to view AWS Budgets alongside actual costs to compare spending with defined limits.

## Technical approach

Implement the `getbudgets` remote procedure call from `finfocus-spec` v0.5.0 as a proxy for the AWS Budgets API.

### API mapping

- **Remote procedure call:** `GetBudgets(GetBudgetsRequest)`.
- **AWS API:** `budgets.DescribeBudgets` for lists and `budgets.GetBudget` for details.
- **Data mapping:**
  - `AWS BudgetLimit` to `FocusBudget.Amount`.
  - `AWS CalculatedSpend` to `FocusBudget.Actual`.
  - `AWS ForecastedSpend` to `FocusBudget.Forecast`.

## Constraints

1. Keep access read-only. Never create, update, or delete budgets.
2. Leave alerts to the core engine or AWS. The plugin doesn't send email when costs exceed a budget.
3. Use AWS `CalculatedSpend` when available. Don't subtract `Actual` from `Limit` to derive `Remaining`, because rounding and credits can affect the result.

## Acceptance criteria

- [ ] `GetBudgets` lists all budgets for the configured account.
- [ ] Budget details include the limit, actual spend, and forecast spend.
- [ ] Support cost and usage budget types, with appropriate filters when needed.
- [ ] Unit tests mock the `DescribeBudgets` response.
