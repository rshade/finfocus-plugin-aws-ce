package pricing

import (
	"context"
	"testing"
	"time"

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

func TestProjectedCostDeclinesResourceLevelRequests(t *testing.T) {
	cases := []struct {
		name         string
		resourceType string
		id           string
	}{
		{name: "ec2 instance", resourceType: "aws:ec2/instance:Instance", id: "i-0123456789abcdef0"},
		{name: "nat gateway", resourceType: "aws:ec2/natGateway:NatGateway", id: "nat-0123456789abcdef0"},
		{name: "service without id", resourceType: "aws:service"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockAPI := &mockCostExplorerAPI{}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
			req := &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
				Provider:     "aws",
				ResourceType: tc.resourceType,
				Id:           tc.id,
				Sku:          "t3.micro",
				Region:       "us-west-2",
			}}
			_, err := calc.GetProjectedCost(context.Background(), req)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("got %v, want InvalidArgument", err)
			}
			if len(mockAPI.forecastCalls) != 0 {
				t.Fatalf("forecast calls=%d, want 0: a service or account total must not stand in for one resource", len(mockAPI.forecastCalls))
			}
		})
	}
}

func TestProjectedCostAccountScopeIsUnfiltered(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostForecastFunc: func(context.Context, *costexplorer.GetCostForecastInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
			return &costexplorer.GetCostForecastOutput{
				ForecastResultsByTime: []types.ForecastResult{
					{TimePeriod: &types.DateInterval{Start: aws.String("2026-10-20"), End: aws.String("2026-10-21")}, MeanValue: aws.String("30")},
				},
			}, nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	req := &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:account",
		Sku:          "total",
		Region:       "us-east-1",
	}}
	resp, err := calc.GetProjectedCost(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCostPerMonth() != 30 {
		t.Fatalf("cost=%v", resp.GetCostPerMonth())
	}
	if len(mockAPI.forecastCalls) != 1 || mockAPI.forecastCalls[0].Filter != nil {
		t.Fatalf("calls=%d, want one unfiltered call", len(mockAPI.forecastCalls))
	}
}

func TestProjectedCostServiceScopeFiltersByService(t *testing.T) {
	mockAPI := &mockCostExplorerAPI{
		GetCostForecastFunc: func(context.Context, *costexplorer.GetCostForecastInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
			return &costexplorer.GetCostForecastOutput{
				ForecastResultsByTime: []types.ForecastResult{
					{TimePeriod: &types.DateInterval{Start: aws.String("2026-10-20"), End: aws.String("2026-10-21")}, MeanValue: aws.String("4")},
				},
			}, nil
		},
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	if _, err := calc.GetProjectedCost(context.Background(), projectedRequest()); err != nil {
		t.Fatal(err)
	}
	if len(mockAPI.forecastCalls) != 1 {
		t.Fatalf("forecast calls=%d, want 1", len(mockAPI.forecastCalls))
	}
	dims := mockAPI.forecastCalls[0].Filter.Dimensions
	if dims == nil || dims.Key != types.DimensionService || len(dims.Values) != 1 || dims.Values[0] != "Amazon Simple Storage Service" {
		t.Fatalf("filter=%+v, want SERVICE = Amazon Simple Storage Service", dims)
	}
}

func TestProjectedCostDeclineCarriesInvalidResourceDetail(t *testing.T) {
	calc := NewCalculatorWithClient(client.NewClientWithAPI(&mockCostExplorerAPI{}, "us-east-1"))
	req := &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:ec2/instance:Instance",
		Id:           "i-0123456789abcdef0",
		Sku:          "t3.micro",
		Region:       "us-west-2",
	}}
	_, err := calc.GetProjectedCost(context.Background(), req)
	got := errorDetailList(err)
	if len(got) != 1 || got[0] != pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE {
		t.Fatalf("details=%v, want [ERROR_CODE_INVALID_RESOURCE]", got)
	}
}

func TestProjectionWindowSplitsCalendarMonthAtToday(t *testing.T) {
	cases := []struct {
		name  string
		now   time.Time
		start string
		today string
		end   string
	}{
		{"mid month", time.Date(2026, 10, 20, 15, 30, 0, 0, time.UTC), "2026-10-01", "2026-10-20", "2026-11-01"},
		{"first day", time.Date(2026, 10, 1, 0, 5, 0, 0, time.UTC), "2026-10-01", "2026-10-01", "2026-11-01"},
		{"last day", time.Date(2026, 10, 31, 23, 59, 0, 0, time.UTC), "2026-10-01", "2026-10-31", "2026-11-01"},
		{"december rolls the year", time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), "2026-12-01", "2026-12-15", "2027-01-01"},
		{"leap february", time.Date(2028, 2, 10, 0, 0, 0, 0, time.UTC), "2028-02-01", "2028-02-10", "2028-03-01"},
		{"non-UTC clock is read in UTC", time.Date(2026, 10, 31, 22, 0, 0, 0, time.FixedZone("west", -5*3600)), "2026-11-01", "2026-11-01", "2026-12-01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, today, end := projectionWindow(tc.now)
			got := [3]string{start.Format("2006-01-02"), today.Format("2006-01-02"), end.Format("2006-01-02")}
			if got != [3]string{tc.start, tc.today, tc.end} {
				t.Fatalf("got %v, want %v", got, [3]string{tc.start, tc.today, tc.end})
			}
		})
	}
}

func monthEndMock(forecastMean string, monthToDate string) *mockCostExplorerAPI {
	return &mockCostExplorerAPI{
		GetCostAndUsageFunc: func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
			return &costexplorer.GetCostAndUsageOutput{ResultsByTime: []types.ResultByTime{{
				Total: map[string]types.MetricValue{"AmortizedCost": {Amount: aws.String(monthToDate), Unit: aws.String("USD")}},
			}}}, nil
		},
		GetCostForecastFunc: func(context.Context, *costexplorer.GetCostForecastInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostForecastOutput, error) {
			return &costexplorer.GetCostForecastOutput{
				Total: &types.MetricValue{Amount: aws.String(forecastMean), Unit: aws.String("USD")},
				ForecastResultsByTime: []types.ForecastResult{{
					TimePeriod:                   &types.DateInterval{Start: aws.String("2026-10-20"), End: aws.String("2026-11-01")},
					MeanValue:                    aws.String(forecastMean),
					PredictionIntervalLowerBound: aws.String("70"),
					PredictionIntervalUpperBound: aws.String("100"),
				}},
			}, nil
		},
	}
}

func accountProjectedRequest() *pbc.GetProjectedCostRequest {
	return &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:account",
		Sku:          "total",
		Region:       "us-east-1",
	}}
}

func TestProjectedCostIsMonthToDatePlusRestOfMonthForecast(t *testing.T) {
	mockAPI := monthEndMock("86.5", "19.25")
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.now = func() time.Time { return time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC) }

	resp, err := calc.GetProjectedCost(context.Background(), accountProjectedRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCostPerMonth() != 105.75 || resp.GetCurrency() != "USD" {
		t.Fatalf("cost=%v %s, want 105.75 USD (19.25 month to date + 86.5 forecast)", resp.GetCostPerMonth(), resp.GetCurrency())
	}
	if resp.PredictionIntervalLower == nil || *resp.PredictionIntervalLower != 89.25 ||
		resp.PredictionIntervalUpper == nil || *resp.PredictionIntervalUpper != 119.25 {
		t.Fatalf("interval=%v..%v, want 89.25..119.25", resp.PredictionIntervalLower, resp.PredictionIntervalUpper)
	}

	if len(mockAPI.costCalls) != 1 || len(mockAPI.forecastCalls) != 1 {
		t.Fatalf("calls: usage=%d forecast=%d, want 1 and 1", len(mockAPI.costCalls), len(mockAPI.forecastCalls))
	}
	usage := mockAPI.costCalls[0]
	if aws.ToString(usage.TimePeriod.Start) != "2026-10-01" || aws.ToString(usage.TimePeriod.End) != "2026-10-20" {
		t.Fatalf("month to date period=%v..%v, want 2026-10-01..2026-10-20", aws.ToString(usage.TimePeriod.Start), aws.ToString(usage.TimePeriod.End))
	}
	if len(usage.Metrics) != 1 || usage.Metrics[0] != "AmortizedCost" || usage.Granularity != types.GranularityMonthly {
		t.Fatalf("usage metrics=%v granularity=%s", usage.Metrics, usage.Granularity)
	}
	forecast := mockAPI.forecastCalls[0]
	if aws.ToString(forecast.TimePeriod.Start) != "2026-10-20" || aws.ToString(forecast.TimePeriod.End) != "2026-11-01" {
		t.Fatalf("forecast period=%v..%v, want 2026-10-20..2026-11-01", aws.ToString(forecast.TimePeriod.Start), aws.ToString(forecast.TimePeriod.End))
	}
	if forecast.Metric != types.MetricAmortizedCost || forecast.Granularity != types.GranularityMonthly {
		t.Fatalf("forecast metric=%s granularity=%s, want AMORTIZED_COST MONTHLY", forecast.Metric, forecast.Granularity)
	}
}

func TestProjectedCostOnFirstOfMonthSkipsMonthToDate(t *testing.T) {
	mockAPI := monthEndMock("40", "99")
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.now = func() time.Time { return time.Date(2026, 10, 1, 0, 30, 0, 0, time.UTC) }

	resp, err := calc.GetProjectedCost(context.Background(), accountProjectedRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCostPerMonth() != 40 {
		t.Fatalf("cost=%v, want the forecast alone", resp.GetCostPerMonth())
	}
	if len(mockAPI.costCalls) != 0 {
		t.Fatalf("usage calls=%d, want 0: no day has closed yet", len(mockAPI.costCalls))
	}
}

func TestProjectedCostMonthToDateFailureIsMapped(t *testing.T) {
	mockAPI := monthEndMock("10", "1")
	mockAPI.GetCostAndUsageFunc = func(context.Context, *costexplorer.GetCostAndUsageInput, ...func(*costexplorer.Options)) (*costexplorer.GetCostAndUsageOutput, error) {
		return nil, &leakyAPIError{code: "AccessDenied", body: "secret usage body"}
	}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(mockAPI, "us-east-1"))
	calc.now = func() time.Time { return time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC) }

	_, err := calc.GetProjectedCost(context.Background(), accountProjectedRequest())
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("got %v, want PermissionDenied", err)
	}
	if len(mockAPI.forecastCalls) != 0 {
		t.Fatalf("forecast calls=%d, want 0 after the month-to-date read failed", len(mockAPI.forecastCalls))
	}
}
