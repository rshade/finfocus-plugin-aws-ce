package pricing

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

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
		assertInvalidArgument(t, err, "EC2")
		assertNoCostExplorerCall(t, mockAPI)
	})

	t.Run("s3_arn", func(t *testing.T) {
		assertResourceLevelUnavailable(t, "contract-resource_id:s3", "arn:aws:s3:::example-bucket", start, end)
	})

	t.Run("rds_arn", func(t *testing.T) {
		assertResourceLevelUnavailable(t, "contract-resource_id:rds", "arn:aws:rds:us-east-1:123456789012:db:example-db", start, end)
	})

	t.Run("instance_id_with_s3_arn", func(t *testing.T) {
		assertResourceLevelUnavailable(t, bareInstanceID, "arn:aws:s3:::example-bucket", start, end)
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

func assertResourceLevelUnavailable(t *testing.T, resourceID, arn string, start, end time.Time) {
	t.Helper()
	calc, mockAPI := newClassifiedCalc()
	_, err := calc.GetActualCost(context.Background(), costRequest(resourceID, arn, start, end, nil))
	assertInvalidArgument(t, err, "resource-level cost is not available")
	assertNoCostExplorerCall(t, mockAPI)
	if strings.Contains(err.Error(), arn) {
		t.Fatalf("error echoes the ARN: %v", err)
	}
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
