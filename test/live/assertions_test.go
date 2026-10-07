package live

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCompareLiveCost(t *testing.T) {
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	periods := []types.ResultByTime{{TimePeriod: &types.DateInterval{Start: aws.String("2026-09-27"), End: aws.String("2026-09-28")}, Groups: []types.Group{{Keys: []string{"Amazon Simple Storage Service"}, Metrics: map[string]types.MetricValue{"AmortizedCost": {Amount: aws.String("12.5"), Unit: aws.String("USD")}}}}}}
	response := &pbc.GetActualCostResponse{Results: []*pbc.ActualCostResult{{Timestamp: timestamppb.New(start), Cost: 12.5, Source: "aws-ce", FocusRecord: &pbc.FocusCostRecord{BillingAccountId: "caller", ServiceName: "Amazon Simple Storage Service", BillingCurrency: "USD", ChargePeriodStart: timestamppb.New(start), ChargePeriodEnd: timestamppb.New(start.Add(24 * time.Hour)), ExtendedColumns: map[string]string{"estimated": "false", "amount_decimal": "12.5"}}}}}
	if err := compareLiveCost(periods, response, "caller"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*pbc.GetActualCostResponse)
	}{
		{"wrong date end", func(r *pbc.GetActualCostResponse) { r.Results[0].FocusRecord.ChargePeriodEnd = timestamppb.New(start) }},
		{"wrong exact amount", func(r *pbc.GetActualCostResponse) { r.Results[0].FocusRecord.ExtendedColumns["amount_decimal"] = "0" }},
		{"wrong amount", func(r *pbc.GetActualCostResponse) { r.Results[0].Cost = 0 }},
		{"wrong account", func(r *pbc.GetActualCostResponse) { r.Results[0].FocusRecord.BillingAccountId = "other" }},
		{"wrong currency", func(r *pbc.GetActualCostResponse) { r.Results[0].FocusRecord.BillingCurrency = "EUR" }},
		{"wrong estimate", func(r *pbc.GetActualCostResponse) { r.Results[0].FocusRecord.ExtendedColumns["estimated"] = "true" }},
		{"missing row", func(r *pbc.GetActualCostResponse) { r.Results = nil }},
		{"duplicate row", func(r *pbc.GetActualCostResponse) { r.Results = append(r.Results, r.Results[0]) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := proto.Clone(response).(*pbc.GetActualCostResponse)
			tc.mutate(r)
			if compareLiveCost(periods, r, "caller") == nil {
				t.Fatal("accepted incorrect live result")
			}
		})
	}
	periods[0].Groups[0].Metrics["AmortizedCost"] = types.MetricValue{Amount: aws.String("bad"), Unit: aws.String("USD")}
	if compareLiveCost(periods, response, "caller") == nil {
		t.Fatal("accepted malformed AWS amount")
	}
}

func TestCompareLiveCostAcrossDays(t *testing.T) {
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	periods := []types.ResultByTime{}
	for i, amount := range []string{"0.1", "0.2"} {
		day := start.Add(time.Duration(i) * 24 * time.Hour)
		periods = append(periods, types.ResultByTime{Estimated: i == 1, TimePeriod: &types.DateInterval{Start: aws.String(day.Format(time.DateOnly)), End: aws.String(day.Add(24 * time.Hour).Format(time.DateOnly))}, Groups: []types.Group{{Keys: []string{"Amazon Simple Storage Service"}, Metrics: map[string]types.MetricValue{"AmortizedCost": {Amount: aws.String(amount), Unit: aws.String("USD")}}}}})
	}
	response := &pbc.GetActualCostResponse{Results: []*pbc.ActualCostResult{{Cost: 0.3, Source: "aws-ce", Timestamp: timestamppb.New(start), FocusRecord: &pbc.FocusCostRecord{BillingAccountId: "caller", ServiceName: "Amazon Simple Storage Service", BillingCurrency: "USD", ChargePeriodStart: timestamppb.New(start), ChargePeriodEnd: timestamppb.New(start.Add(48 * time.Hour)), ExtendedColumns: map[string]string{"estimated": "true", "amount_decimal": "0.3"}}}}}
	if err := compareLiveCost(periods, response, "caller"); err != nil {
		t.Fatal(err)
	}
}

func TestCompareLiveCostSumsCommitmentPartitions(t *testing.T) {
	start := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	periods := []types.ResultByTime{{TimePeriod: &types.DateInterval{Start: aws.String("2026-09-27"), End: aws.String("2026-09-28")}, Groups: []types.Group{{Keys: []string{"Amazon Elastic Compute Cloud - Compute"}, Metrics: map[string]types.MetricValue{"AmortizedCost": {Amount: aws.String("7.5"), Unit: aws.String("USD")}, "UnblendedCost": {Amount: aws.String("100"), Unit: aws.String("USD")}}}}}}
	response := &pbc.GetActualCostResponse{}
	for i, amount := range []string{"2.5", "4", "1"} {
		columns := map[string]string{"estimated": "false", "amount_decimal": amount, "metric": "AmortizedCost"}
		switch i {
		case 0:
			columns["reservation_id"] = "ri-1"
		case 1:
			columns["savings_plan_arn"] = "sp-1"
		case 2:
			columns["metric"] = "UnblendedCost"
		}
		response.Results = append(response.Results, &pbc.ActualCostResult{Timestamp: timestamppb.New(start), Cost: []float64{2.5, 4, 1}[i], Source: "aws-ce", FocusRecord: &pbc.FocusCostRecord{BillingAccountId: "caller", ServiceName: "Amazon Elastic Compute Cloud - Compute", BillingCurrency: "USD", ChargePeriodStart: timestamppb.New(start), ChargePeriodEnd: timestamppb.New(start.Add(24 * time.Hour)), ExtendedColumns: columns}})
	}
	if err := compareLiveCost(periods, response, "caller"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*pbc.GetActualCostResponse)
	}{
		{"float sum", func(r *pbc.GetActualCostResponse) { r.Results[1].Cost = 3 }},
		{"decimal sum", func(r *pbc.GetActualCostResponse) { r.Results[1].FocusRecord.ExtendedColumns["amount_decimal"] = "3" }},
		{"mixed currency", func(r *pbc.GetActualCostResponse) { r.Results[1].FocusRecord.BillingCurrency = "EUR" }},
		{"missing partition", func(r *pbc.GetActualCostResponse) { r.Results = r.Results[:2] }},
		{"duplicate partition", func(r *pbc.GetActualCostResponse) { r.Results = append(r.Results, r.Results[0]) }},
		{"wrong period", func(r *pbc.GetActualCostResponse) {
			r.Results[1].FocusRecord.ChargePeriodEnd = timestamppb.New(start.Add(48 * time.Hour))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := proto.Clone(response).(*pbc.GetActualCostResponse)
			tc.mutate(r)
			if compareLiveCost(periods, r, "caller") == nil {
				t.Fatal("accepted incorrect partition total")
			}
		})
	}
}
