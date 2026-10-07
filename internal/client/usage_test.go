package client

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

func TestUnconstrainedQueryOmitsUsage(t *testing.T) {
	api := &mockCostExplorerAPI{getCostAndUsageFunc: func(_ context.Context, input *costexplorer.GetCostAndUsageInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
		for _, metric := range input.Metrics {
			if metric == "UsageQuantity" {
				t.Error("unconstrained query requested mixed usage")
			}
		}
		return &costexplorer.GetCostAndUsageOutput{ResultsByTime: []types.ResultByTime{{TimePeriod: &types.DateInterval{Start: aws.String("2026-10-01"), End: aws.String("2026-10-02")}, Groups: []types.Group{{Keys: []string{"S3"}, Metrics: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String("2"), Unit: aws.String("USD")},
			"UsageQuantity": {Amount: aws.String("5"), Unit: aws.String("N/A")},
		}}}}}}, nil
	}}
	ce := NewClientWithAPI(api, "us-east-1")
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetCost(context.Background(), nil, []string{"SERVICE"}, start, start.Add(24*time.Hour), "DAILY")
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if rows[0].UsageExact != nil || rows[0].UsageUnit != "" {
		t.Fatalf("reported mixed usage: %+v", rows[0])
	}
}
