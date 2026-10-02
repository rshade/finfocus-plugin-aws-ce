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
		key := row.ServiceName
		a, ok := byKey[key]
		if !ok {
			a = &acc{entry: CostEntry{
				Timestamp:        row.StartDate.UTC(),
				Currency:         row.Currency,
				Service:          key,
				AccountID:        row.AccountID,
				Region:           row.Region,
				AvailabilityZone: row.AvailabilityZone,
				Tags:             row.Tags,
				ReservationARN:   row.ReservationARN,
				SavingsPlanARN:   row.SavingsPlanARN,
			}}
			byKey[key] = a
			order = append(order, key)
		}
		a.cost = addCost(a.cost, row.AmountExact)
		if row.Estimated {
			a.entry.Estimated = true
		}
		if row.StartDate.UTC().Before(a.entry.Timestamp) {
			a.entry.Timestamp = row.StartDate.UTC()
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

func focusFor(entry CostEntry) *pbc.FocusCostRecord {
	start := entry.Timestamp.UTC()
	end := start.Add(24 * time.Hour)
	account := entry.AccountID
	if account == "" {
		account = "unknown"
	}
	category := pbc.FocusChargeCategory_FOCUS_CHARGE_CATEGORY_ADJUSTMENT
	switch {
	case entry.Amount < 0:
		category = pbc.FocusChargeCategory_FOCUS_CHARGE_CATEGORY_CREDIT
	case entry.HasUsage && entry.UsageAmount > 0:
		category = pbc.FocusChargeCategory_FOCUS_CHARGE_CATEGORY_USAGE
	}
	estimated := strconv.FormatBool(entry.Estimated)
	builder := pluginsdk.NewFocusRecordBuilder().
		WithIdentity("AWS", account, account).
		WithBillingPeriod(start, end, entry.Currency).
		WithChargePeriod(start, end).
		WithChargeDetails(category, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_STANDARD).
		WithChargeClassification(
			pbc.FocusChargeClass_FOCUS_CHARGE_CLASS_REGULAR,
			"Unblended cost for "+entry.Service,
			pbc.FocusChargeFrequency_FOCUS_CHARGE_FREQUENCY_USAGE_BASED,
		).
		WithService(pbc.FocusServiceCategory_FOCUS_SERVICE_CATEGORY_OTHER, entry.Service).
		WithFinancials(entry.Amount, entry.Amount, entry.Amount, entry.Currency, "").
		WithExtension("estimated", estimated).
		WithExtension("group_key", entry.Service).
		WithExtension("currency", entry.Currency).
		WithExtension("amount_decimal", entry.AmountDecimal)
	if entry.HasUsage && entry.UsageAmount > 0 {
		builder = builder.WithUsage(entry.UsageAmount, entry.UsageUnit)
	}
	record, err := builder.Build()
	if err != nil {
		return &pbc.FocusCostRecord{
			ExtendedColumns: map[string]string{
				"estimated":      estimated,
				"group_key":      entry.Service,
				"currency":       entry.Currency,
				"amount_decimal": entry.AmountDecimal,
			},
		}
	}
	return record
}
