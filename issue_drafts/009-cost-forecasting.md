# Cost forecasting, issue 9

**Status:** planned
**Type:** feature
**Priority:** high

## User story

A FinOps practitioner needs projected costs for the coming months in FinFocus to anticipate budget overruns.

## Technical approach

Implement the `GetProjectedCost` remote procedure call as a direct proxy for the AWS Cost Explorer `GetCostForecast` API.

### API mapping

- **Remote procedure call:** `GetProjectedCost(GetProjectedCostRequest)`.
- **AWS API:** `costexplorer.GetCostForecast`.
- **Input mapping:**
  - `Request.StartTime` and `Request.EndTime` to `TimePeriod` for the start and end dates.
  - `Request.Granularity` to `Granularity`. Support only `DAILY` and `MONTHLY`.
  - `Request.Filter` to `Filter`, for service, region, and tag.

## Constraints

1. Don't implement custom forecasting algorithms such as linear regression or moving averages.
2. Enforce API limits:
   - Return `codes.InvalidArgument` for `HOURLY` granularity, which AWS doesn't support.
   - Return `codes.InvalidArgument` for ranges longer than 3 months with daily granularity or 18 months with monthly granularity.
3. If AWS returns `DataUnavailableException` because historical data is insufficient, return a gRPC error that explains the cause instead of a zero value.

## Acceptance criteria

- [ ] `GetProjectedCost` returns valid forecasts for `DAILY` granularity for up to 3 months.
- [ ] `GetProjectedCost` returns valid forecasts for `MONTHLY` granularity for up to 12 months.
- [ ] Requests for `HOURLY` granularity return a clear unsupported-granularity error.
- [ ] Include the prediction interval at 80% confidence when AWS provides it.
