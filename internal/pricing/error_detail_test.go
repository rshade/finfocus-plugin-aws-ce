package pricing

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetActualCost_ErrorDetails(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	start := timestamppb.New(now.Add(-24 * time.Hour))
	end := timestamppb.New(now)
	same := timestamppb.New(now)

	tests := []struct {
		name     string
		calc     func() *Calculator
		req      *pbc.GetActualCostRequest
		grpcCode codes.Code
		detail   pbc.ErrorCode
	}{
		{
			name: "invalid time range",
			calc: func() *Calculator { return NewCalculator() },
			req: &pbc.GetActualCostRequest{
				ResourceId: "AmazonS3",
				Start:      same,
				End:        same,
			},
			grpcCode: codes.InvalidArgument,
			detail:   pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE,
		},
		{
			name: "empty results",
			calc: emptyCostCalculator,
			req: &pbc.GetActualCostRequest{
				ResourceId: "AmazonS3",
				Start:      start,
				End:        end,
			},
			grpcCode: codes.NotFound,
			detail:   pbc.ErrorCode_ERROR_CODE_RESOURCE_NOT_FOUND,
		},
		{
			name: "missing resource id",
			calc: func() *Calculator { return NewCalculator() },
			req: &pbc.GetActualCostRequest{
				Start: start,
				End:   end,
			},
			grpcCode: codes.InvalidArgument,
			detail:   pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := pluginsdk.NewTestServer(t, tt.calc())
			t.Cleanup(srv.Close)

			_, err := srv.Client().GetActualCost(context.Background(), tt.req)
			if status.Code(err) != tt.grpcCode {
				t.Fatalf("status.Code = %s, want %s (%v)", status.Code(err), tt.grpcCode, err)
			}
			if got := errorDetailCode(err); got != tt.detail {
				t.Fatalf("ErrorDetail code = %s, want %s; details=%v", got, tt.detail, status.Convert(err).Details())
			}
		})
	}
}

func emptyCostCalculator() *Calculator {
	mockAPI := &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{}, nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.cache = nil
	return calc
}

func errorDetailCode(err error) pbc.ErrorCode {
	for _, detail := range status.Convert(err).Details() {
		if coded, ok := detail.(*pbc.ErrorDetail); ok {
			return coded.GetCode()
		}
	}
	return pbc.ErrorCode_ERROR_CODE_UNSPECIFIED
}
