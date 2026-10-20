package pricing

import (
	"context"
	"reflect"
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

const bareInstanceID = "i-0abc123def4567890"

const resourceQueryEC2ARN = "arn:aws:ec2:us-east-1:123456789012:instance/" + bareInstanceID

func TestPerResourceCostClassification(t *testing.T) {
	start, end := recentCostWindow()
	oldEnd := time.Now().UTC().AddDate(0, 0, -40)
	oldStart := oldEnd.Add(-24 * time.Hour)

	t.Run("bare_"+bareInstanceID, func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		if _, err := calc.GetActualCost(context.Background(), costRequest(bareInstanceID, "", start, end, nil)); err != nil {
			t.Fatalf("GetActualCost: %v", err)
		}
		assertResourceQuery(t, mockAPI, bareInstanceID)
	})

	t.Run("ec2_arn", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		req := costRequest("contract-not-the-instance", resourceQueryEC2ARN, start, end, nil)
		if _, err := calc.GetActualCost(context.Background(), req); err != nil {
			t.Fatalf("GetActualCost: %v", err)
		}
		assertResourceQuery(t, mockAPI, bareInstanceID)
		if got := strings.Join(resourceIDs(mockAPI.resourceCalls[0].Filter), ","); strings.Contains(got, resourceQueryEC2ARN) {
			t.Fatalf("RESOURCE_ID contains the full ARN: %s", got)
		}
	})

	t.Run("contract_id", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		if _, err := calc.GetActualCost(context.Background(), costRequest("contract-official_gcu_request", "", start, end, nil)); err != nil {
			t.Fatalf("GetActualCost: %v", err)
		}
		assertServiceTotalQuery(t, mockAPI)
	})

	t.Run("other_non_instance_id", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		_, err := calc.GetActualCost(context.Background(), costRequest("example-bucket", "", start, end, nil))
		assertInvalidArgument(t, err, "ARN")
		assertNoCostExplorerCall(t, mockAPI)
	})

	t.Run("s3_arn", func(t *testing.T) {
		assertNonEC2ResourceQuery(t, "contract-resource_id:s3", "arn:aws:s3:::example-bucket", "Amazon Simple Storage Service", "arn:aws:s3:::example-bucket", "example-bucket")
	})

	t.Run("rds_arn", func(t *testing.T) {
		assertNonEC2ResourceQuery(t, "contract-resource_id:rds", "arn:aws:rds:us-east-1:123456789012:db:example-db", "Amazon Relational Database Service", "arn:aws:rds:us-east-1:123456789012:db:example-db", "example-db")
	})

	t.Run("lambda_arn", func(t *testing.T) {
		assertNonEC2ResourceQuery(t, "contract-resource_id:lambda", "arn:aws:lambda:us-east-1:123456789012:function:billing-report", "AWS Lambda", "arn:aws:lambda:us-east-1:123456789012:function:billing-report", "billing-report")
	})

	t.Run("instance_id_with_s3_arn", func(t *testing.T) {
		assertNonEC2ResourceQuery(t, bareInstanceID, "arn:aws:s3:::example-bucket", "Amazon Simple Storage Service", "arn:aws:s3:::example-bucket", "example-bucket")
	})

	t.Run("s3_no_rows", func(t *testing.T) {
		calc, mockAPI := newEmptyResourceCalc()
		_, err := calc.GetActualCost(context.Background(), costRequest("example-bucket", "arn:aws:s3:::example-bucket", start, end, nil))
		if status.Code(err) != codes.NotFound {
			t.Fatalf("got %v, want NotFound", err)
		}
		if len(mockAPI.resourceCalls) == 0 {
			t.Fatal("no-row case did not query Cost Explorer")
		}
	})

	t.Run("s3_older_than_14_days", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		_, err := calc.GetActualCost(context.Background(), costRequest("example-bucket", "arn:aws:s3:::example-bucket", oldStart, oldEnd, nil))
		assertInvalidArgument(t, err, "14 days")
		assertNoCostExplorerCall(t, mockAPI)
	})

	t.Run("older_than_14_days", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		_, err := calc.GetActualCost(context.Background(), costRequest(bareInstanceID, resourceQueryEC2ARN, oldStart, oldEnd, nil))
		assertInvalidArgument(t, err, "14 days")
		assertNoCostExplorerCall(t, mockAPI)
	})

	t.Run("malformed_arn_empty_resource_id", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		req := costRequest("", "not-an-arn", start, end, nil)
		_, err := calc.GetActualCost(context.Background(), req)
		assertInvalidArgument(t, err, "resource_id is required")
		assertNoCostExplorerCall(t, mockAPI)

		_, err = calc.planCostQuery(zerolog.Nop(), req)
		assertInvalidArgument(t, err, "malformed ARN")
	})

	t.Run("malformed_arn_keeps_instance_plan", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		if _, err := calc.GetActualCost(context.Background(), costRequest(bareInstanceID, "not-an-arn", start, end, nil)); err != nil {
			t.Fatalf("GetActualCost: %v", err)
		}
		assertResourceQuery(t, mockAPI, bareInstanceID)
	})

	t.Run("contract_id_outside_14_days", func(t *testing.T) {
		calc, mockAPI := newClassifiedCalc()
		if _, err := calc.GetActualCost(context.Background(), costRequest("contract-official_gcu_request", "", oldStart, oldEnd, nil)); err != nil {
			t.Fatalf("service total inside 14 months: %v", err)
		}
		assertServiceTotalQuery(t, mockAPI)
	})
}

func TestGetActualCostTagsDoNotChangeFilter(t *testing.T) {
	start, end := recentCostWindow()
	tags := map[string]string{"Environment": "prod", "CostCenter": "finops"}
	cases := []struct {
		name       string
		resourceID string
		arn        string
	}{
		{name: "service_total", resourceID: "contract-official_gcu_request"},
		{name: "bare_instance", resourceID: bareInstanceID},
		{name: "ec2_arn", resourceID: bareInstanceID, arn: resourceQueryEC2ARN},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plain := costExplorerFilter(t, costRequest(tc.resourceID, tc.arn, start, end, nil))
			tagged := costExplorerFilter(t, costRequest(tc.resourceID, tc.arn, start, end, tags))
			if !reflect.DeepEqual(plain, tagged) {
				t.Fatalf("tags changed the Cost Explorer filter\nplain=%#v\ntagged=%#v", plain, tagged)
			}
			if expressionUsesTags(plain) || expressionUsesTags(tagged) {
				t.Fatalf("Cost Explorer filter uses tags: %#v", tagged)
			}
		})
	}
}

func newClassifiedCalc() (*Calculator, *mockCostExplorerAPI) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return pricedCostOutput(), nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	return calc, mockAPI
}

func costRequest(resourceID, arn string, start, end time.Time, tags map[string]string) *pbc.GetActualCostRequest {
	req := &pbc.GetActualCostRequest{
		ResourceId: resourceID,
		Arn:        arn,
		Start:      timestamppb.New(start),
		End:        timestamppb.New(end),
		Tags:       tags,
	}
	if arn == "" && (strings.HasPrefix(resourceID, "contract-") || resourceID == "AmazonS3") {
		req.Resource = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:account", Id: resourceID}
	}
	return req
}

func recentCostWindow() (time.Time, time.Time) {
	end := time.Now().UTC()
	return end.Add(-24 * time.Hour), end
}

func assertServiceTotalQuery(t *testing.T, mockAPI *mockCostExplorerAPI) {
	t.Helper()
	if len(mockAPI.resourceCalls) != 0 {
		t.Fatalf("service total used GetCostAndUsageWithResources %d times", len(mockAPI.resourceCalls))
	}
	if len(mockAPI.costCalls) != 1 {
		t.Fatalf("GetCostAndUsage calls=%d", len(mockAPI.costCalls))
	}
	call := mockAPI.costCalls[0]
	if call.Filter != nil {
		t.Fatalf("service total filter=%#v", call.Filter)
	}
	if len(call.GroupBy) != 1 || call.GroupBy[0].Key == nil || *call.GroupBy[0].Key != "SERVICE" {
		t.Fatalf("GroupBy=%v, want SERVICE", call.GroupBy)
	}
	if call.GroupBy[0].Type != types.GroupDefinitionTypeDimension {
		t.Fatalf("GroupBy type=%s", call.GroupBy[0].Type)
	}
}

func assertNonEC2ResourceQuery(t *testing.T, resourceID, arn, service string, wantIDs ...string) {
	t.Helper()
	start, end := recentCostWindow()
	calc, mockAPI := newClassifiedCalc()
	if _, err := calc.GetActualCost(context.Background(), costRequest(resourceID, arn, start, end, nil)); err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	if len(mockAPI.resourceCalls) != 1 {
		t.Fatalf("GetCostAndUsageWithResources calls=%d", len(mockAPI.resourceCalls))
	}
	filter := mockAPI.resourceCalls[0].Filter
	got := resourceIDs(filter)
	if !sameIDs(got, wantIDs) {
		t.Fatalf("RESOURCE_ID=%v, want %v", got, wantIDs)
	}
	if !strings.Contains(strings.Join(serviceValues(filter), ","), service) {
		t.Fatalf("SERVICE=%v, want %s", serviceValues(filter), service)
	}
}

func sameIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[string]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func serviceValues(filter *types.Expression) []string {
	return dimensionValuesOf(filter, types.DimensionService)
}

func dimensionValuesOf(filter *types.Expression, key types.Dimension) []string {
	if filter == nil {
		return nil
	}
	var out []string
	var walk func(*types.Expression)
	walk = func(e *types.Expression) {
		if e == nil {
			return
		}
		if e.Dimensions != nil && e.Dimensions.Key == key {
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

func newEmptyResourceCalc() (*Calculator, *mockCostExplorerAPI) {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{ResultsByTime: []types.ResultByTime{{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-09-01"), End: aws.String("2026-09-02")},
			}}}, nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	return calc, mockAPI
}

func assertInvalidArgument(t *testing.T, err error, fragment string) {
	t.Helper()
	if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), fragment) {
		t.Fatalf("got %v, want InvalidArgument containing %q", err, fragment)
	}
}

func assertNoCostExplorerCall(t *testing.T, mockAPI *mockCostExplorerAPI) {
	t.Helper()
	if len(mockAPI.costCalls) != 0 || len(mockAPI.resourceCalls) != 0 {
		t.Fatalf("AWS calls cost=%d resource=%d", len(mockAPI.costCalls), len(mockAPI.resourceCalls))
	}
}

func costExplorerFilter(t *testing.T, req *pbc.GetActualCostRequest) *types.Expression {
	t.Helper()
	calc, mockAPI := newClassifiedCalc()
	if _, err := calc.GetActualCost(context.Background(), req); err != nil {
		t.Fatalf("GetActualCost: %v", err)
	}
	switch {
	case len(mockAPI.resourceCalls) == 1 && len(mockAPI.costCalls) == 0:
		return mockAPI.resourceCalls[0].Filter
	case len(mockAPI.costCalls) == 1 && len(mockAPI.resourceCalls) == 0:
		return mockAPI.costCalls[0].Filter
	default:
		t.Fatalf("calls cost=%d resource=%d", len(mockAPI.costCalls), len(mockAPI.resourceCalls))
		return nil
	}
}

func expressionUsesTags(e *types.Expression) bool {
	if e == nil {
		return false
	}
	if e.Tags != nil {
		return true
	}
	for i := range e.And {
		if expressionUsesTags(&e.And[i]) {
			return true
		}
	}
	for i := range e.Or {
		if expressionUsesTags(&e.Or[i]) {
			return true
		}
	}
	return expressionUsesTags(e.Not)
}
