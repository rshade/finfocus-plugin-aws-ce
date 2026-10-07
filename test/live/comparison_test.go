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
			metric, ok := group.Metrics["UnblendedCost"]
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
	if len(response.GetResults()) != len(wanted) {
		return fmt.Errorf("result count differs: got %d want %d", len(response.GetResults()), len(wanted))
	}
	for _, row := range response.GetResults() {
		if row == nil || row.GetTimestamp() == nil || row.GetFocusRecord() == nil {
			return fmt.Errorf("missing plugin record")
		}
		focus := row.GetFocusRecord()
		service := focus.GetServiceName()
		want, ok := wanted[service]
		if !ok {
			return fmt.Errorf("unexpected or duplicate plugin service")
		}
		if !row.GetTimestamp().AsTime().Equal(want.start) || focus.GetChargePeriodStart() == nil || focus.GetChargePeriodEnd() == nil || !focus.GetChargePeriodStart().AsTime().Equal(want.start) || !focus.GetChargePeriodEnd().AsTime().Equal(want.end) {
			return fmt.Errorf("plugin charge period differs from AWS")
		}
		amount, _ := want.amount.Float64()
		if math.IsNaN(row.GetCost()) || math.IsInf(row.GetCost(), 0) || math.Abs(row.GetCost()-amount) > 1e-9*math.Max(1, math.Abs(amount)) {
			return fmt.Errorf("plugin amount differs from AWS")
		}
		exact, ok := new(big.Rat).SetString(focus.GetExtendedColumns()["amount_decimal"])
		if !ok || exact.Cmp(want.amount) != 0 {
			return fmt.Errorf("plugin exact decimal differs from AWS")
		}
		if focus.GetBillingAccountId() != account || focus.GetBillingCurrency() != want.currency || focus.GetExtendedColumns()["estimated"] != strconv.FormatBool(want.estimated) || row.GetSource() != "aws-ce" {
			return fmt.Errorf("plugin provenance, currency or estimate differs")
		}
		delete(wanted, service)
	}
	return nil
}
