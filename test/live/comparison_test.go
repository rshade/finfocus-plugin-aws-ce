package live

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

func compareLiveCost(periods []types.ResultByTime, response *pbc.GetActualCostResponse, account string) error {
	type expected struct {
		amount     *big.Rat
		currency   string
		estimated  bool
		start, end time.Time
	}
	wanted := map[string]*expected{}
	seen := map[string]bool{}
	for _, period := range periods {
		if period.TimePeriod == nil {
			return fmt.Errorf("AWS period is missing")
		}
		start, err := time.Parse(time.DateOnly, aws.ToString(period.TimePeriod.Start))
		if err != nil {
			return fmt.Errorf("invalid AWS start period")
		}
		end, err := time.Parse(time.DateOnly, aws.ToString(period.TimePeriod.End))
		if err != nil || !end.After(start) {
			return fmt.Errorf("invalid AWS end period")
		}
		for _, group := range period.Groups {
			if len(group.Keys) != 1 {
				return fmt.Errorf("expected SERVICE grouping")
			}
			service := group.Keys[0]
			unique := start.Format(time.DateOnly) + "/" + service
			if seen[unique] {
				return fmt.Errorf("duplicate AWS group")
			}
			seen[unique] = true
			metric, ok := group.Metrics["AmortizedCost"]
			if !ok || metric.Amount == nil || aws.ToString(metric.Unit) == "" {
				return fmt.Errorf("missing AWS metric")
			}
			amount, ok := new(big.Rat).SetString(*metric.Amount)
			if !ok {
				return fmt.Errorf("invalid AWS amount")
			}
			want := wanted[service]
			if want == nil {
				want = &expected{amount: new(big.Rat), currency: aws.ToString(metric.Unit), start: start, end: end}
				wanted[service] = want
			}
			if want.currency != aws.ToString(metric.Unit) {
				return fmt.Errorf("mixed AWS currency")
			}
			want.amount.Add(want.amount, amount)
			want.estimated = want.estimated || period.Estimated
			if start.Before(want.start) {
				want.start = start
			}
			if end.After(want.end) {
				want.end = end
			}
		}
	}
	if len(wanted) == 0 {
		return fmt.Errorf("no grouped AWS costs to compare")
	}
	type actual struct {
		amount     *big.Rat
		cost       float64
		estimated  bool
		start, end time.Time
	}
	got := map[string]*actual{}
	for _, row := range response.GetResults() {
		if row == nil || row.GetTimestamp() == nil || row.GetFocusRecord() == nil {
			return fmt.Errorf("missing plugin record")
		}
		focus := row.GetFocusRecord()
		service := focus.GetServiceName()
		want, ok := wanted[service]
		if !ok {
			return fmt.Errorf("unexpected plugin service")
		}
		if focus.GetChargePeriodStart() == nil || focus.GetChargePeriodEnd() == nil {
			return fmt.Errorf("missing plugin charge period")
		}
		start, end := focus.GetChargePeriodStart().AsTime(), focus.GetChargePeriodEnd().AsTime()
		if !row.GetTimestamp().AsTime().Equal(start) || !end.After(start) || start.Before(want.start) || end.After(want.end) {
			return fmt.Errorf("plugin charge period differs from AWS")
		}
		if math.IsNaN(row.GetCost()) || math.IsInf(row.GetCost(), 0) {
			return fmt.Errorf("invalid plugin amount")
		}
		exact, ok := new(big.Rat).SetString(focus.GetExtendedColumns()["amount_decimal"])
		if !ok {
			return fmt.Errorf("invalid plugin exact decimal")
		}
		estimated, err := strconv.ParseBool(focus.GetExtendedColumns()["estimated"])
		if err != nil || focus.GetBillingAccountId() != account || focus.GetBillingCurrency() != want.currency || row.GetSource() != "aws-ce" {
			return fmt.Errorf("plugin provenance, currency or estimate differs")
		}
		sum := got[service]
		if sum == nil {
			sum = &actual{amount: new(big.Rat), start: start, end: end}
			got[service] = sum
		}
		sum.amount.Add(sum.amount, exact)
		sum.cost += row.GetCost()
		sum.estimated = sum.estimated || estimated
		if start.Before(sum.start) {
			sum.start = start
		}
		if end.After(sum.end) {
			sum.end = end
		}
	}
	if len(got) != len(wanted) {
		return fmt.Errorf("service count differs: got %d want %d", len(got), len(wanted))
	}
	for service, want := range wanted {
		sum := got[service]
		if sum == nil || !sum.start.Equal(want.start) || !sum.end.Equal(want.end) || sum.estimated != want.estimated {
			return fmt.Errorf("plugin period or estimate differs from AWS for %s", service)
		}
		amount, _ := want.amount.Float64()
		if math.IsNaN(sum.cost) || math.IsInf(sum.cost, 0) || math.Abs(sum.cost-amount) > 1e-9*math.Max(1, math.Abs(amount)) {
			return fmt.Errorf("plugin amount differs from AWS for %s", service)
		}
		if sum.amount.Cmp(want.amount) != 0 {
			return fmt.Errorf("plugin exact decimal differs from AWS for %s", service)
		}
	}
	return nil
}
