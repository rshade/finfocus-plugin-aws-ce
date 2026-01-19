package client

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
)

// mockCostExplorerAPI implements CostExplorerAPI for testing.
type mockCostExplorerAPI struct {
	getCostAndUsageFunc         func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	getReservationUtilizationFn func(ctx context.Context, params *costexplorer.GetReservationUtilizationInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetReservationUtilizationOutput, error)
	getSavingsPlansCoverageFn   func(ctx context.Context, params *costexplorer.GetSavingsPlansCoverageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansCoverageOutput, error)
}

func (m *mockCostExplorerAPI) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	if m.getCostAndUsageFunc != nil {
		return m.getCostAndUsageFunc(ctx, params, optFns...)
	}
	return &costexplorer.GetCostAndUsageOutput{}, nil
}

func (m *mockCostExplorerAPI) GetReservationUtilization(ctx context.Context, params *costexplorer.GetReservationUtilizationInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetReservationUtilizationOutput, error) {
	if m.getReservationUtilizationFn != nil {
		return m.getReservationUtilizationFn(ctx, params, optFns...)
	}
	return &costexplorer.GetReservationUtilizationOutput{}, nil
}

func (m *mockCostExplorerAPI) GetSavingsPlansCoverage(ctx context.Context, params *costexplorer.GetSavingsPlansCoverageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansCoverageOutput, error) {
	if m.getSavingsPlansCoverageFn != nil {
		return m.getSavingsPlansCoverageFn(ctx, params, optFns...)
	}
	return &costexplorer.GetSavingsPlansCoverageOutput{}, nil
}

func (m *mockCostExplorerAPI) GetCostForecast(ctx context.Context, params *costexplorer.GetCostForecastInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
	return nil, nil
}

func TestNewClientWithAPI(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{}
	client := NewClientWithAPI(mockAPI, "us-east-1")

	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Region() != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got %s", client.Region())
	}
}

func TestClient_GetCost_Success(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		getCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						TimePeriod: &types.DateInterval{
							Start: aws.String("2024-01-01"),
							End:   aws.String("2024-01-02"),
						},
						Groups: []types.Group{
							{
								Keys: []string{"Amazon EC2"},
								Metrics: map[string]types.MetricValue{
									"UnblendedCost": {
										Amount: aws.String("100.50"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	client := NewClientWithAPI(mockAPI, "us-east-1")
	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	results, err := client.GetCost(ctx, nil, []string{"SERVICE"}, startTime, endTime, "DAILY")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Amount != 100.50 {
		t.Errorf("expected amount 100.50, got %f", results[0].Amount)
	}
	if results[0].Currency != "USD" {
		t.Errorf("expected currency USD, got %s", results[0].Currency)
	}
	if results[0].ServiceName != "Amazon EC2" {
		t.Errorf("expected service 'Amazon EC2', got %s", results[0].ServiceName)
	}
}

func TestClient_GetCost_DefaultGranularity(t *testing.T) {
	var capturedInput *costexplorer.GetCostAndUsageInput
	mockAPI := &mockCostExplorerAPI{
		getCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			capturedInput = params
			return &costexplorer.GetCostAndUsageOutput{}, nil
		},
	}

	client := NewClientWithAPI(mockAPI, "us-east-1")
	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	// Call with empty granularity
	_, _ = client.GetCost(ctx, nil, []string{}, startTime, endTime, "")

	if capturedInput.Granularity != types.GranularityDaily {
		t.Errorf("expected default granularity DAILY, got %v", capturedInput.Granularity)
	}
}

func TestClient_GetServiceCost(t *testing.T) {
	var capturedInput *costexplorer.GetCostAndUsageInput
	mockAPI := &mockCostExplorerAPI{
		getCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			capturedInput = params
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						TimePeriod: &types.DateInterval{
							Start: aws.String("2024-01-01"),
							End:   aws.String("2024-01-02"),
						},
						Groups: []types.Group{
							{
								Keys: []string{"BoxUsage"},
								Metrics: map[string]types.MetricValue{
									"UnblendedCost": {
										Amount: aws.String("50.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	client := NewClientWithAPI(mockAPI, "us-east-1")
	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	results, err := client.GetServiceCost(ctx, "Amazon EC2", startTime, endTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Verify filter was set correctly
	if capturedInput.Filter == nil || capturedInput.Filter.Dimensions == nil {
		t.Fatal("expected filter to be set")
	}
	if capturedInput.Filter.Dimensions.Key != types.DimensionService {
		t.Errorf("expected SERVICE dimension filter")
	}
}

func TestClient_GetAccountCost(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		getCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{
				ResultsByTime: []types.ResultByTime{
					{
						TimePeriod: &types.DateInterval{
							Start: aws.String("2024-01-01"),
							End:   aws.String("2024-01-02"),
						},
						Groups: []types.Group{
							{
								Keys: []string{"Amazon S3"},
								Metrics: map[string]types.MetricValue{
									"UnblendedCost": {
										Amount: aws.String("25.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
							{
								Keys: []string{"Amazon EC2"},
								Metrics: map[string]types.MetricValue{
									"UnblendedCost": {
										Amount: aws.String("75.00"),
										Unit:   aws.String("USD"),
									},
								},
							},
						},
					},
				},
			}, nil
		},
	}

	client := NewClientWithAPI(mockAPI, "us-east-1")
	ctx := context.Background()
	startTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	results, err := client.GetAccountCost(ctx, startTime, endTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestClient_GetSupportedRegions(t *testing.T) {
	client := NewClientWithAPI(&mockCostExplorerAPI{}, "us-east-1")

	regions, err := client.GetSupportedRegions(context.Background())

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(regions) == 0 {
		t.Error("expected non-empty regions list")
	}

	// Check that us-east-1 is included
	found := false
	for _, r := range regions {
		if r == "us-east-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected us-east-1 in supported regions")
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123456789012", true},  // valid account ID
		{"000000000000", true},  // all zeros
		{"12345678901a", false}, // has letter
		{"", true},              // empty string is vacuously numeric
		{"12-34", false},        // has dash
	}

	for _, tc := range tests {
		result := isNumeric(tc.input)
		if result != tc.expected {
			t.Errorf("isNumeric(%q) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"Throttling error", "Throttling", true},
		{"RateExceeded", "Rate", true},
		{"normal error", "Throttling", false},
		{"", "test", false},
		{"test", "", true},
	}

	for _, tc := range tests {
		result := contains(tc.s, tc.substr)
		if result != tc.expected {
			t.Errorf("contains(%q, %q) = %v, expected %v", tc.s, tc.substr, result, tc.expected)
		}
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"throttling", &mockError{msg: "Throttling exception"}, true},
		{"rate exceeded", &mockError{msg: "RateExceeded"}, true},
		{"limit exceeded", &mockError{msg: "LimitExceededException"}, true},
		{"request limit", &mockError{msg: "RequestLimitExceeded"}, true},
		{"normal error", &mockError{msg: "some other error"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isRetryableError(tc.err)
			if result != tc.expected {
				t.Errorf("isRetryableError(%v) = %v, expected %v", tc.err, result, tc.expected)
			}
		})
	}
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
