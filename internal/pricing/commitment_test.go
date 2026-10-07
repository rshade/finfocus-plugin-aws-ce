package pricing

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
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
)

// These fixtures exercise FOCUS mapping after Cost Explorer parsing. Commitment
// identifiers are discovered/filter scoped by the client, not API GroupBy keys.
func TestCommitmentLineMapsToFocusRecord(t *testing.T) {
	for _, tc := range []struct {
		name, reservation, savings, amount, metric, wantID, wantType string
		usage                                                        *big.Rat
	}{
		{name: "reservation", reservation: reservationKey, amount: "2.50", metric: "AmortizedCost", wantID: reservationKey, wantType: "Reserved Instance"},
		{name: "savings_plan", savings: savingsPlanKey, amount: "2.50", metric: "AmortizedCost", wantID: savingsPlanKey, wantType: "Savings Plan"},
		{name: "service_only", amount: "1.00", metric: "UnblendedCost"},
		{name: "both_ids", reservation: reservationKey, savings: savingsPlanKey, amount: "3.00", metric: "UnblendedCost"},
		{name: "missing_amortized", reservation: reservationKey, amount: "3.00", metric: "UnblendedCost", wantID: reservationKey, wantType: "Reserved Instance"},
		{name: "usage_with_reservation", reservation: reservationKey, amount: "2.50", metric: "AmortizedCost", usage: big.NewRat(1, 1)},
		{name: "zero_usage_reservation", reservation: reservationKey, amount: "2.50", metric: "AmortizedCost", usage: big.NewRat(0, 1), wantID: reservationKey, wantType: "Reserved Instance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			amount, _ := new(big.Rat).SetString(tc.amount)
			row := client.CostResult{AmountExact: amount, Currency: "USD", ServiceName: "AmazonEC2", ReservationARN: tc.reservation, SavingsPlanARN: tc.savings, Metric: tc.metric, StartDate: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), EndDate: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), UsageExact: tc.usage}
			if tc.usage != nil {
				row.UsageUnit = "Hrs"
			}
			costs, err := aggregateCosts([]client.CostResult{row})
			if err != nil || len(costs) != 1 {
				t.Fatalf("aggregate costs=%v err=%v", costs, err)
			}
			rec := focusFor(costs[0])
			assertFocusAmount(t, rec, tc.amount)
			if rec.GetCommitmentDiscountId() != tc.wantID || rec.GetCommitmentDiscountType() != tc.wantType {
				t.Fatalf("id=%q type=%q", rec.GetCommitmentDiscountId(), rec.GetCommitmentDiscountType())
			}
			if rec.GetExtendedColumns()["metric"] != tc.metric || rec.GetExtendedColumns()["group_key"] != "AmazonEC2" {
				t.Fatalf("extended columns=%v", rec.GetExtendedColumns())
			}
			if err := pluginsdk.ValidateFocusRecord(rec); err != nil {
				t.Fatal(err)
			}
			if tc.usage == nil {
				assertCommitmentShape(t, rec, tc.metric)
			}
			if tc.usage != nil && tc.usage.Sign() > 0 && rec.GetExtendedColumns()["reservation_id"] != reservationKey {
				t.Fatalf("usage reservation metadata=%v", rec.GetExtendedColumns())
			}
		})
	}
}

func TestGetActualCost_CommitmentFieldsUnset(t *testing.T) {
	mockAPI := pricedCostAPI()
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	start, end := recentCostWindow()
	req := costRequest("aws-account-total", "", start, end, nil)
	req.BillingAccountId = "123456789012"
	resp, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	assertServiceTotalQuery(t, mockAPI)
	assertMetrics(t, mockAPI.costCalls[0].Metrics, "UnblendedCost")
	if len(resp.GetResults()) != 1 {
		t.Fatalf("results=%d", len(resp.GetResults()))
	}
	rec := resp.GetResults()[0].GetFocusRecord()
	if rec.GetCommitmentDiscountId() != "" || rec.GetCommitmentDiscountType() != "" || rec.GetCommitmentDiscountUnit() != "" {
		t.Fatalf("unexpected commitment metadata: %v", rec)
	}
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

func TestGetActualCostCommitmentAttributionThroughRPC(t *testing.T) {
	var mu sync.Mutex
	var discoveries []string
	var queries []*costexplorer.GetCostAndUsageInput
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		mu.Lock()
		defer mu.Unlock()
		target := r.Header.Get("X-Amz-Target")
		switch target[strings.LastIndex(target, ".")+1:] {
		case "GetDimensionValues":
			var input costexplorer.GetDimensionValuesInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			discoveries = append(discoveries, string(input.Dimension))
			id := reservationKey
			if input.Dimension == types.DimensionSavingsPlanArn {
				id = savingsPlanKey
			} else if input.Dimension != types.DimensionReservationId {
				t.Errorf("unexpected discovery %s", input.Dimension)
			}
			if input.TimePeriod == nil || aws.ToString(input.TimePeriod.Start) != "2026-09-01" || aws.ToString(input.TimePeriod.End) != "2026-09-03" || input.Context != types.ContextCostAndUsage {
				t.Errorf("discovery input=%v", input)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"DimensionValues": []map[string]any{{"Value": id}}})
		case "GetCostAndUsage":
			var input costexplorer.GetCostAndUsageInput
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			queries = append(queries, &input)
			amount, unblended := "1.00", "1.00"
			if len(queries) == 1 {
				amount, unblended = "2.50", "3.00"
			}
			if len(queries) == 2 {
				amount, unblended = "4.00", "5.00"
			}
			// Responses deliberately include unconstrained quantities in incompatible
			// units; the API boundary must omit them from consumer-facing records.
			periods := []map[string]any{}
			for day, unit := range []string{"Hrs", "GB"} {
				value := amount
				plain := unblended
				if unit == "GB" {
					value, plain = "0.00", "0.00"
				}
				group := map[string]any{"Keys": []string{"AmazonEC2"}, "Metrics": map[string]any{"UnblendedCost": map[string]string{"Amount": plain, "Unit": "USD"}, "AmortizedCost": map[string]string{"Amount": value, "Unit": "USD"}, "UsageQuantity": map[string]string{"Amount": "10", "Unit": unit}}}
				dates := []string{"2026-09-01", "2026-09-02", "2026-09-03"}
				periods = append(periods, map[string]any{"TimePeriod": map[string]string{"Start": dates[day], "End": dates[day+1]}, "Groups": []map[string]any{group}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ResultsByTime": periods})
		default:
			t.Errorf("unexpected API operation %s", r.Header.Get("X-Amz-Target"))
			w.WriteHeader(400)
		}
	}))
	defer server.Close()
	ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculatorWithClient(ce)
	calc.cache = nil
	rpc := pluginsdk.NewTestServer(t, calc)
	defer rpc.Close()
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	req := costRequest("aws-account-total", "", start, start.Add(48*time.Hour), nil)
	req.BillingAccountId = "123456789012"
	response, err := rpc.Client().GetActualCost(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.GetResults()) != 3 {
		t.Fatalf("results=%v", response)
	}
	total := 0.0
	for i, want := range []struct{ id, kind, amount, metric string }{{reservationKey, "Reserved Instance", "2.50", "AmortizedCost"}, {savingsPlanKey, "Savings Plan", "4.00", "AmortizedCost"}, {"", "", "1.00", "UnblendedCost"}} {
		result := response.GetResults()[i]
		record := result.GetFocusRecord()
		assertFocusAmount(t, record, want.amount)
		if record.GetCommitmentDiscountId() != want.id || record.GetCommitmentDiscountType() != want.kind || record.GetExtendedColumns()["metric"] != want.metric {
			t.Fatalf("result %d metadata=%v", i, record)
		}
		if result.GetUsageAmount() != 0 || result.GetUsageUnit() != "" || record.GetConsumedQuantity() != 0 || record.GetConsumedUnit() != "" {
			t.Fatalf("mixed usage leaked: %v", result)
		}
		if err := pluginsdk.ValidateFocusRecord(record); err != nil {
			t.Fatal(err)
		}
		total += result.GetCost()
	}
	if total != 7.5 {
		t.Fatalf("partitioned total=%v, want 7.5", total)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(discoveries, []string{"RESERVATION_ID", "SAVINGS_PLAN_ARN"}) || len(queries) != 3 {
		t.Fatalf("discoveries=%v queries=%d", discoveries, len(queries))
	}
	ri := &types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionReservationId, Values: []string{reservationKey}}}
	sp := &types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionSavingsPlanArn, Values: []string{savingsPlanKey}}}
	wantFilters := []*types.Expression{ri, {And: []types.Expression{*sp, {Not: ri}}}, {And: []types.Expression{{Not: ri}, {Not: sp}}}}
	for i, query := range queries {
		if len(query.GroupBy) != 1 || aws.ToString(query.GroupBy[0].Key) != "SERVICE" || query.GroupBy[0].Type != types.GroupDefinitionTypeDimension {
			t.Fatalf("query %d unsupported grouping=%v", i, query.GroupBy)
		}
		if !reflect.DeepEqual(query.Filter, wantFilters[i]) {
			t.Fatalf("query %d filter=%v want=%v", i, query.Filter, wantFilters[i])
		}
		metrics := []string{"UnblendedCost"}
		if i < 2 {
			metrics = append(metrics, "AmortizedCost")
		}
		assertMetrics(t, query.Metrics, metrics...)
		if query.TimePeriod == nil || aws.ToString(query.TimePeriod.Start) != "2026-09-01" || aws.ToString(query.TimePeriod.End) != "2026-09-03" {
			t.Fatalf("query %d period=%v", i, query.TimePeriod)
		}
	}
}
