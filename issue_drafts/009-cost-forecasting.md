# Issue: Feature - Cost Forecasting (#9)

**Status:** Planned
**Type:** Feature
**Priority:** High

## User Story
As a FinOps practitioner, I want to see projected costs for the coming months directly in PulumiCost, so that I can anticipate budget overruns without leaving my workflow.

## Technical Thesis
Implement the `GetProjectedCost` RPC by acting as a direct proxy to the AWS Cost Explorer `GetCostForecast` API.

### API Mapping
*   **RPC:** `GetProjectedCost(GetProjectedCostRequest)`
*   **AWS API:** `costexplorer.GetCostForecast`
*   **Input Mapping:**
    *   `Request.StartTime` / `Request.EndTime` -> `TimePeriod` (Start/End)
    *   `Request.Granularity` -> `Granularity` (DAILY/MONTHLY only)
    *   `Request.Filter` -> `Filter` (Service, Region, Tag)

## Boundary Guardrails (Hard Constraints)
1.  **No Internal Math:** We strictly forbid implementing custom forecasting algorithms (e.g., linear regression, moving averages).
2.  **API Limits:**
    *   If the user requests `HOURLY` granularity, return `codes.InvalidArgument` (AWS does not support it).
    *   If the user requests a date range > 3 months (Daily) or > 18 months (Monthly), return `codes.InvalidArgument`.
3.  **Error Handling:** If AWS returns `DataUnavailableException` (not enough historical data), return a clean gRPC error explaining why, rather than a zero value.

## Acceptance Criteria
- [ ] `GetProjectedCost` returns valid forecasted values for DAILY granularity (up to 3 months).
- [ ] `GetProjectedCost` returns valid forecasted values for MONTHLY granularity (up to 12 months).
- [ ] Requests for HOURLY granularity return a clear "Not Supported" error.
- [ ] Response includes the "Prediction Interval" (80% confidence) if available from AWS.
