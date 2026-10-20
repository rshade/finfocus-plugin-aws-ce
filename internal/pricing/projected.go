package pricing

import (
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

// forecastWindow is the Cost Explorer forecast range. Start is the next UTC
// day because the forecast API rejects a start in the past. The length is 30
// days so CostPerMonth can hold the sum of the daily means AWS returns.
func forecastWindow(now time.Time) (time.Time, time.Time) {
	now = now.UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return start, start.AddDate(0, 0, 30)
}

func forecastFilter(resource *pbc.ResourceDescriptor) *types.Expression {
	if resource.GetResourceType() == "aws:service" && resource.GetId() != "" {
		return &types.Expression{Dimensions: &types.DimensionValues{
			Key:    types.DimensionService,
			Values: []string{resource.GetId()},
		}}
	}
	if resource.GetResourceType() == pulumiEC2InstanceType {
		service, _ := costExplorerService("ec2")
		return &types.Expression{Dimensions: &types.DimensionValues{
			Key:    types.DimensionService,
			Values: []string{service},
		}}
	}
	return nil
}

func forecastResponse(rows []client.ForecastResult) *pbc.GetProjectedCostResponse {
	resp := &pbc.GetProjectedCostResponse{Currency: rows[0].Currency}
	var total float64
	for _, row := range rows {
		total += row.MeanValue
		if row.Currency != "" {
			resp.Currency = row.Currency
		}
	}
	resp.CostPerMonth = total
	if len(rows) == 1 && rows[0].PredictionIntervalLowerBound != nil && rows[0].PredictionIntervalUpperBound != nil {
		lower := *rows[0].PredictionIntervalLowerBound
		upper := *rows[0].PredictionIntervalUpperBound
		resp.PredictionIntervalLower = &lower
		resp.PredictionIntervalUpper = &upper
		if rows[0].PredictionIntervalLevel != nil {
			level := *rows[0].PredictionIntervalLevel
			resp.ConfidenceLevel = &level
		}
	}
	return resp
}
