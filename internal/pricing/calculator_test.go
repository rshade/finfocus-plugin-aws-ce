package pricing

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Mock implementation of CostExplorerAPI
type mockCostExplorerAPI struct {
	GetCostAndUsageFunc func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error)
	costCalls           []*costexplorer.GetCostAndUsageInput
	resourceCalls       []*costexplorer.GetCostAndUsageWithResourcesInput
}

func (m *mockCostExplorerAPI) GetCostAndUsage(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
	m.costCalls = append(m.costCalls, params)
	if m.GetCostAndUsageFunc != nil {
		return m.GetCostAndUsageFunc(ctx, params, optFns...)
	}
	return &costexplorer.GetCostAndUsageOutput{}, nil
}

func (m *mockCostExplorerAPI) GetCostForecast(ctx context.Context, params *costexplorer.GetCostForecastInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
	return nil, nil
}

func (m *mockCostExplorerAPI) GetCostAndUsageWithResources(ctx context.Context, params *costexplorer.GetCostAndUsageWithResourcesInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageWithResourcesOutput, error) {
	m.resourceCalls = append(m.resourceCalls, params)
	if m.GetCostAndUsageFunc == nil {
		return &costexplorer.GetCostAndUsageWithResourcesOutput{}, nil
	}
	out, err := m.GetCostAndUsageFunc(ctx, &costexplorer.GetCostAndUsageInput{}, optFns...)
	if err != nil || out == nil {
		return nil, err
	}
	return &costexplorer.GetCostAndUsageWithResourcesOutput{
		ResultsByTime:            out.ResultsByTime,
		NextPageToken:            out.NextPageToken,
		GroupDefinitions:         out.GroupDefinitions,
		DimensionValueAttributes: out.DimensionValueAttributes,
	}, nil
}

func pricedCostOutput() *costexplorer.GetCostAndUsageOutput {
	return &costexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			{
				TimePeriod: &types.DateInterval{
					Start: aws.String("2026-09-01"),
					End:   aws.String("2026-09-02"),
				},
				Groups: []types.Group{
					{
						Keys: []string{"Amazon Elastic Compute Cloud - Compute"},
						Metrics: map[string]types.MetricValue{
							"UnblendedCost": {
								Amount: aws.String("1.00"),
								Unit:   aws.String("USD"),
							},
						},
					},
				},
			},
		},
	}
}

func TestCalculator_Structure(t *testing.T) {
	c := NewCalculator()
	if c == nil {
		t.Fatal("NewCalculator returned nil")
	}
}

// T010: Add unit test for ARN field access (and usage intent)
func TestGetActualCost_WithArn(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			// In T015, we will update the filter to use ARN.
			// For now, we just ensure the call goes through.
			// Later we can inspect params.Filter to ensure ARN is used.
			return pricedCostOutput(), nil
		},
	}

	ceClient := client.NewClientWithAPI(mockAPI, "us-east-1")
	calc := NewCalculatorWithClient(ceClient)
	calc.cache = nil

	start := timestamppb.New(time.Now().Add(-24 * time.Hour))
	end := timestamppb.New(time.Now())

	req := &pbc.GetActualCostRequest{
		ResourceId: "i-1234567890abcdef0",
		Arn:        "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
		Start:      start,
		End:        end,
	}

	if req.GetArn() != "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0" {
		t.Errorf("Expected ARN to be accessible via GetArn(), got %s", req.GetArn())
	}

	_, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Errorf("GetActualCost failed with valid ARN: %v", err)
	}
	assertResourceQuery(t, mockAPI, "i-1234567890abcdef0")
}

// T011: Add backward compatibility test (empty ARN)
func TestGetActualCost_BackwardCompatibility(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}

	ceClient := client.NewClientWithAPI(mockAPI, "us-east-1")
	calc := NewCalculatorWithClient(ceClient)
	calc.cache = nil

	start := timestamppb.New(time.Now().Add(-24 * time.Hour))
	end := timestamppb.New(time.Now())

	// Request without ARN
	req := &pbc.GetActualCostRequest{
		ResourceId: "i-1234567890abcdef0",
		Start:      start,
		End:        end,
	}

	if req.GetArn() != "" {
		t.Errorf("Expected empty ARN, got %s", req.GetArn())
	}

	_, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Errorf("GetActualCost failed without ARN (backward compatibility broken): %v", err)
	}
	assertResourceQuery(t, mockAPI, "i-1234567890abcdef0")
}

func TestGetActualCost_BareInstanceID(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}
	ceClient := client.NewClientWithAPI(mockAPI, "us-east-1")
	calc := NewCalculatorWithClient(ceClient)
	calc.cache = nil
	req := &pbc.GetActualCostRequest{
		ResourceId: "i-0abc123def4567890",
		Start:      timestamppb.New(time.Now().Add(-24 * time.Hour)),
		End:        timestamppb.New(time.Now()),
	}
	if _, err := calc.GetActualCost(context.Background(), req); err != nil {
		t.Fatalf("GetActualCost bare instance id: %v", err)
	}
	assertResourceQuery(t, mockAPI, "i-0abc123def4567890")

	oldMock := &mockCostExplorerAPI{}
	oldCalc := NewCalculatorWithClient(client.NewClientWithAPI(oldMock, "us-east-1"))
	oldCalc.cache = nil
	oldReq := &pbc.GetActualCostRequest{
		ResourceId: "i-0abc123def4567890",
		Start:      timestamppb.New(time.Now().AddDate(0, 0, -40)),
		End:        timestamppb.New(time.Now().AddDate(0, 0, -39)),
	}
	_, err := oldCalc.GetActualCost(context.Background(), oldReq)
	if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "14 days") {
		t.Fatalf("bare instance id older than 14 days: %v", err)
	}
	if len(oldMock.resourceCalls) != 0 || len(oldMock.costCalls) != 0 {
		t.Fatalf("14-day rejection called AWS: resources=%d costs=%d", len(oldMock.resourceCalls), len(oldMock.costCalls))
	}
}

// T016: Add unit test for matching identifiers (no warning)
func TestGetActualCost_MatchingIdentifiers(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}

	ceClient := client.NewClientWithAPI(mockAPI, "us-east-1")
	calc := NewCalculatorWithClient(ceClient)
	calc.cache = nil

	start := timestamppb.New(time.Now().Add(-24 * time.Hour))
	end := timestamppb.New(time.Now())

	req := &pbc.GetActualCostRequest{
		ResourceId: "i-1234567890abcdef0",
		Arn:        "arn:aws:ec2:us-east-1:123456789012:instance/i-1234567890abcdef0",
		Start:      start,
		End:        end,
	}

	// Logic should rely on ARN or verify match.
	_, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Errorf("GetActualCost failed with matching identifiers: %v", err)
	}
	assertResourceQuery(t, mockAPI, "i-1234567890abcdef0")
}

// T017: Add unit test for mismatched identifiers (warning logged)
func TestGetActualCost_MismatchIdentifiers(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}

	ceClient := client.NewClientWithAPI(mockAPI, "us-east-1")
	calc := NewCalculatorWithClient(ceClient)
	calc.cache = nil

	start := timestamppb.New(time.Now().Add(-24 * time.Hour))
	end := timestamppb.New(time.Now())

	req := &pbc.GetActualCostRequest{
		ResourceId: "i-11111111111111111",
		Arn:        "arn:aws:ec2:us-east-1:123456789012:instance/i-22222222222222222",
		Start:      start,
		End:        end,
	}

	_, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Errorf("GetActualCost failed with mismatched identifiers: %v", err)
	}
	assertResourceQuery(t, mockAPI, "i-22222222222222222")
}

// T034: Test malformed ARN handling
func TestGetActualCost_MalformedArn(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}

	ceClient := client.NewClientWithAPI(mockAPI, "us-east-1")
	calc := NewCalculatorWithClient(ceClient)
	calc.cache = nil
	var logs bytes.Buffer
	calc.logger = zerolog.New(&logs)

	start := timestamppb.New(time.Now().Add(-24 * time.Hour))
	end := timestamppb.New(time.Now())

	req := &pbc.GetActualCostRequest{
		ResourceId: "i-fallback",
		Arn:        "invalid-arn-format",
		Start:      start,
		End:        end,
	}

	_, err := calc.GetActualCost(context.Background(), req)
	if err != nil {
		t.Errorf("GetActualCost failed with malformed ARN: %v", err)
	}
	if len(mockAPI.resourceCalls) != 0 {
		t.Fatalf("malformed ARN with a non-instance id called GetCostAndUsageWithResources")
	}
	if len(mockAPI.costCalls) != 1 {
		t.Fatalf("malformed ARN service query calls=%d", len(mockAPI.costCalls))
	}
	if ids := resourceIDs(mockAPI.costCalls[0].Filter); len(ids) != 0 {
		t.Fatalf("service total filtered by RESOURCE_ID %v", ids)
	}
	if strings.Contains(logs.String(), "falling back to ResourceId") {
		t.Fatalf("log claims a ResourceId fallback that does not filter: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "querying service totals") {
		t.Fatalf("log does not describe the service-total query: %s", logs.String())
	}

	instanceMock := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(ctx context.Context, params *costexplorer.GetCostAndUsageInput, optFns ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}
	instanceCalc := NewCalculatorWithClient(client.NewClientWithAPI(instanceMock, "us-east-1"))
	instanceCalc.cache = nil
	var instanceLogs bytes.Buffer
	instanceCalc.logger = zerolog.New(&instanceLogs)
	instanceReq := &pbc.GetActualCostRequest{
		ResourceId: "i-0abc123def4567890",
		Arn:        "invalid-arn-format",
		Start:      start,
		End:        end,
	}
	if _, err := instanceCalc.GetActualCost(context.Background(), instanceReq); err != nil {
		t.Fatalf("malformed ARN with instance ResourceId: %v", err)
	}
	assertResourceQuery(t, instanceMock, "i-0abc123def4567890")
	if !strings.Contains(instanceLogs.String(), "using ResourceId as the EC2 instance id") {
		t.Fatalf("log does not match the ResourceId query: %s", instanceLogs.String())
	}
}

func assertResourceQuery(t *testing.T, mockAPI *mockCostExplorerAPI, instanceID string) {
	t.Helper()
	if len(mockAPI.costCalls) != 0 {
		t.Fatalf("instance id used GetCostAndUsage %d times", len(mockAPI.costCalls))
	}
	if len(mockAPI.resourceCalls) != 1 {
		t.Fatalf("GetCostAndUsageWithResources calls=%d", len(mockAPI.resourceCalls))
	}
	ids := resourceIDs(mockAPI.resourceCalls[0].Filter)
	if len(ids) != 1 || ids[0] != instanceID {
		t.Fatalf("RESOURCE_ID=%v, want %s", ids, instanceID)
	}
	for _, id := range ids {
		if strings.HasPrefix(id, "arn:") {
			t.Fatalf("RESOURCE_ID is a full ARN: %s", id)
		}
	}
}

func resourceIDs(filter *types.Expression) []string {
	if filter == nil {
		return nil
	}
	var out []string
	var walk func(*types.Expression)
	walk = func(e *types.Expression) {
		if e == nil {
			return
		}
		if e.Dimensions != nil && e.Dimensions.Key == types.DimensionResourceId {
			out = append(out, e.Dimensions.Values...)
		}
		for i := range e.And {
			walk(&e.And[i])
		}
		for i := range e.Or {
			walk(&e.Or[i])
		}
		if e.Not != nil {
			walk(e.Not)
		}
	}
	walk(filter)
	return out
}
