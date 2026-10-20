package pricing

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

// projectionWindow splits the current UTC month at today. Cost Explorer
// reports the days before today as actuals and forecasts today through the last
// day of the month, so the two parts meet without a gap or an overlap. monthEnd
// is the first day of the next month, the exclusive end Cost Explorer expects.
func projectionWindow(now time.Time) (monthStart, today, monthEnd time.Time) {
	now = now.UTC()
	monthStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return monthStart, today, monthStart.AddDate(0, 1, 0)
}

func forecastScopeSupported(resource *pbc.ResourceDescriptor) bool {
	switch resource.GetResourceType() {
	case "aws:account":
		return true
	case "aws:service":
		return resource.GetId() != ""
	default:
		return false
	}
}

func forecastFilter(resource *pbc.ResourceDescriptor) *types.Expression {
	if resource.GetResourceType() == "aws:service" && resource.GetId() != "" {
		return &types.Expression{Dimensions: &types.DimensionValues{
			Key:    types.DimensionService,
			Values: []string{resource.GetId()},
		}}
	}
	return nil
}

// forecastResponse adds the month-to-date actuals to the forecast for the rest
// of the month. Actuals are known, so they shift the prediction interval
// without widening it.
func forecastResponse(rows []client.ForecastResult, monthToDate float64, monthToDateCurrency string) *pbc.GetProjectedCostResponse {
	resp := &pbc.GetProjectedCostResponse{Currency: rows[0].Currency}
	if resp.Currency == "" {
		resp.Currency = monthToDateCurrency
	}
	total := monthToDate
	for _, row := range rows {
		total += row.MeanValue
		if row.Currency != "" {
			resp.Currency = row.Currency
		}
	}
	resp.CostPerMonth = total
	if len(rows) == 1 && rows[0].PredictionIntervalLowerBound != nil && rows[0].PredictionIntervalUpperBound != nil {
		lower := monthToDate + *rows[0].PredictionIntervalLowerBound
		upper := monthToDate + *rows[0].PredictionIntervalUpperBound
		resp.PredictionIntervalLower = &lower
		resp.PredictionIntervalUpper = &upper
		if rows[0].PredictionIntervalLevel != nil {
			level := *rows[0].PredictionIntervalLevel
			resp.ConfidenceLevel = &level
		}
	}
	return resp
}
