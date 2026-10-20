package pricing

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func projectedRequest() *pbc.GetProjectedCostRequest {
	return &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:service",
		Id:           "Amazon Simple Storage Service",
		Sku:          "standard",
		Region:       "us-east-1",
	}}
}

func TestProjectedCostPassesAWSForecast(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostForecastFunc: func(context.Context, *costexplorer.GetCostForecastInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
			return &costexplorer.GetCostForecastOutput{
				Total: &types.MetricValue{Amount: aws.String("12.5"), Unit: aws.String("USD")},
				ForecastResultsByTime: []types.ForecastResult{
					{
						TimePeriod:                   &types.DateInterval{Start: aws.String("2026-10-20"), End: aws.String("2026-10-21")},
						MeanValue:                    aws.String("12.5"),
						PredictionIntervalLowerBound: aws.String("10"),
						PredictionIntervalUpperBound: aws.String("15"),
					},
				},
			}, nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	resp, err := calc.GetProjectedCost(context.Background(), projectedRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCostPerMonth() != 12.5 || resp.GetCurrency() != "USD" {
		t.Fatalf("cost=%v %s", resp.GetCostPerMonth(), resp.GetCurrency())
	}
	if resp.PredictionIntervalLower == nil || *resp.PredictionIntervalLower != 10 {
		t.Fatalf("lower=%v", resp.PredictionIntervalLower)
	}
	if resp.PredictionIntervalUpper == nil || *resp.PredictionIntervalUpper != 15 {
		t.Fatalf("upper=%v", resp.PredictionIntervalUpper)
	}
	if resp.ConfidenceLevel == nil || *resp.ConfidenceLevel != 80 {
		t.Fatalf("confidence=%v", resp.ConfidenceLevel)
	}
	if len(mockAPI.forecastCalls) != 1 {
		t.Fatalf("forecast calls=%d", len(mockAPI.forecastCalls))
	}
}

func TestProjectedCostNoData(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostForecastFunc: func(context.Context, *costexplorer.GetCostForecastInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
			return &costexplorer.GetCostForecastOutput{}, nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	_, err := calc.GetProjectedCost(context.Background(), projectedRequest())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
}

func TestProjectedCostAWSErrors(t *testing.T) {
	cases := []struct {
		name string
		code string
		want codes.Code
	}{
		{name: "bad period", code: "ValidationException", want: codes.InvalidArgument},
		{name: "limit", code: "LimitExceededException", want: codes.ResourceExhausted},
		{name: "denied", code: "AccessDenied", want: codes.PermissionDenied},
		{name: "expired", code: "ExpiredToken", want: codes.Unauthenticated},
		{name: "unavailable", code: "DataUnavailableException", want: codes.NotFound},
		{name: "unknown", code: "ServiceException", want: codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := &mockCostExplorerAPI{
				GetCostForecastFunc: func(context.Context, *costexplorer.GetCostForecastInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
					return nil, &leakyAPIError{code: tc.code, body: "secret forecast body"}
				},
			}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
			_, err := calc.GetProjectedCost(context.Background(), projectedRequest())
			if status.Code(err) != tc.want {
				t.Fatalf("got %v, want %s", err, tc.want)
			}
			if tc.code == "ValidationException" && status.Convert(err).Message() == "" {
				t.Fatal("bad period error has an empty message")
			}
		})
	}
}
