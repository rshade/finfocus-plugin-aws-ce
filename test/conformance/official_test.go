package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	sdktesting "github.com/rshade/finfocus-spec/sdk/go/testing"
	"google.golang.org/grpc/metadata"
)

// suiteProxy adapts the SDK's server entry point to a real subprocess client.
// It forwards responses and errors unchanged; no pricing response is synthesized.
type suiteProxy struct {
	pbc.UnimplementedCostSourceServiceServer
	client pbc.CostSourceServiceClient
}

func (p *suiteProxy) Name(ctx context.Context, req *pbc.NameRequest) (*pbc.NameResponse, error) {
	return p.client.Name(ctx, req)
}
func (p *suiteProxy) Supports(ctx context.Context, req *pbc.SupportsRequest) (*pbc.SupportsResponse, error) {
	return p.client.Supports(ctx, req)
}
func (p *suiteProxy) GetPluginInfo(ctx context.Context, req *pbc.GetPluginInfoRequest) (*pbc.GetPluginInfoResponse, error) {
	return p.client.GetPluginInfo(ctx, req)
}
func (p *suiteProxy) GetActualCost(ctx context.Context, req *pbc.GetActualCostRequest) (*pbc.GetActualCostResponse, error) {
	// Explicit synthetic request credentials bypass persistent cost caches.
	// The request payload and the plugin response are not changed.
	ctx = metadata.AppendToOutgoingContext(ctx,
		pluginsdk.CredentialMetadataPrefix+"access_key_id", "test",
		pluginsdk.CredentialMetadataPrefix+"secret_access_key", "test")
	return p.client.GetActualCost(ctx, req)
}
func (p *suiteProxy) GetProjectedCost(ctx context.Context, req *pbc.GetProjectedCostRequest) (*pbc.GetProjectedCostResponse, error) {
	return p.client.GetProjectedCost(ctx, req)
}
func (p *suiteProxy) GetPricingSpec(ctx context.Context, req *pbc.GetPricingSpecRequest) (*pbc.GetPricingSpecResponse, error) {
	return p.client.GetPricingSpec(ctx, req)
}
func (p *suiteProxy) GetBudgets(ctx context.Context, req *pbc.GetBudgetsRequest) (*pbc.GetBudgetsResponse, error) {
	return p.client.GetBudgets(ctx, req)
}
func (p *suiteProxy) GetRecommendations(ctx context.Context, req *pbc.GetRecommendationsRequest) (*pbc.GetRecommendationsResponse, error) {
	return p.client.GetRecommendations(ctx, req)
}

func (p *suiteProxy) GetRecommendations(ctx context.Context, req *pbc.GetRecommendationsRequest) (*pbc.GetRecommendationsResponse, error) {
	return p.client.GetRecommendations(ctx, req)
}

func TestOfficialStandardConformance(t *testing.T) {
	const resourceID = "i-0123456789abcdef0"
	var costs, reservations, savings, forecasts, budgets, recommendations atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read AWS request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var request struct {
			Dimension   string
			Granularity string
			TimePeriod  struct{ Start, End string }
			Filter      json.RawMessage
		}
		target := r.Header.Get("X-Amz-Target")
		switch target {
		case "AWSInsightsIndexService.GetCostForecast":
			_, _ = w.Write([]byte(`{"Total":{"Amount":"1.25","Unit":"USD"},"ForecastResultsByTime":[{"TimePeriod":{"Start":"2026-10-20","End":"2026-10-21"},"MeanValue":"1.25","PredictionIntervalLowerBound":"1","PredictionIntervalUpperBound":"2"}]}`))
			return
		case "AWSInsightsIndexService.GetAnomalies":
			_, _ = w.Write([]byte(`{"Anomalies":[]}`))
			return
		case "AWSBudgetServiceGateway.DescribeBudgets":
			_, _ = w.Write([]byte(`{"Budgets":[]}`))
			return
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("decode Cost Explorer request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch r.Header.Get("X-Amz-Target") {
		case "AWSInsightsIndexService.GetCostForecast":
			forecasts.Add(1)
			_, _ = fmt.Fprintf(w, `{"Total":{"Amount":"30","Unit":"USD"},"ForecastResultsByTime":[{"TimePeriod":{"Start":%q,"End":%q},"MeanValue":"30","PredictionIntervalLowerBound":"20","PredictionIntervalUpperBound":"40"}]}`, request.TimePeriod.Start, request.TimePeriod.End)
		case "AWSBudgetServiceGateway.DescribeBudgets":
			budgets.Add(1)
			_, _ = w.Write([]byte(`{"Budgets":[{"BudgetName":"offline","BudgetType":"COST","TimeUnit":"MONTHLY","BudgetLimit":{"Amount":"100","Unit":"USD"},"CalculatedSpend":{"ActualSpend":{"Amount":"10","Unit":"USD"},"ForecastedSpend":{"Amount":"30","Unit":"USD"}}}]}`))
		case "AWSInsightsIndexService.GetRightsizingRecommendation":
			recommendations.Add(1)
			_, _ = w.Write([]byte(`{"RightsizingRecommendations":[{"AccountId":"123456789012","RightsizingType":"TERMINATE","CurrentInstance":{"ResourceId":"i-0123456789abcdef0"},"TerminateRecommendationDetail":{"EstimatedMonthlySavings":"12.50","CurrencyCode":"USD"}}]}`))
		case "AWSInsightsIndexService.GetSavingsPlansPurchaseRecommendation":
			recommendations.Add(1)
			_, _ = w.Write([]byte(`{}`))
		case "AWSInsightsIndexService.GetDimensionValues":
			switch request.Dimension {
			case "RESERVATION_ID":
				reservations.Add(1)
			case "SAVINGS_PLAN_ARN":
				savings.Add(1)
			default:
				t.Errorf("unexpected discovery dimension %q", request.Dimension)
			}
			_, _ = w.Write([]byte(`{"DimensionValues":[]}`))
		case "AWSInsightsIndexService.GetCostAndUsageWithResources":
			costs.Add(1)
			if request.Granularity != "DAILY" || request.TimePeriod.Start == "" || request.TimePeriod.End == "" {
				t.Errorf("invalid cost granularity or date range: %+v", request)
			}
			if !strings.Contains(string(request.Filter), resourceID) {
				t.Errorf("cost filter lacks suite resource identity: %s", request.Filter)
			}
			_, _ = fmt.Fprintf(w, `{"ResultsByTime":[{"TimePeriod":{"Start":%q,"End":%q},"Groups":[{"Keys":[%q],"Metrics":{"UnblendedCost":{"Amount":"1.75","Unit":"USD"}}}]}]}`,
				request.TimePeriod.Start, request.TimePeriod.End, resourceID)
		default:
			t.Errorf("unexpected AWS operation %q", r.Header.Get("X-Amz-Target"))
			http.Error(w, "unexpected operation", http.StatusBadRequest)
		}
	}))
	defer fake.Close()

	process := testutil.StartPlugin(t, conformanceEnvironment(t, fake.URL), "--port", "0")
	// The pinned Standard suite does not exercise recommendations yet.
	// Explicitly verify the newly advertised RPC through the real subprocess.
	recs, recErr := process.Client.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	if recErr != nil || len(recs.GetRecommendations()) != 1 || recs.GetRecommendations()[0].GetImpact().GetEstimatedSavings() != 12.5 {
		t.Fatalf("offline recommendation RPC: response=%v error=%v", recs, recErr)
	}
	result, err := sdktesting.RunConformance(&suiteProxy{client: process.Client},
		sdktesting.ConformanceLevelStandard,
		sdktesting.WithSampleResource(&pbc.ResourceDescriptor{
			Provider: "aws", ResourceType: "ec2", Sku: "t3.micro", Region: "us-east-1", Id: resourceID,
		}))
	if err != nil {
		t.Fatalf("run official conformance: %v", err)
	}
	report, err := result.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("official SDK conformance report:\n%s", report)
	if !result.Passed() {
		for _, category := range result.Categories {
			for _, check := range category.Results {
				if !check.Success {
					t.Logf("%s/%s: %v (%s)", category.Name, check.Method, check.Error, check.Details)
				}
			}
		}
		t.Fatalf("official Standard conformance failed: %d checks", result.Summary.Failed)
	}
	if result.LevelAchieved != sdktesting.ConformanceLevelStandard {
		t.Fatalf("conformance level: %s", result.LevelAchieved)
	}
	// The official suite allows unavailable data. Prove this run actually
	// exercised positive offline AWS responses, not merely no-data statuses.
	if forecasts.Load() == 0 || budgets.Load() == 0 || recommendations.Load() == 0 {
		t.Fatalf("advertised RPCs did not reach offline AWS: forecasts=%d budgets=%d recommendations=%d", forecasts.Load(), budgets.Load(), recommendations.Load())
	}
	if costs.Load() == 0 || reservations.Load() == 0 || savings.Load() == 0 {
		t.Fatalf("missing actual-cost queries: costs=%d reservations=%d savings=%d", costs.Load(), reservations.Load(), savings.Load())
	}
}

func conformanceEnvironment(t *testing.T, endpoint string) map[string]string {
	t.Helper()
	env := make(map[string]string)
	// StartPlugin overlays the parent environment. Clear inherited AWS and
	// plugin settings before adding the deterministic offline configuration.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "AWS_") || strings.HasPrefix(key, "FINFOCUS_") || strings.HasPrefix(key, "PULUMI_") {
			env[key] = ""
		}
	}
	emptyConfig := filepath.Join(t.TempDir(), "aws-config")
	if err := os.WriteFile(emptyConfig, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"AWS_ENDPOINT_URL_BUDGETS": endpoint, "FINFOCUS_AWS_ACCOUNT_ID": "123456789012",
		"AWS_ENDPOINT_URL_COST_EXPLORER": endpoint, "AWS_ENDPOINT_URL": endpoint,
		"AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "false", "AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "us-east-1",
		"AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_EC2_METADATA_DISABLED": "true",
		"AWS_CONFIG_FILE": emptyConfig, "AWS_SHARED_CREDENTIALS_FILE": emptyConfig, "AWS_MAX_ATTEMPTS": "1",
		"FINFOCUS_LOG_FILE": "", "FINFOCUS_PLUGIN_PORT": "0", "FINFOCUS_TEST_MODE": "true",
		"FINFOCUS_AWS_ACCOUNT_ID": "123456789012",
	} {
		env[key] = value
	}
	return env
}
