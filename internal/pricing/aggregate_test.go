package pricing

import (
	"math/big"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
)

func TestAggregateCosts_FocusPeriodUsesLatestExclusiveEnd(t *testing.T) {
	start := time.Date(2017, 9, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2017, 9, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2017, 10, 1, 0, 0, 0, 0, time.UTC)
	amount := func(s string) *big.Rat {
		r, ok := new(big.Rat).SetString(s)
		if !ok {
			t.Fatalf("bad amount %s", s)
		}
		return r
	}
	rows := []client.CostResult{
		{
			ServiceName: "Amazon Simple Storage Service",
			Currency:    "USD",
			AmountExact: amount("10"),
			StartDate:   mid,
			EndDate:     end,
		},
		{
			ServiceName: "Amazon Simple Storage Service",
			Currency:    "USD",
			AmountExact: amount("29.2940765264"),
			StartDate:   start,
			EndDate:     mid,
		},
	}
	costs, err := aggregateCosts(rows)
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	if len(costs) != 1 {
		t.Fatalf("groups = %d", len(costs))
	}
	if !costs[0].Timestamp.Equal(start) {
		t.Fatalf("period start = %s", costs[0].Timestamp)
	}
	if !costs[0].PeriodEnd.Equal(end) {
		t.Fatalf("period end = %s", costs[0].PeriodEnd)
	}
	record := focusFor(costs[0])
	if got := record.GetChargePeriodStart().AsTime(); !got.Equal(start) {
		t.Fatalf("charge start = %s", got)
	}
	if got := record.GetChargePeriodEnd().AsTime(); !got.Equal(end) {
		t.Fatalf("charge end = %s, want %s", got, end)
	}
	if got := record.GetBillingPeriodEnd().AsTime(); !got.Equal(end) {
		t.Fatalf("billing end = %s, want %s", got, end)
	}
	if record.GetChargePeriodEnd().AsTime().Equal(start.Add(24 * time.Hour)) {
		t.Fatal("charge end is start+24h")
	}
}
