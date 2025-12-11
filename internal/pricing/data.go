package pricing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ResourceDescriptor identifies the AWS resource or scope being queried.
// It maps to the pluginsdk ResourceDescriptor.
type ResourceDescriptor struct {
	Provider   string
	Type       string
	ID         string
	Properties map[string]interface{}
}

// DateRange defines the time period for cost queries.
type DateRange struct {
	Start time.Time
	End   time.Time
}

// CostEntry represents a single cost data point.
type CostEntry struct {
	Timestamp        time.Time         `json:"timestamp"`
	Amount           float64           `json:"amount"` // Using float64 for simplicity, matching client.CostResult
	Currency         string            `json:"currency"`
	Service          string            `json:"service"`
	AccountID        string            `json:"account_id"`
	Region           string            `json:"region"`
	AvailabilityZone string            `json:"availability_zone"`
	Tags             map[string]string `json:"tags"`
	ReservationARN   string            `json:"reservation_arn,omitempty"`
	SavingsPlanARN   string            `json:"savings_plan_arn,omitempty"`
}

// ReservationData contains information about RI or Savings Plan utilization.
type ReservationData struct {
	ReservationARN        string    `json:"reservation_arn"`
	InstanceType          string    `json:"instance_type,omitempty"`
	Region                string    `json:"region,omitempty"`
	UtilizationPercentage float64   `json:"utilization_percentage"`
	CoveragePercentage    float64   `json:"coverage_percentage"`
	TotalCost             float64   `json:"total_cost"`
	UnusedCost            float64   `json:"unused_cost"`
	StartDate             time.Time `json:"start_date"`
	EndDate               time.Time `json:"end_date"`
}

// CacheEntry represents a cached cost query result.
type CacheEntry struct {
	QueryKey  string      `json:"query_key"`
	Results   []CostEntry `json:"results"`
	CreatedAt time.Time   `json:"created_at"`
	ExpiresAt time.Time   `json:"expires_at"`
	FilePath  string      `json:"-"` // Not serialized
}

// FallbackHint enum values.
// Note: This should ideally come from the SDK. Defining locally if not available.
// Using pbc.FallbackHint if available, otherwise just mapping logic.
// Checking imports, pbc is "github.com/rshade/pulumicost-spec/sdk/go/proto/pulumicost/v1".
// If the SDK doesn't have it yet, we'll need to handle it or wait.
// For now, assuming it's NOT in SDK based on analysis, but we need it for logic.
// We will use pbc.FallbackHint if we can, or just comments for now.

// PricingData represents pricing information for resources.
// Kept for compatibility if needed, but CostEntry is the main entity now.
type PricingData struct {
	Provider     string             `json:"provider"`
	Region       string             `json:"region"`
	ResourceType string             `json:"resource_type"`
	Pricing      map[string]float64 `json:"pricing"`
}

// LoadPricingData loads pricing data from a JSON file.
func LoadPricingData(path string) ([]PricingData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pricing data: %w", err)
	}

	var pricing []PricingData
	if err := json.Unmarshal(data, &pricing); err != nil {
		return nil, fmt.Errorf("parsing pricing data: %w", err)
	}

	return pricing, nil
}

// SavePricingData saves pricing data to a JSON file.
func SavePricingData(path string, data []PricingData) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling pricing data: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0600); err != nil {
		return fmt.Errorf("writing pricing data: %w", err)
	}

	return nil
}

// ToProto converts a CostEntry to a protobuf CostEntry.
// Placeholder removed to fix build errors.
// Conversion will be handled in calculator.go.
func (c *CostEntry) ToProto() {
	// Removed
}