package live

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

type baselineAPI interface {
	GetDimensionValues(context.Context, *costexplorer.GetDimensionValuesInput, ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error)
	GetCostAndUsage(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
}

func baselineTotal(periods []types.ResultByTime, service string) (float64, string, error) {
	total := new(big.Rat)
	currency := ""
	for _, period := range periods {
		for _, group := range period.Groups {
			if len(group.Keys) != 1 || group.Keys[0] != service {
				return 0, "", fmt.Errorf("unexpected baseline service")
			}
			metric, ok := group.Metrics["UnblendedCost"]
			if !ok || metric.Amount == nil || aws.ToString(metric.Unit) == "" {
				return 0, "", fmt.Errorf("missing baseline financial data")
			}
			value, ok := new(big.Rat).SetString(*metric.Amount)
			if !ok {
				return 0, "", fmt.Errorf("invalid baseline decimal")
			}
			if currency != "" && currency != *metric.Unit {
				return 0, "", fmt.Errorf("mixed baseline currency")
			}
			currency = *metric.Unit
			total.Add(total, value)
		}
	}
	amount, _ := total.Float64()
	if currency == "" || math.IsInf(amount, 0) || math.IsNaN(amount) {
		return 0, "", fmt.Errorf("empty or invalid baseline costs")
	}
	return amount, currency, nil
}

// fetchLiveBaseline verifies a residual-only service before comparing UnblendedCost.
// A service with commitments needs a different oracle; global AmortizedCost is
// not equivalent to the plugin's mixed commitment/residual attribution.
func fetchLiveBaseline(ctx context.Context, ce baselineAPI, service string, start, end time.Time) ([]types.ResultByTime, error) {
	if service == "" || !end.After(start) || end.Sub(start) > 7*24*time.Hour {
		return nil, fmt.Errorf("baseline requires a service and a positive window of at most seven days")
	}
	period := &types.DateInterval{Start: aws.String(start.Format(time.DateOnly)), End: aws.String(end.Format(time.DateOnly))}
	filter := &types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionService, Values: []string{service}}}
	for _, dimension := range []types.Dimension{types.DimensionReservationId, types.DimensionSavingsPlanArn} {
		out, err := ce.GetDimensionValues(ctx, &costexplorer.GetDimensionValuesInput{
			TimePeriod: period, Dimension: dimension, Context: types.ContextCostAndUsage, Filter: filter,
		})
		if err != nil {
			return nil, fmt.Errorf("baseline commitment discovery failed: %w", err)
		}
		if out == nil || aws.ToString(out.NextPageToken) != "" {
			return nil, fmt.Errorf("baseline discovery exceeds the single-page budget")
		}
		for _, value := range out.DimensionValues {
			if aws.ToString(value.Value) != "" {
				return nil, fmt.Errorf("baseline service has commitments; choose a service with residual-only costs")
			}
		}
	}
	out, err := ce.GetCostAndUsage(ctx, &costexplorer.GetCostAndUsageInput{
		TimePeriod: period, Filter: filter, Granularity: types.GranularityDaily,
		Metrics: []string{"UnblendedCost"},
		GroupBy: []types.GroupDefinition{{Type: types.GroupDefinitionTypeDimension, Key: aws.String("SERVICE")}},
	})
	if err != nil {
		return nil, fmt.Errorf("baseline cost query failed: %w", err)
	}
	if out == nil || aws.ToString(out.NextPageToken) != "" {
		return nil, fmt.Errorf("baseline costs exceed the single-page budget")
	}
	return out.ResultsByTime, nil
}
