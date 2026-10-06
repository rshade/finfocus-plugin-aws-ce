package pricing

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

// addCost sums two decimal amounts. A nil sum starts a new total.
func addCost(sum, next *big.Rat) *big.Rat {
	if next == nil {
		return sum
	}
	if sum == nil {
		return new(big.Rat).Set(next)
	}
	return new(big.Rat).Add(sum, next)
}

// ratDecimal is the exact decimal text of r. Cost Explorer amounts are decimal,
// so the reduced denominator only has factors 2 and 5.
func ratDecimal(r *big.Rat) string {
	if r == nil {
		return ""
	}
	n := new(big.Int).Abs(r.Num())
	d := new(big.Int).Abs(r.Denom())
	two := big.NewInt(2)
	five := big.NewInt(5)
	twos, fives := 0, 0
	rem := new(big.Int)
	for rem.Mod(d, two).Sign() == 0 {
		d.Quo(d, two)
		twos++
	}
	for rem.Mod(d, five).Sign() == 0 {
		d.Quo(d, five)
		fives++
	}
	if d.Cmp(big.NewInt(1)) != 0 {
		return r.RatString()
	}
	scale := twos
	if fives > scale {
		scale = fives
	}
	for i := twos; i < scale; i++ {
		n.Mul(n, two)
	}
	for i := fives; i < scale; i++ {
		n.Mul(n, five)
	}
	digits := n.String()
	var b strings.Builder
	if r.Sign() < 0 {
		b.WriteByte('-')
	}
	if scale == 0 {
		b.WriteString(digits)
		return b.String()
	}
	if len(digits) <= scale {
		b.WriteString("0.")
		b.WriteString(strings.Repeat("0", scale-len(digits)))
		b.WriteString(digits)
		return b.String()
	}
	b.WriteString(digits[:len(digits)-scale])
	b.WriteByte('.')
	b.WriteString(digits[len(digits)-scale:])
	return b.String()
}

func aggregateCosts(rows []client.CostResult) ([]CostEntry, error) {
	type acc struct {
		entry CostEntry
		cost  *big.Rat
		usage *big.Rat
	}
	order := make([]string, 0)
	byKey := make(map[string]*acc)
	var currency string
	for _, row := range rows {
		if row.AmountExact == nil {
			return nil, fmt.Errorf("%w for group %q", client.ErrAmountMissing, row.ServiceName)
		}
		if row.Currency != "" {
			if currency == "" {
				currency = row.Currency
			} else if currency != row.Currency {
				return nil, fmt.Errorf("%w: %s and %s", client.ErrMixedCurrency, currency, row.Currency)
			}
		}
		key := costMergeKey(row)
		a, ok := byKey[key]
		if !ok {
			a = &acc{entry: CostEntry{
				Timestamp:        row.StartDate.UTC(),
				PeriodEnd:        row.EndDate.UTC(),
				Currency:         row.Currency,
				Service:          row.ServiceName,
				AccountID:        row.AccountID,
				Region:           row.Region,
				AvailabilityZone: row.AvailabilityZone,
				Tags:             row.Tags,
				ReservationARN:   row.ReservationARN,
				SavingsPlanARN:   row.SavingsPlanARN,
				Metric:           row.Metric,
			}}
			byKey[key] = a
			order = append(order, key)
		} else if row.Metric != "" && a.entry.Metric != "" && row.Metric != a.entry.Metric {
			return nil, fmt.Errorf("mixed cost metrics %s and %s for group %q", a.entry.Metric, row.Metric, row.ServiceName)
		}
		if a.entry.Metric == "" {
			a.entry.Metric = row.Metric
		}
		a.cost = addCost(a.cost, row.AmountExact)
		if row.Estimated {
			a.entry.Estimated = true
		}
		if row.StartDate.UTC().Before(a.entry.Timestamp) {
			a.entry.Timestamp = row.StartDate.UTC()
		}
		if end := row.EndDate.UTC(); !end.IsZero() && end.After(a.entry.PeriodEnd) {
			a.entry.PeriodEnd = end
		}
		if row.UsageExact != nil {
			a.usage = addCost(a.usage, row.UsageExact)
			a.entry.HasUsage = true
			if a.entry.UsageUnit == "" {
				a.entry.UsageUnit = row.UsageUnit
			}
		}
		if a.entry.Currency == "" && row.Currency != "" {
			a.entry.Currency = row.Currency
		}
	}

	out := make([]CostEntry, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		a.entry.AmountDecimal = ratDecimal(a.cost)
		cost, _ := a.cost.Float64()
		a.entry.Amount = cost
		if a.entry.HasUsage && a.usage != nil {
			usage, _ := a.usage.Float64()
			a.entry.UsageAmount = usage
		}
		out = append(out, a.entry)
	}
	return out, nil
}

func costMergeKey(row client.CostResult) string {
	return row.ServiceName + "\x00" + row.ReservationARN + "\x00" + row.SavingsPlanARN
}

func focusFor(entry CostEntry) *pbc.FocusCostRecord {
	start := entry.Timestamp.UTC()
	end := entry.PeriodEnd.UTC()
	if !end.After(start) {
		end = start.Add(24 * time.Hour)
	}
	account := entry.AccountID
	if account == "" {
		account = "unknown"
	}
	// A zero UsageQuantity is not a usage charge. FR-003 keys off this category.
	usageCharge := entry.HasUsage && entry.UsageAmount > 0
	category := pbc.FocusChargeCategory_FOCUS_CHARGE_CATEGORY_ADJUSTMENT
	switch {
	case entry.Amount < 0:
		category = pbc.FocusChargeCategory_FOCUS_CHARGE_CATEGORY_CREDIT
	case usageCharge:
		category = pbc.FocusChargeCategory_FOCUS_CHARGE_CATEGORY_USAGE
	}
	estimated := strconv.FormatBool(entry.Estimated)
	metric := entry.Metric
	if metric == "" {
		metric = "UnblendedCost"
	}
	ri := entry.ReservationARN
	sp := entry.SavingsPlanARN
	serviceName := entry.Service
	if serviceName == "" && (ri != "" || sp != "") {
		serviceName = "unknown"
	}
	description := "Unblended cost for " + serviceName
	if metric == "AmortizedCost" {
		description = "Amortized cost for " + serviceName
	}
	commitmentID := ""
	commitmentType := ""
	if ((ri != "") != (sp != "")) && !usageCharge {
		if ri != "" {
			commitmentID = ri
			commitmentType = "Reserved Instance"
		} else {
			commitmentID = sp
			commitmentType = "Savings Plan"
		}
	}
	copyRawIDs := usageCharge || (ri != "" && sp != "")
	builder := pluginsdk.NewFocusRecordBuilder().
		WithIdentity("AWS", account, account).
		WithBillingPeriod(start, end, entry.Currency).
		WithChargePeriod(start, end).
		WithChargeDetails(category, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_STANDARD).
		WithChargeClassification(
			pbc.FocusChargeClass_FOCUS_CHARGE_CLASS_REGULAR,
			description,
			pbc.FocusChargeFrequency_FOCUS_CHARGE_FREQUENCY_USAGE_BASED,
		).
		WithService(pbc.FocusServiceCategory_FOCUS_SERVICE_CATEGORY_OTHER, serviceName).
		WithFinancials(entry.Amount, entry.Amount, entry.Amount, entry.Currency, "").
		WithExtension("estimated", estimated).
		WithExtension("group_key", entry.Service).
		WithExtension("currency", entry.Currency).
		WithExtension("amount_decimal", entry.AmountDecimal).
		WithExtension("metric", metric).
		WithExtension("data_source", "AWS Cost Explorer").
		WithExtension("granularity", "DAILY").
		WithExtension("lookback", entry.Lookback)
	if commitmentID != "" {
		builder = builder.WithCommitmentDiscount(
			pbc.FocusCommitmentDiscountCategory_FOCUS_COMMITMENT_DISCOUNT_CATEGORY_UNSPECIFIED,
			commitmentID,
			"",
		)
	}
	if copyRawIDs {
		if ri != "" {
			builder = builder.WithExtension("reservation_id", ri)
		}
		if sp != "" {
			builder = builder.WithExtension("savings_plan_arn", sp)
		}
	}
	if usageCharge {
		builder = builder.WithUsage(entry.UsageAmount, entry.UsageUnit)
	}
	record, err := builder.Build()
	if err != nil {
		columns := map[string]string{
			"estimated":      estimated,
			"group_key":      entry.Service,
			"currency":       entry.Currency,
			"amount_decimal": entry.AmountDecimal,
			"metric":         metric,
			"data_source":    "AWS Cost Explorer",
			"granularity":    "DAILY",
			"lookback":       entry.Lookback,
		}
		if copyRawIDs {
			if ri != "" {
				columns["reservation_id"] = ri
			}
			if sp != "" {
				columns["savings_plan_arn"] = sp
			}
		}
		return &pbc.FocusCostRecord{ExtendedColumns: columns}
	}
	if commitmentType != "" {
		record.CommitmentDiscountType = commitmentType
	}
	return record
}
