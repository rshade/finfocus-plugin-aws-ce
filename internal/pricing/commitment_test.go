package pricing

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

const (
	reservationKey = "arn:aws:ec2:us-east-1:123456789012:reserved-instances/ri-0abc"
	savingsPlanKey = "arn:aws:savingsplans::123456789012:savingsplan/sp-0abc"
	secondRIKey    = "arn:aws:ec2:us-east-1:123456789012:reserved-instances/ri-0def"
)

func TestCommitmentLineMapsToFocusRecord(t *testing.T) {
	t.Run("reservation", func(t *testing.T) {
		rec, metrics := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", reservationKey}, commitmentMetrics("2.50", "3.00", ""))
		assertFocusAmount(t, rec, "2.50")
		if rec.GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetCommitmentDiscountType() != "Reserved Instance" {
			t.Fatalf("type = %q", rec.GetCommitmentDiscountType())
		}
		assertCommitmentShape(t, rec, "AmortizedCost")
		if !strings.Contains(rec.GetChargeDescription(), "Amortized") {
			t.Fatalf("description %q does not say amortized", rec.GetChargeDescription())
		}
		if rec.GetExtendedColumns()["group_key"] != "AmazonEC2" {
			t.Fatalf("group_key = %q", rec.GetExtendedColumns()["group_key"])
		}
		assertMetrics(t, metrics, "UnblendedCost", "UsageQuantity", "AmortizedCost")
	})

	t.Run("savings_plan", func(t *testing.T) {
		rec, metrics := oneCommitmentRecord(t, []string{"SERVICE", "SAVINGS_PLAN_ARN"}, []string{"AmazonEC2", savingsPlanKey}, commitmentMetrics("2.50", "3.00", ""))
		assertFocusAmount(t, rec, "2.50")
		if rec.GetCommitmentDiscountId() != savingsPlanKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetCommitmentDiscountType() != "Savings Plan" {
			t.Fatalf("type = %q", rec.GetCommitmentDiscountType())
		}
		assertCommitmentShape(t, rec, "AmortizedCost")
		assertMetrics(t, metrics, "UnblendedCost", "UsageQuantity", "AmortizedCost")
	})

	t.Run("reservation_before_service", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"RESERVATION_ID", "SERVICE"}, []string{reservationKey, "AmazonEC2"}, commitmentMetrics("2.50", "3.00", ""))
		assertFocusAmount(t, rec, "2.50")
		if rec.GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetServiceName() != "AmazonEC2" || rec.GetExtendedColumns()["group_key"] != "AmazonEC2" {
			t.Fatalf("service %q group_key %q", rec.GetServiceName(), rec.GetExtendedColumns()["group_key"])
		}
	})

	t.Run("service_only", func(t *testing.T) {
		metricsIn := map[string]types.MetricValue{
			"UnblendedCost": metricUSD("1.00"),
			"BlendedCost":   metricUSD("9.99"),
			"AmortizedCost": metricUSD("2.50"),
		}
		rec, metrics := oneCommitmentRecord(t, []string{"SERVICE"}, []string{"AmazonEC2"}, metricsIn)
		assertNoCommitment(t, rec, "1.00", "UnblendedCost")
		if strings.Contains(rec.GetChargeDescription(), "Amortized") {
			t.Fatalf("description %q", rec.GetChargeDescription())
		}
		assertMetrics(t, metrics, "UnblendedCost", "UsageQuantity")
	})

	t.Run("empty_reservation_key", func(t *testing.T) {
		rec, metrics := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", ""}, commitmentMetrics("2.50", "3.00", ""))
		assertNoCommitment(t, rec, "3.00", "UnblendedCost")
		assertMetrics(t, metrics, "UnblendedCost", "UsageQuantity", "AmortizedCost")
	})

	t.Run("blank_reservation_key", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", "   "}, commitmentMetrics("2.50", "3.00", ""))
		assertNoCommitment(t, rec, "3.00", "UnblendedCost")
	})

	t.Run("usage_with_reservation", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", reservationKey}, commitmentMetrics("2.50", "3.00", "1"))
		assertFocusAmount(t, rec, "2.50")
		if rec.GetCommitmentDiscountId() != "" {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetCommitmentDiscountStatus() != pbc.FocusCommitmentDiscountStatus_FOCUS_COMMITMENT_DISCOUNT_STATUS_UNSPECIFIED {
			t.Fatalf("status = %s", rec.GetCommitmentDiscountStatus())
		}
		if rec.GetCommitmentDiscountStatus() == pbc.FocusCommitmentDiscountStatus_FOCUS_COMMITMENT_DISCOUNT_STATUS_USED {
			t.Fatal("status is USED")
		}
		if rec.GetExtendedColumns()["reservation_id"] != reservationKey {
			t.Fatalf("reservation_id = %q", rec.GetExtendedColumns()["reservation_id"])
		}
		if rec.GetExtendedColumns()["metric"] != "AmortizedCost" {
			t.Fatalf("metric = %q", rec.GetExtendedColumns()["metric"])
		}
		if rec.GetCommitmentDiscountQuantity() != 0 || rec.GetCommitmentDiscountUnit() != "" {
			t.Fatalf("commitment quantity %v unit %q", rec.GetCommitmentDiscountQuantity(), rec.GetCommitmentDiscountUnit())
		}
		if err := pluginsdk.ValidateFocusRecord(rec); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("both_ids", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t,
			[]string{"SERVICE", "RESERVATION_ID", "SAVINGS_PLAN_ARN"},
			[]string{"AmazonEC2", reservationKey, savingsPlanKey},
			commitmentMetrics("2.50", "3.00", ""),
		)
		assertNoCommitment(t, rec, "3.00", "UnblendedCost")
		if rec.GetExtendedColumns()["reservation_id"] != reservationKey {
			t.Fatalf("reservation_id = %q", rec.GetExtendedColumns()["reservation_id"])
		}
		if rec.GetExtendedColumns()["savings_plan_arn"] != savingsPlanKey {
			t.Fatalf("savings_plan_arn = %q", rec.GetExtendedColumns()["savings_plan_arn"])
		}
	})

	t.Run("placeholder_reservation_key", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", "NoReservationID"}, commitmentMetrics("2.50", "3.00", ""))
		if rec.GetCommitmentDiscountId() != "NoReservationID" {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		assertFocusAmount(t, rec, "2.50")
	})

	t.Run("trimmed_reservation_key", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", "  " + reservationKey + "  "}, commitmentMetrics("2.50", "3.00", ""))
		if rec.GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
	})

	t.Run("missing_amortized", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", reservationKey}, map[string]types.MetricValue{
			"UnblendedCost": metricUSD("3.00"),
			"BlendedCost":   metricUSD("9.99"),
		})
		assertFocusAmount(t, rec, "3.00")
		if rec.GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetExtendedColumns()["metric"] != "UnblendedCost" {
			t.Fatalf("metric = %q", rec.GetExtendedColumns()["metric"])
		}
		if rec.GetCommitmentDiscountType() != "Reserved Instance" {
			t.Fatalf("type = %q", rec.GetCommitmentDiscountType())
		}
	})

	t.Run("unparseable_amortized", func(t *testing.T) {
		metrics := commitmentMetrics("2.50", "3.00", "")
		metrics["AmortizedCost"] = types.MetricValue{Amount: aws.String("not-a-number"), Unit: aws.String("USD")}
		_, _, err := commitmentRows(t, []string{"SERVICE", "RESERVATION_ID"}, []types.Group{
			{Keys: []string{"AmazonEC2", reservationKey}, Metrics: metrics},
		})
		if !errors.Is(err, client.ErrAmountUnparseable) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("two_reservation_ids", func(t *testing.T) {
		_, records, err := commitmentRows(t, []string{"SERVICE", "RESERVATION_ID"}, []types.Group{
			{Keys: []string{"AmazonEC2", reservationKey}, Metrics: commitmentMetrics("2.50", "3.00", "")},
			{Keys: []string{"AmazonEC2", secondRIKey}, Metrics: commitmentMetrics("4.00", "5.00", "")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 2 {
			t.Fatalf("records = %d", len(records))
		}
		if records[0].GetCommitmentDiscountId() != reservationKey || records[1].GetCommitmentDiscountId() != secondRIKey {
			t.Fatalf("ids %q %q", records[0].GetCommitmentDiscountId(), records[1].GetCommitmentDiscountId())
		}
		assertFocusAmount(t, records[0], "2.50")
		assertFocusAmount(t, records[1], "4.00")
		if records[0].GetExtendedColumns()["group_key"] != "AmazonEC2" || records[1].GetExtendedColumns()["group_key"] != "AmazonEC2" {
			t.Fatalf("group keys %q %q", records[0].GetExtendedColumns()["group_key"], records[1].GetExtendedColumns()["group_key"])
		}
	})

	t.Run("zero_usage_reservation", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"SERVICE", "RESERVATION_ID"}, []string{"AmazonEC2", reservationKey}, commitmentMetrics("2.50", "3.00", "0"))
		assertFocusAmount(t, rec, "2.50")
		if rec.GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetCommitmentDiscountType() != "Reserved Instance" {
			t.Fatalf("type = %q", rec.GetCommitmentDiscountType())
		}
		if _, ok := rec.GetExtendedColumns()["reservation_id"]; ok {
			t.Fatalf("reservation_id = %q", rec.GetExtendedColumns()["reservation_id"])
		}
		if err := pluginsdk.ValidateFocusRecord(rec); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reservation_only", func(t *testing.T) {
		rec, _ := oneCommitmentRecord(t, []string{"RESERVATION_ID"}, []string{reservationKey}, commitmentMetrics("2.50", "3.00", ""))
		assertFocusAmount(t, rec, "2.50")
		if rec.GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
		}
		if rec.GetCommitmentDiscountType() != "Reserved Instance" {
			t.Fatalf("type = %q", rec.GetCommitmentDiscountType())
		}
		if rec.GetServiceName() != "unknown" {
			t.Fatalf("service = %q", rec.GetServiceName())
		}
		if rec.GetServiceName() == reservationKey {
			t.Fatal("service name is the reservation ARN")
		}
		if rec.GetExtendedColumns()["group_key"] != "" {
			t.Fatalf("group_key = %q", rec.GetExtendedColumns()["group_key"])
		}
		if err := pluginsdk.ValidateFocusRecord(rec); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("resource_and_reservation", func(t *testing.T) {
		_, records, err := commitmentRows(t, []string{"RESOURCE_ID", "RESERVATION_ID"}, []types.Group{
			{Keys: []string{"i-0abc", reservationKey}, Metrics: commitmentMetrics("2.50", "3.00", "")},
			{Keys: []string{"i-0def", reservationKey}, Metrics: commitmentMetrics("4.00", "5.00", "")},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 2 {
			t.Fatalf("records = %d", len(records))
		}
		if records[0].GetCommitmentDiscountId() != reservationKey || records[1].GetCommitmentDiscountId() != reservationKey {
			t.Fatalf("ids %q %q", records[0].GetCommitmentDiscountId(), records[1].GetCommitmentDiscountId())
		}
		if records[0].GetExtendedColumns()["group_key"] != "i-0abc" || records[1].GetExtendedColumns()["group_key"] != "i-0def" {
			t.Fatalf("group keys %q %q", records[0].GetExtendedColumns()["group_key"], records[1].GetExtendedColumns()["group_key"])
		}
	})
}

func TestGetActualCost_CommitmentFieldsUnset(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	start, end := recentCostWindow()
	resp, err := calc.GetActualCost(context.Background(), costRequest("contract-official_gcu_request", "", start, end, nil))
	if err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	assertServiceTotalQuery(t, mockAPI)
	assertMetrics(t, mockAPI.costCalls[0].Metrics, "UnblendedCost", "UsageQuantity")
	if len(resp.GetResults()) != 1 {
		t.Fatalf("results = %d", len(resp.GetResults()))
	}
	rec := resp.GetResults()[0].GetFocusRecord()
	if rec.GetCommitmentDiscountId() != "" || rec.GetCommitmentDiscountType() != "" || rec.GetCommitmentDiscountUnit() != "" {
		t.Fatalf("id %q type %q unit %q", rec.GetCommitmentDiscountId(), rec.GetCommitmentDiscountType(), rec.GetCommitmentDiscountUnit())
	}
}

func oneCommitmentRecord(t *testing.T, dimensions, keys []string, metrics map[string]types.MetricValue) (*pbc.FocusCostRecord, []string) {
	t.Helper()
	requested, records, err := commitmentRows(t, dimensions, []types.Group{{Keys: keys, Metrics: metrics}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d", len(records))
	}
	return records[0], requested
}

func commitmentRows(t *testing.T, dimensions []string, groups []types.Group) ([]string, []*pbc.FocusCostRecord, error) {
	t.Helper()
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{{
					TimePeriod: &types.DateInterval{
						Start: aws.String("2026-09-01"),
						End:   aws.String("2026-09-02"),
					},
					Groups: groups,
				}},
			}, nil
		},
	}
	ce := client.NewClientWithAPI(mockAPI, "us-east-1")
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	rows, err := ce.GetCost(context.Background(), nil, dimensions, start, end, "DAILY")
	if err != nil {
		return nil, nil, err
	}
	if len(mockAPI.costCalls) != 1 {
		t.Fatalf("GetCostAndUsage calls = %d", len(mockAPI.costCalls))
	}
	costs, err := aggregateCosts(rows)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]*pbc.FocusCostRecord, 0, len(costs))
	for _, cost := range costs {
		records = append(records, focusFor(cost))
	}
	return append([]string(nil), mockAPI.costCalls[0].Metrics...), records, nil
}

func commitmentMetrics(amortized, unblended, usage string) map[string]types.MetricValue {
	out := map[string]types.MetricValue{
		"AmortizedCost": metricUSD(amortized),
		"UnblendedCost": metricUSD(unblended),
		"BlendedCost":   metricUSD("9.99"),
	}
	if usage != "" {
		out["UsageQuantity"] = types.MetricValue{Amount: aws.String(usage), Unit: aws.String("Hrs")}
	}
	return out
}

func metricUSD(amount string) types.MetricValue {
	return types.MetricValue{Amount: aws.String(amount), Unit: aws.String("USD")}
}

func assertCommitmentShape(t *testing.T, rec *pbc.FocusCostRecord, metric string) {
	t.Helper()
	if err := pluginsdk.ValidateFocusRecord(rec); err != nil {
		t.Fatal(err)
	}
	if rec.GetCommitmentDiscountName() != "" {
		t.Fatalf("name = %q", rec.GetCommitmentDiscountName())
	}
	if rec.GetCommitmentDiscountCategory() != pbc.FocusCommitmentDiscountCategory_FOCUS_COMMITMENT_DISCOUNT_CATEGORY_UNSPECIFIED {
		t.Fatalf("category = %s", rec.GetCommitmentDiscountCategory())
	}
	if rec.GetCommitmentDiscountStatus() != pbc.FocusCommitmentDiscountStatus_FOCUS_COMMITMENT_DISCOUNT_STATUS_UNSPECIFIED {
		t.Fatalf("status = %s", rec.GetCommitmentDiscountStatus())
	}
	if rec.GetCommitmentDiscountQuantity() != 0 || rec.GetCommitmentDiscountUnit() != "" {
		t.Fatalf("quantity %v unit %q", rec.GetCommitmentDiscountQuantity(), rec.GetCommitmentDiscountUnit())
	}
	if rec.GetConsumedQuantity() != 0 || rec.GetConsumedUnit() != "" {
		t.Fatalf("consumed %v %q", rec.GetConsumedQuantity(), rec.GetConsumedUnit())
	}
	if rec.GetExtendedColumns()["metric"] != metric {
		t.Fatalf("metric = %q", rec.GetExtendedColumns()["metric"])
	}
}

func assertNoCommitment(t *testing.T, rec *pbc.FocusCostRecord, amount, metric string) {
	t.Helper()
	assertFocusAmount(t, rec, amount)
	if rec.GetCommitmentDiscountId() != "" {
		t.Fatalf("id = %q", rec.GetCommitmentDiscountId())
	}
	if rec.GetCommitmentDiscountStatus() != pbc.FocusCommitmentDiscountStatus_FOCUS_COMMITMENT_DISCOUNT_STATUS_UNSPECIFIED {
		t.Fatalf("status = %s", rec.GetCommitmentDiscountStatus())
	}
	if rec.GetCommitmentDiscountType() != "" {
		t.Fatalf("type = %q", rec.GetCommitmentDiscountType())
	}
	if rec.GetExtendedColumns()["metric"] != metric {
		t.Fatalf("metric = %q, want %s", rec.GetExtendedColumns()["metric"], metric)
	}
	if err := pluginsdk.ValidateFocusRecord(rec); err != nil {
		t.Fatal(err)
	}
}

func assertFocusAmount(t *testing.T, rec *pbc.FocusCostRecord, want string) {
	t.Helper()
	gotText := rec.GetExtendedColumns()["amount_decimal"]
	got, ok := new(big.Rat).SetString(gotText)
	wantRat, wantOK := new(big.Rat).SetString(want)
	if !ok || !wantOK || got.Cmp(wantRat) != 0 {
		t.Fatalf("amount_decimal %q, want %s", gotText, want)
	}
	blended, _ := new(big.Rat).SetString("9.99")
	if got.Cmp(blended) == 0 {
		t.Fatalf("amount_decimal is blended 9.99")
	}
	wantFloat, _ := wantRat.Float64()
	for name, value := range map[string]float64{
		"billed":    rec.GetBilledCost(),
		"list":      rec.GetListCost(),
		"effective": rec.GetEffectiveCost(),
	} {
		if value == 9.99 {
			t.Fatalf("%s cost is blended 9.99", name)
		}
		if value != wantFloat {
			t.Fatalf("%s cost %v, want %v", name, value, wantFloat)
		}
	}
}

func assertMetrics(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("metrics %v, want %v", got, want)
	}
	have := make(map[string]int, len(got))
	for _, metric := range got {
		have[metric]++
	}
	for _, metric := range want {
		if have[metric] != 1 {
			t.Fatalf("metrics %v, want %v", got, want)
		}
	}
}
