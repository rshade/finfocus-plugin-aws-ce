package pricing

import (
	"bytes"
	"context"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

const explicitZoneSpotRecommendationID = "aws-spot-pricing:b11651ad1eb452997525c1d9f046fa0f9f150be860362523053dc5f9f406e99c"

func regionalSpotTarget(t *testing.T) *pbc.ResourceDescriptor {
	t.Helper()
	attrs, err := structpb.NewStruct(map[string]any{"instanceType": "m5.large"})
	require.NoError(t, err)
	return &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: pulumiEC2InstanceType,
		Id:           "i-0abc",
		Region:       "us-east-1",
		Attributes:   attrs,
	}
}

func regionalSpotRequest(t *testing.T) *pbc.GetProjectedCostRequest {
	t.Helper()
	target := regionalSpotTarget(t)
	target.Sku = "m5.large"
	return &pbc.GetProjectedCostRequest{Resource: target}
}

func regionalSpotCalculator(spot *fakeSpotHourly) *Calculator {
	calc := NewCalculator()
	calc.spotComparison = true
	calc.spotQuote = spot
	calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
	calc.spotRisk = fakeRisk{value: spotInterruption{Label: "<5%", Score: 0.05, Known: true}}
	return calc
}

func buildRegionalSpotRecommendation(t *testing.T, quote spotQuote) (*pbc.Recommendation, error) {
	t.Helper()
	target := regionalSpotTarget(t)
	query, err := parseSpotQuery(target)
	require.NoError(t, err)
	require.True(t, query.regional())
	return buildSpotRecommendation(target, query, quote, pluginsdk.HoursProd)
}

func TestSpotComparisonRegionLevelQuoteIsLabeled(t *testing.T) {
	calc := regionalSpotCalculator(&fakeSpotHourly{hourly: 0.04, zones: 3})
	resp, err := calc.GetProjectedCost(context.Background(), regionalSpotRequest(t))
	require.NoError(t, err)
	require.NoError(t, pluginsdk.ValidateGetProjectedCostResponse(resp))
	require.InDelta(t, 0.04, resp.GetUnitPrice(), 1e-12)
	require.InDelta(t, 0.04*730, resp.GetCostPerMonth(), 1e-9)
	require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC, resp.GetPricingCategory())
	require.Contains(t, resp.GetBillingDetail(), "us-east-1 (median of 3 zones)")
	require.Equal(t, "region-median", resp.GetMetadata()["spot_availability_zone"])
	require.Equal(t, "region", resp.GetMetadata()["spot_az_basis"])
	require.Equal(t, "3", resp.GetMetadata()["spot_zone_count"])
	require.Equal(t, "Linux/UNIX", resp.GetMetadata()["spot_product_description"])
}

func TestSpotComparisonExplicitZoneHasNoBasisMetadata(t *testing.T) {
	calc := NewCalculator()
	calc.spotComparison = true
	calc.spotQuote = &fakeSpotHourly{hourly: 0.03}
	calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
	calc.spotRisk = fakeRisk{}
	resp, err := calc.GetProjectedCost(context.Background(), spotRequest())
	require.NoError(t, err)
	require.Equal(t, "us-east-1a", resp.GetMetadata()["spot_availability_zone"])
	require.NotContains(t, resp.GetMetadata(), "spot_az_basis")
	require.NotContains(t, resp.GetMetadata(), "spot_zone_count")
	require.Contains(t, resp.GetBillingDetail(), "in us-east-1a")
}

func TestSpotComparisonRegionLevelMissingPriceNamesRegion(t *testing.T) {
	calc := regionalSpotCalculator(&fakeSpotHourly{err: client.ErrNoSpotPrice})
	_, err := calc.GetProjectedCost(context.Background(), regionalSpotRequest(t))
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), "in the region")
}

func TestSpotComparisonWithoutZoneOrRegionStaysDeclined(t *testing.T) {
	noRegion := regionalSpotTarget(t)
	noRegion.Region = ""
	noType := regionalSpotTarget(t)
	noType.Attributes = nil
	for name, target := range map[string]*pbc.ResourceDescriptor{"no region or zone": noRegion, "no instance type": noType} {
		t.Run(name, func(t *testing.T) {
			spots := &fakeSpotHourly{hourly: 0.03}
			calc := regionalSpotCalculator(spots)
			_, err := calc.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{Resource: target})
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Zero(t, spots.calls)
		})
	}
}

func TestSpotRecommendationRegionLevelLabels(t *testing.T) {
	rec, err := buildRegionalSpotRecommendation(t, spotQuote{hourly: 0.03, zones: 3, onDemand: 0.10, hasOnDemand: true})
	require.NoError(t, err)
	require.NoError(t, pluginsdk.ValidateRecommendation(rec))
	require.Contains(t, rec.GetDescription(), "us-east-1 (median of 3 zones)")
	require.Len(t, rec.GetReasoning(), 3)
	require.Contains(t, rec.GetReasoning()[2], "median of the latest prices in 3 zones of us-east-1")
	require.Equal(t, "region-median", rec.GetMetadata()["spot_availability_zone"])
	require.Equal(t, "region", rec.GetMetadata()["spot_az_basis"])
	require.Equal(t, "3", rec.GetMetadata()["spot_zone_count"])
	require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC, rec.GetPricingCategory())
}

func TestSpotRecommendationExplicitZoneKeepsReasoningAndID(t *testing.T) {
	rec, err := buildTestSpotRecommendation(t, spotQuote{hourly: 0.03, onDemand: 0.10, hasOnDemand: true})
	require.NoError(t, err)
	require.Len(t, rec.GetReasoning(), 2)
	require.Equal(t, "Move i-0abc (m5.large) from on-demand to Spot capacity in us-east-1a", rec.GetDescription())
	require.NotContains(t, rec.GetMetadata(), "spot_az_basis")
	require.Equal(t, explicitZoneSpotRecommendationID, rec.GetId())
}

func TestSpotRecommendationRegionalIDIgnoresPriceRiskAndZoneCount(t *testing.T) {
	first, err := buildRegionalSpotRecommendation(t, spotQuote{hourly: 0.03, zones: 3, onDemand: 0.10, hasOnDemand: true, risk: spotInterruption{Score: 0.05, Known: true}})
	require.NoError(t, err)
	moved, err := buildRegionalSpotRecommendation(t, spotQuote{hourly: 0.05, zones: 4, onDemand: 0.11, hasOnDemand: true, risk: spotInterruption{Score: 0.20, Known: true}})
	require.NoError(t, err)
	require.Equal(t, first.GetId(), moved.GetId())

	explicit, err := buildTestSpotRecommendation(t, spotQuote{hourly: 0.03, onDemand: 0.10, hasOnDemand: true})
	require.NoError(t, err)
	require.NotEqual(t, first.GetId(), explicit.GetId())

	otherRegion := regionalSpotTarget(t)
	otherRegion.Region = "eu-west-1"
	query, err := parseSpotQuery(otherRegion)
	require.NoError(t, err)
	other, err := buildSpotRecommendation(otherRegion, query, spotQuote{hourly: 0.03, zones: 3, onDemand: 0.10, hasOnDemand: true}, pluginsdk.HoursProd)
	require.NoError(t, err)
	require.NotEqual(t, first.GetId(), other.GetId())
}

func TestGetRecommendationsEmitsRegionLevelRow(t *testing.T) {
	calc := spotRecommendationCalculator(&fakeSpotHourly{hourly: 0.03, zones: 3}, &fakeOnDemand{hourly: 0.10, ok: true})
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{regionalSpotTarget(t)}})
	require.NoError(t, err)
	rows := spotRows(resp.GetRecommendations())
	require.Len(t, rows, 1)
	require.Equal(t, "region", rows[0].GetMetadata()["spot_az_basis"])
	require.InDelta(t, 51.1, rows[0].GetImpact().GetEstimatedSavings(), 1e-6)
}

func TestSpotRecommendationSkipReasonsAreLoggedAtDebug(t *testing.T) {
	calc := spotRecommendationCalculator(&fakeSpotHourly{hourly: 0.03}, &fakeOnDemand{hourly: 0.10, ok: true})
	noType := regionalSpotTarget(t)
	noType.Attributes = nil
	noRegionOrZone := regionalSpotTarget(t)
	noRegionOrZone.Region = ""
	notEC2 := regionalSpotTarget(t)
	notEC2.ResourceType = "aws:rds/instance:Instance"

	var buf bytes.Buffer
	calc.spotRecommendations(context.Background(), zerolog.New(&buf).Level(zerolog.DebugLevel), []*pbc.ResourceDescriptor{noType, noRegionOrZone, notEC2})
	logs := buf.String()
	require.Contains(t, logs, `"reason":"no instance type"`)
	require.Contains(t, logs, `"reason":"no region or availability zone"`)
	require.NotContains(t, logs, "not an EC2 instance")
	require.Contains(t, logs, `"level":"debug"`)
}
