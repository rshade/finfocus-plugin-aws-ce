package pricing

import "time"

// CostEntry represents a single cost data point.
type CostEntry struct {
	Timestamp        time.Time         `json:"timestamp"`
	PeriodEnd        time.Time         `json:"period_end,omitempty"`
	Amount           float64           `json:"amount"` // Single float64 conversion of the decimal sum
	AmountDecimal    string            `json:"amount_decimal,omitempty"`
	Currency         string            `json:"currency"`
	Service          string            `json:"service"`
	UsageAmount      float64           `json:"usage_amount,omitempty"`
	UsageUnit        string            `json:"usage_unit,omitempty"`
	HasUsage         bool              `json:"has_usage,omitempty"`
	Estimated        bool              `json:"estimated,omitempty"`
	AccountID        string            `json:"account_id"`
	Region           string            `json:"region"`
	AvailabilityZone string            `json:"availability_zone"`
	Tags             map[string]string `json:"tags"`
	ReservationARN   string            `json:"reservation_arn,omitempty"`
	SavingsPlanARN   string            `json:"savings_plan_arn,omitempty"`
	Lookback         string            `json:"lookback,omitempty"`
	Metric           string            `json:"metric,omitempty"`
}

// CacheEntry represents a cached cost query result.
type CacheEntry struct {
	QueryKey  string      `json:"query_key"`
	Results   []CostEntry `json:"results"`
	CreatedAt time.Time   `json:"created_at"`
	ExpiresAt time.Time   `json:"expires_at"`
	FilePath  string      `json:"-"` // Not serialized
}
