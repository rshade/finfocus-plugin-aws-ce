package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

type baselineFake struct {
	discovery int
	costs     int
	ids       bool
	page      bool
	fail      bool
}

func (f *baselineFake) GetDimensionValues(_ context.Context, in *costexplorer.GetDimensionValuesInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetDimensionValuesOutput, error) {
	f.discovery++
	if in.Filter == nil || in.Filter.Dimensions == nil || aws.ToString(in.TimePeriod.Start) != "2026-09-20" || in.Filter.Dimensions.Values[0] != "Amazon Simple Storage Service" {
		return nil, errors.New("incorrect discovery scope")
	}
	out := &costexplorer.GetDimensionValuesOutput{}
	if f.fail {
		return nil, errors.New("discovery failed")
	}
	if f.ids {
		out.DimensionValues = []types.DimensionValuesWithAttributes{{Value: aws.String("commitment")}}
	}
	if f.page {
		out.NextPageToken = aws.String("more")
	}
	return out, nil
}

func (f *baselineFake) GetCostAndUsage(_ context.Context, in *costexplorer.GetCostAndUsageInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	f.costs++
	if in.Metrics[0] != "UnblendedCost" || in.Granularity != types.GranularityDaily || in.Filter.Dimensions.Values[0] != "Amazon Simple Storage Service" {
		return nil, errors.New("incorrect cost scope")
	}
	return &costexplorer.GetCostAndUsageOutput{ResultsByTime: []types.ResultByTime{{TimePeriod: in.TimePeriod, Groups: []types.Group{{Keys: []string{"Amazon Simple Storage Service"}, Metrics: map[string]types.MetricValue{"UnblendedCost": {Amount: aws.String("1.25"), Unit: aws.String("USD")}}}}}}}, nil
}

func TestLiveBaselineRequiresNoCommitments(t *testing.T) {
	start := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		fake    baselineFake
		wantErr bool
	}{
		{name: "residual only"},
		{name: "commitment makes baseline incomparable", fake: baselineFake{ids: true}, wantErr: true},
		{name: "incomplete discovery", fake: baselineFake{page: true}, wantErr: true},
		{name: "failed discovery", fake: baselineFake{fail: true}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := fetchLiveBaseline(context.Background(), &tc.fake, "Amazon Simple Storage Service", start, start.AddDate(0, 0, 1))
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr && tc.fake.costs != 0 {
				t.Fatal("queried incomparable costs")
			}
			if !tc.wantErr && (len(rows) != 1 || tc.fake.discovery != 2 || tc.fake.costs != 1) {
				t.Fatalf("unexpected rows or calls: %d %d %d", len(rows), tc.fake.discovery, tc.fake.costs)
			}
		})
	}
}

func TestBaselineTotal(t *testing.T) {
	const service = "Amazon Simple Storage Service"
	group := func(name, amount, currency string) types.Group {
		return types.Group{Keys: []string{name}, Metrics: map[string]types.MetricValue{
			"UnblendedCost": {Amount: aws.String(amount), Unit: aws.String(currency)},
		}}
	}
	for _, tc := range []struct {
		name    string
		groups  []types.Group
		wantErr bool
	}{
		{name: "exact sum", groups: []types.Group{group(service, "0.1", "USD"), group(service, "0.2", "USD")}},
		{name: "empty", wantErr: true},
		{name: "wrong service", groups: []types.Group{group("other", "1", "USD")}, wantErr: true},
		{name: "invalid decimal", groups: []types.Group{group(service, "NaN", "USD")}, wantErr: true},
		{name: "missing currency", groups: []types.Group{group(service, "1", "")}, wantErr: true},
		{name: "mixed currencies", groups: []types.Group{group(service, "1", "USD"), group(service, "2", "EUR")}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			total, currency, err := baselineTotal([]types.ResultByTime{{Groups: tc.groups}}, service)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && (total != 0.3 || currency != "USD") {
				t.Fatalf("unexpected total=%v currency=%s", total, currency)
			}
		})
	}
}
