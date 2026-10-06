package pricing

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestParserAmountRegression locks the contract cases that must not turn a bad
// amount into a successful zero. precision_small_and_large compares amount_decimal
// with big.Rat, not float equality.
func TestParserAmountRegression(t *testing.T) {
	doc := loadContract(t)
	byID := make(map[string]contractCase, len(doc.Cases))
	for _, tc := range doc.Cases {
		byID[tc.ID] = tc
	}
	for _, id := range []string{
		"amount_missing",
		"amount_unparseable",
		"real_zero",
		"precision_small_and_large",
		"mixed_currencies",
	} {
		tc, ok := byID[id]
		if !ok {
			t.Fatalf("contract case %s not found", id)
		}
		t.Run(id, func(t *testing.T) {
			f := newFakeCE(t, tc.Pages)
			start, end := recentWindow()
			resp, err := callActual(t, f, tc.ID, "", start, end)
			switch id {
			case "amount_missing", "amount_unparseable":
				assertNotZeroCost(t, resp, err)
			case "real_zero":
				assertRealZero(t, resp, err)
			case "precision_small_and_large":
				assertPrecisionRats(t, tc, resp, err)
			case "mixed_currencies":
				assertMixedCurrencies(t, resp, err)
			default:
				t.Fatalf("unhandled case %s", id)
			}
		})
	}
}

func assertNotZeroCost(t *testing.T, resp *pbc.GetActualCostResponse, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("parser returned a zero cost instead of an error: %v", resultCosts(resp))
	}
	if resp != nil && len(resp.GetResults()) > 0 {
		t.Fatalf("error plus cost rows %v: %v", resultCosts(resp), err)
	}
	code := status.Code(err)
	if code != codes.Internal && code != codes.DataLoss {
		t.Fatalf("status %s, want Internal or DataLoss: %v", code, err)
	}
}

func assertRealZero(t *testing.T, resp *pbc.GetActualCostResponse, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("real zero: %v", err)
	}
	results := resp.GetResults()
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	res := results[0]
	if res.GetCost() != 0 {
		t.Fatalf("cost = %v, want 0", res.GetCost())
	}
	got := resultAmountDecimal(res)
	gotRat, ok := new(big.Rat).SetString(got)
	if !ok || gotRat.Sign() != 0 {
		t.Fatalf("amount_decimal %q, want 0", got)
	}
	if res.GetUsageAmount() != 0 || res.GetUsageUnit() != "" {
		t.Fatalf("absent usage = %v %q, want 0 and empty", res.GetUsageAmount(), res.GetUsageUnit())
	}
}

func assertPrecisionRats(t *testing.T, tc contractCase, resp *pbc.GetActualCostResponse, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("precision: %v", err)
	}
	got := map[string]*pbc.ActualCostResult{}
	for _, res := range resp.GetResults() {
		got[resultGroupKey(res)] = res
	}
	sum := new(big.Rat)
	for key, want := range tc.Expect.PerService {
		res := got[key]
		if res == nil {
			t.Fatalf("missing group %s", key)
		}
		part := decimalRat(t, resultAmountDecimal(res))
		wantRat := decimalRat(t, want)
		if part.Cmp(wantRat) != 0 {
			t.Fatalf("%s amount_decimal %s != %s", key, part.RatString(), wantRat.RatString())
		}
		sum.Add(sum, part)
		if res.GetUsageUnit() == tc.Expect.Currency && tc.Expect.Currency != "" {
			t.Fatalf("%s usage unit is the currency", key)
		}
		if res.GetUsageAmount() != 0 || res.GetUsageUnit() != "" {
			t.Fatalf("%s absent usage = %v %q", key, res.GetUsageAmount(), res.GetUsageUnit())
		}
	}
	total := decimalRat(t, tc.Expect.Total)
	if sum.Cmp(total) != 0 {
		t.Fatalf("amount_decimal sum %s != %s", sum.RatString(), total.RatString())
	}
}

func assertMixedCurrencies(t *testing.T, resp *pbc.GetActualCostResponse, err error) {
	t.Helper()
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("status %s, want FailedPrecondition: %v", status.Code(err), err)
	}
	if details := status.Convert(err).Details(); len(details) != 0 {
		t.Fatalf("status details = %#v, want none", details)
	}
	if resp != nil && len(resp.GetResults()) > 0 {
		t.Fatalf("mixed currencies returned costs %v", resultCosts(resp))
	}
}

func decimalRat(t *testing.T, text string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		t.Fatalf("not a decimal: %q", text)
	}
	return r
}

func resultCosts(resp *pbc.GetActualCostResponse) []float64 {
	if resp == nil {
		return nil
	}
	out := make([]float64, 0, len(resp.GetResults()))
	for _, res := range resp.GetResults() {
		out = append(out, res.GetCost())
	}
	return out
}

func TestServiceAndAccountDecimalSum(t *testing.T) {
	start, end := recentWindow()
	ctx := context.Background()

	t.Run("three_1_10", func(t *testing.T) {
		// 1.10+1.10+1.10 is 3.3000000000000003 as float64. One conversion of 3.30 is not.
		calc := calculatorWithCostGroups(t, unblendedGroups(
			moneyRow{key: "BoxUsage", unit: "USD", amount: strPtr("1.10")},
			moneyRow{key: "DataTransfer", unit: "USD", amount: strPtr("1.10")},
			moneyRow{key: "SpotUsage", unit: "USD", amount: strPtr("1.10")},
		))
		want, _ := decimalRat(t, "3.30").Float64()
		checkTotal(t, ctx, calc, start, end, want, "USD", nil)
	})

	t.Run("real_zero", func(t *testing.T) {
		calc := calculatorWithCostGroups(t, unblendedGroups(
			moneyRow{key: "AWS Free Tier", unit: "USD", amount: strPtr("0.0")},
		))
		checkTotal(t, ctx, calc, start, end, 0, "USD", nil)
	})

	t.Run("amount_missing", func(t *testing.T) {
		calc := calculatorWithCostGroups(t, unblendedGroups(
			moneyRow{key: "X", unit: "USD"},
		))
		checkTotal(t, ctx, calc, start, end, 0, "", client.ErrAmountMissing)
	})

	t.Run("amount_unparseable", func(t *testing.T) {
		calc := calculatorWithCostGroups(t, unblendedGroups(
			moneyRow{key: "X", unit: "USD", amount: strPtr("not-a-number")},
		))
		checkTotal(t, ctx, calc, start, end, 0, "", client.ErrAmountUnparseable)
	})

	t.Run("mixed_currencies", func(t *testing.T) {
		calc := calculatorWithCostGroups(t, unblendedGroups(
			moneyRow{key: "X", unit: "USD", amount: strPtr("1.00")},
			moneyRow{key: "Y", unit: "EUR", amount: strPtr("1.00")},
		))
		checkTotal(t, ctx, calc, start, end, 0, "", client.ErrMixedCurrency)
	})
}

func checkTotal(t *testing.T, ctx context.Context, calc *Calculator, start, end time.Time, want float64, currency string, wantErr error) {
	t.Helper()
	calls := []struct {
		name string
		fn   func() (float64, string, error)
	}{
		{name: "GetServiceActualCost", fn: func() (float64, string, error) {
			return calc.GetServiceActualCost(ctx, "AmazonEC2", start, end)
		}},
		{name: "GetAccountActualCost", fn: func() (float64, string, error) {
			return calc.GetAccountActualCost(ctx, start, end)
		}},
	}
	for _, call := range calls {
		got, cur, err := call.fn()
		if wantErr != nil {
			if err == nil {
				t.Fatalf("%s returned success %v %s, want an error instead of a total", call.name, got, cur)
			}
			if !errors.Is(err, wantErr) {
				t.Fatalf("%s err = %v, want %v", call.name, err, wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", call.name, err)
		}
		if cur != currency {
			t.Fatalf("%s currency = %s, want %s", call.name, cur, currency)
		}
		if math.Float64bits(got) != math.Float64bits(want) {
			t.Fatalf("%s bits %#x != %#x (one conversion of the decimal sum)", call.name, math.Float64bits(got), math.Float64bits(want))
		}
	}
}

type moneyRow struct {
	key    string
	unit   string
	amount *string
}

func strPtr(s string) *string { return &s }

func unblendedGroups(rows ...moneyRow) []types.Group {
	groups := make([]types.Group, 0, len(rows))
	for _, row := range rows {
		metric := types.MetricValue{}
		if row.unit != "" {
			metric.Unit = aws.String(row.unit)
		}
		if row.amount != nil {
			metric.Amount = aws.String(*row.amount)
		}
		key := row.key
		if key == "" {
			key = "X"
		}
		groups = append(groups, types.Group{
			Keys: []string{key},
			Metrics: map[string]types.MetricValue{
				"UnblendedCost": metric,
			},
		})
	}
	return groups
}

func calculatorWithCostGroups(t *testing.T, groups []types.Group) *Calculator {
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
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	return calc
}
