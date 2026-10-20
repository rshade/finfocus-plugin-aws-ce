package pricing

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type fakeSpotHourly struct {
	hourly       float64
	err          error
	calls        int
	instanceType string
	zone         string
	product      string
}

func (f *fakeSpotHourly) LatestSpotPrice(_ context.Context, _, instanceType, product, availabilityZone string) (float64, error) {
	f.calls++
	f.instanceType = instanceType
	f.zone = availabilityZone
	f.product = product
	return f.hourly, f.err
}

type fakeOnDemand struct {
	hourly float64
	ok     bool
	err    error
	os     string
}

func (f *fakeOnDemand) OnDemandHourly(_ context.Context, _, _, operatingSystem string) (float64, bool, error) {
	f.os = operatingSystem
	return f.hourly, f.ok, f.err
}

type fakeRisk struct {
	value spotInterruption
	err   error
}

func (f fakeRisk) Interruption(context.Context, string, string, string) (spotInterruption, error) {
	return f.value, f.err
}

func spotRequest() *pbc.GetProjectedCostRequest {
	attrs, err := structpb.NewStruct(map[string]any{
		"availabilityZone": "us-east-1a",
		"instanceType":     "m5.large",
	})
	if err != nil {
		panic(err)
	}
	return &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:ec2/instance:Instance",
		Sku:          "t3.micro",
		Region:       "us-east-1",
		Attributes:   attrs,
		Tags: map[string]string{
			"instanceType":     "c5.large",
			"availabilityZone": "us-east-1b",
		},
	}}
}

func TestSpotComparisonUsesZonePriceAndOnDemand(t *testing.T) {
	spots := &fakeSpotHourly{hourly: 0.03}
	demands := &fakeOnDemand{hourly: 0.10, ok: true}
	calc := NewCalculator()
	calc.spotQuote = spots
	calc.onDemandQuote = demands
	calc.spotRisk = fakeRisk{value: spotInterruption{Label: "5-10%", Score: 0.10, Known: true}}

	resp, err := calc.GetProjectedCost(context.Background(), spotRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginsdk.ValidateGetProjectedCostResponse(resp); err != nil {
		t.Fatal(err)
	}
	if spots.instanceType != "m5.large" || spots.zone != "us-east-1a" || spots.product != "Linux/UNIX" || demands.os != "Linux" {
		t.Fatalf("type=%s zone=%s product=%s os=%s", spots.instanceType, spots.zone, spots.product, demands.os)
	}
	if resp.GetPricingCategory() != pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC {
		t.Fatalf("category=%s", resp.GetPricingCategory())
	}
	if resp.GetUnitPrice() != 0.03 || resp.GetCostPerMonth() != 0.03*730 {
		t.Fatalf("price=%v monthly=%v", resp.GetUnitPrice(), resp.GetCostPerMonth())
	}
	if resp.OnDemandEquivalentMonthlyCost == nil || *resp.OnDemandEquivalentMonthlyCost != 0.10*730 {
		t.Fatalf("on demand=%v", resp.OnDemandEquivalentMonthlyCost)
	}
	if resp.GetSpotInterruptionRiskScore() != 0.10 {
		t.Fatalf("score=%v", resp.GetSpotInterruptionRiskScore())
	}
	options := resp.GetPriceOptions()
	if len(options) != 1 || options[0].GetCategory() != pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_STANDARD {
		t.Fatalf("options=%v", options)
	}
	wantFraction := (0.03 - 0.10) / 0.03
	if math.Abs(options[0].GetSavingsFraction()-wantFraction) > 1e-12 {
		t.Fatalf("fraction=%v", options[0].GetSavingsFraction())
	}
	if resp.GetMetadata()["spot_interruption_label"] != "5-10%" || resp.GetMetadata()["spot_availability_zone"] != "us-east-1a" {
		t.Fatalf("metadata=%v", resp.GetMetadata())
	}
	if resp.GetExpiresAt() == nil {
		t.Fatal("expires_at unset")
	}
}

func TestSpotComparisonOpenBucketLeavesScoreUnset(t *testing.T) {
	calc := NewCalculator()
	calc.spotQuote = &fakeSpotHourly{hourly: 0.03}
	calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
	calc.spotRisk = fakeRisk{value: spotInterruption{Label: ">20%", Known: true}}
	resp, err := calc.GetProjectedCost(context.Background(), spotRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetSpotInterruptionRiskScore() != 0 || resp.GetMetadata()["spot_interruption_label"] != ">20%" {
		t.Fatalf("score=%v label=%s", resp.GetSpotInterruptionRiskScore(), resp.GetMetadata()["spot_interruption_label"])
	}
	if resp.GetPricingCategory() != pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC {
		t.Fatalf("category=%s", resp.GetPricingCategory())
	}
}

func TestSpotComparisonOmitsUnknownOnDemand(t *testing.T) {
	calc := NewCalculator()
	calc.spotQuote = &fakeSpotHourly{hourly: 0.03}
	calc.onDemandQuote = &fakeOnDemand{}
	calc.spotRisk = fakeRisk{value: spotInterruption{Label: "<5%", Score: 0.05, Known: true}}
	resp, err := calc.GetProjectedCost(context.Background(), spotRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.OnDemandEquivalentMonthlyCost != nil || len(resp.GetPriceOptions()) != 0 {
		t.Fatalf("on demand=%v options=%d", resp.OnDemandEquivalentMonthlyCost, len(resp.GetPriceOptions()))
	}
	if resp.GetUnitPrice() != 0.03 {
		t.Fatalf("price=%v", resp.GetUnitPrice())
	}
}

func TestSpotComparisonAdvisorFailureStillReturnsPrice(t *testing.T) {
	calc := NewCalculator()
	calc.spotQuote = &fakeSpotHourly{hourly: 0.03}
	calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
	calc.spotRisk = fakeRisk{err: context.DeadlineExceeded}
	resp, err := calc.GetProjectedCost(context.Background(), spotRequest())
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetUnitPrice() != 0.03 || resp.OnDemandEquivalentMonthlyCost == nil {
		t.Fatalf("price=%v on demand=%v", resp.GetUnitPrice(), resp.OnDemandEquivalentMonthlyCost)
	}
	if resp.GetSpotInterruptionRiskScore() != 0 || resp.GetMetadata()["spot_interruption_label"] != "" {
		t.Fatalf("score=%v metadata=%v", resp.GetSpotInterruptionRiskScore(), resp.GetMetadata())
	}
}

func TestSpotComparisonMissingPrice(t *testing.T) {
	calc := NewCalculator()
	calc.spotQuote = &fakeSpotHourly{err: client.ErrNoSpotPrice}
	calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
	calc.spotRisk = fakeRisk{}
	_, err := calc.GetProjectedCost(context.Background(), spotRequest())
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v", err)
	}
}

func TestEC2WithoutZoneStaysDeclined(t *testing.T) {
	calc := NewCalculatorWithClient(client.NewClientWithAPI(&mockCostExplorerAPI{}, "us-east-1"))
	calc.spotQuote = &fakeSpotHourly{hourly: 1}
	req := &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{
		Provider: "aws", ResourceType: "aws:ec2/instance:Instance", Sku: "m5.large", Region: "us-east-1",
	}}
	_, err := calc.GetProjectedCost(context.Background(), req)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("got %v", err)
	}
	if calc.spotQuote.(*fakeSpotHourly).calls != 0 {
		t.Fatal("spot price was called without an availability zone")
	}
}

func TestSpotAdvisorParsesBuckets(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"ranges":[{"index":1,"label":"5-10%"},{"index":4,"label":">20%"}],"spot_advisor":{"us-east-1":{"Linux":{"m5.large":{"r":1},"c5.large":{"r":4}}}}}`))
	}))
	defer server.Close()
	advisor := newHTTPAdvisor(server.Client(), server.URL, spotAdvisorTTL)
	got, err := advisor.Interruption(context.Background(), "us-east-1", "Linux", "m5.large")
	if err != nil {
		t.Fatal(err)
	}
	if got.Score != 0.10 || got.Label != "5-10%" || !got.Known {
		t.Fatalf("closed=%+v", got)
	}
	open, err := advisor.Interruption(context.Background(), "us-east-1", "Linux", "c5.large")
	if err != nil {
		t.Fatal(err)
	}
	if open.Score != 0 || open.Label != ">20%" || !open.Known {
		t.Fatalf("open=%+v", open)
	}
	if calls != 1 {
		t.Fatalf("feed calls=%d", calls)
	}
}
