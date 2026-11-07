package pricing

import (
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func spotRecommendationTarget(t *testing.T, zone string) *pbc.ResourceDescriptor {
	t.Helper()
	attrs, err := structpb.NewStruct(map[string]any{"availabilityZone": zone, "instanceType": "m5.large"})
	require.NoError(t, err)
	return &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: pulumiEC2InstanceType,
		Id:           "i-0abc",
		Region:       "us-east-1",
		Attributes:   attrs,
	}
}

func buildTestSpotRecommendation(t *testing.T, quote spotQuote) (*pbc.Recommendation, error) {
	t.Helper()
	target := spotRecommendationTarget(t, "us-east-1a")
	query, err := parseSpotQuery(target)
	require.NoError(t, err)
	return buildSpotRecommendation(target, query, quote, pluginsdk.HoursProd)
}

func TestSpotRecommendationKnownRisk(t *testing.T) {
	rec, err := buildTestSpotRecommendation(t, spotQuote{
		hourly: 0.03, onDemand: 0.10, hasOnDemand: true,
		risk: spotInterruption{Label: "<5%", Score: 0.05, Known: true},
	})
	require.NoError(t, err)
	require.NoError(t, pluginsdk.ValidateRecommendation(rec))
	require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC, rec.GetPricingCategory())
	require.InDelta(t, 0.05, rec.GetSpotInterruptionRiskScore(), 1e-9)
	require.Equal(t, pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_MIGRATE, rec.GetActionType())
	require.Equal(t, pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST, rec.GetCategory())
	require.Equal(t, spotRecommendationSource, rec.GetSource())
	require.Equal(t, "i-0abc", rec.GetResource().GetId())
	require.Equal(t, "ec2:Instance", rec.GetResource().GetResourceType())
	require.Equal(t, "m5.large", rec.GetResource().GetSku())
	require.Equal(t, "us-east-1", rec.GetResource().GetRegion())

	impact := rec.GetImpact()
	require.Equal(t, "USD", impact.GetCurrency())
	require.Equal(t, "monthly", impact.GetProjectionPeriod())
	require.InDelta(t, 73.0, impact.GetCurrentCost(), 1e-6)
	require.InDelta(t, 21.9, impact.GetProjectedCost(), 1e-6)
	require.InDelta(t, 51.1, impact.GetEstimatedSavings(), 1e-6)
	require.InDelta(t, 70.0, impact.GetSavingsPercentage(), 1e-6)
	require.Equal(t, "51.1", rec.GetMetadata()["monthly_savings"])
	require.Equal(t, "us-east-1a", rec.GetMetadata()["spot_availability_zone"])
	require.Equal(t, "<5%", rec.GetMetadata()["spot_interruption_label"])
}

func TestSpotRecommendationUnknownRiskStaysDynamic(t *testing.T) {
	rec, err := buildTestSpotRecommendation(t, spotQuote{hourly: 0.03, onDemand: 0.10, hasOnDemand: true})
	require.NoError(t, err)
	require.NoError(t, pluginsdk.ValidateRecommendation(rec))
	require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC, rec.GetPricingCategory())
	require.Zero(t, rec.GetSpotInterruptionRiskScore())
	require.NotContains(t, rec.GetMetadata(), "spot_interruption_label")
}

func TestSpotRecommendationOverTwentyPercentBucket(t *testing.T) {
	rec, err := buildTestSpotRecommendation(t, spotQuote{
		hourly: 0.03, onDemand: 0.10, hasOnDemand: true,
		risk: spotInterruption{Label: ">20%", Known: true},
	})
	require.NoError(t, err)
	require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC, rec.GetPricingCategory())
	require.Zero(t, rec.GetSpotInterruptionRiskScore())
	require.Equal(t, ">20%", rec.GetMetadata()["spot_interruption_label"])
}

func TestSpotRecommendationSkipsWhenSavingsCannotBeComputed(t *testing.T) {
	tests := []struct {
		name  string
		quote spotQuote
	}{
		{"no on-demand row", spotQuote{hourly: 0.03}},
		{"spot equals on-demand", spotQuote{hourly: 0.10, onDemand: 0.10, hasOnDemand: true}},
		{"spot above on-demand", spotQuote{hourly: 0.12, onDemand: 0.10, hasOnDemand: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := buildTestSpotRecommendation(t, tt.quote)
			require.Error(t, err)
			require.Nil(t, rec)
		})
	}
}

func TestSpotRecommendationIDIgnoresPricesAndRisk(t *testing.T) {
	cheap, err := buildTestSpotRecommendation(t, spotQuote{
		hourly: 0.03, onDemand: 0.10, hasOnDemand: true,
		risk: spotInterruption{Score: 0.05, Known: true},
	})
	require.NoError(t, err)
	moved, err := buildTestSpotRecommendation(t, spotQuote{
		hourly: 0.05, onDemand: 0.11, hasOnDemand: true,
		risk: spotInterruption{Score: 0.20, Known: true},
	})
	require.NoError(t, err)
	require.Equal(t, cheap.GetId(), moved.GetId())

	otherZone := spotRecommendationTarget(t, "us-east-1b")
	query, err := parseSpotQuery(otherZone)
	require.NoError(t, err)
	other, err := buildSpotRecommendation(otherZone, query, spotQuote{hourly: 0.03, onDemand: 0.10, hasOnDemand: true}, pluginsdk.HoursProd)
	require.NoError(t, err)
	require.NotEqual(t, cheap.GetId(), other.GetId())
}

func TestNonSpotRecommendationsCarryNoSpotFields(t *testing.T) {
	recs, err := mapRightsizingRecommendations(rightsizingFixture())
	require.NoError(t, err)
	require.NotEmpty(t, recs)
	for _, rec := range recs {
		require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_UNSPECIFIED, rec.GetPricingCategory())
		require.Zero(t, rec.GetSpotInterruptionRiskScore())
	}
}

func spotRecommendationCalculator(spot *fakeSpotHourly, onDemand *fakeOnDemand) *Calculator {
	calc := NewCalculatorWithClient(client.NewClientWithAPI(&recommendationAPI{}, "us-east-1"))
	calc.spotComparison = true
	calc.spotQuote = spot
	calc.onDemandQuote = onDemand
	calc.spotRisk = fakeRisk{value: spotInterruption{Label: "<5%", Score: 0.05, Known: true}}
	return calc
}

func TestGetRecommendationsEmitsSpotRowForTarget(t *testing.T) {
	calc := spotRecommendationCalculator(&fakeSpotHourly{hourly: 0.03}, &fakeOnDemand{hourly: 0.10, ok: true})
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}})
	require.NoError(t, err)
	require.Len(t, resp.GetRecommendations(), 1)
	rec := resp.GetRecommendations()[0]
	require.Equal(t, pbc.FocusPricingCategory_FOCUS_PRICING_CATEGORY_DYNAMIC, rec.GetPricingCategory())
	require.InDelta(t, 0.05, rec.GetSpotInterruptionRiskScore(), 1e-9)
	require.InDelta(t, 51.1, resp.GetSummary().GetTotalEstimatedSavings(), 1e-6)
}

func TestGetRecommendationsScalesSpotSavingsByProjectionPeriod(t *testing.T) {
	calc := spotRecommendationCalculator(&fakeSpotHourly{hourly: 0.03}, &fakeOnDemand{hourly: 0.10, ok: true})
	req := &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}}
	req.ProjectionPeriod = "annual"
	resp, err := calc.GetRecommendations(t.Context(), req)
	require.NoError(t, err)
	require.Len(t, resp.GetRecommendations(), 1)
	require.InDelta(t, 51.1*12, resp.GetRecommendations()[0].GetImpact().GetEstimatedSavings(), 1e-6)
}

func TestGetRecommendationsSpotOptInOff(t *testing.T) {
	spots := &fakeSpotHourly{hourly: 0.03}
	calc := spotRecommendationCalculator(spots, &fakeOnDemand{hourly: 0.10, ok: true})
	calc.spotComparison = false
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}})
	require.NoError(t, err)
	require.Empty(t, resp.GetRecommendations())
	require.Zero(t, spots.calls)
}

func TestGetRecommendationsSpotIneligibleTargets(t *testing.T) {
	spots := &fakeSpotHourly{hourly: 0.03}
	calc := spotRecommendationCalculator(spots, &fakeOnDemand{hourly: 0.10, ok: true})
	noZone := spotRecommendationTarget(t, "us-east-1a")
	noZone.Attributes = nil
	notEC2 := spotRecommendationTarget(t, "us-east-1a")
	notEC2.ResourceType = "aws:rds/instance:Instance"
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{noZone, notEC2}})
	require.NoError(t, err)
	require.Empty(t, resp.GetRecommendations())
	require.Zero(t, spots.calls)
}

func TestGetRecommendationsSpotFailsSoftPerTarget(t *testing.T) {
	spots := &fakeSpotHourly{err: status.Error(codes.Unavailable, "throttled")}
	calc := spotRecommendationCalculator(spots, &fakeOnDemand{hourly: 0.10, ok: true})
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}})
	require.NoError(t, err)
	require.Empty(t, resp.GetRecommendations())
	require.Equal(t, 1, spots.calls)
}

func TestGetRecommendationsSpotOptInLeavesCostExplorerRowsUnchanged(t *testing.T) {
	respond := func(optIn bool) *pbc.GetRecommendationsResponse {
		calc := NewCalculatorWithClient(client.NewClientWithAPI(&recommendationAPI{rights: rightsizingFixture()}, "us-east-1"))
		calc.spotComparison = optIn
		calc.spotQuote = &fakeSpotHourly{hourly: 0.03}
		calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
		resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{
			Filter: &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST},
		})
		require.NoError(t, err)
		return resp
	}
	baseline := respond(false)
	require.NotEmpty(t, baseline.GetRecommendations())
	require.True(t, proto.Equal(baseline, respond(true)))
}

func TestGetRecommendationsSkipsSpotCallsWhenFilterExcludesThem(t *testing.T) {
	for name, filter := range map[string]*pbc.RecommendationFilter{
		"other category": {Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_PERFORMANCE},
		"other type":       {ResourceType: "savingsplans:Account"},
		"other provider":   {Provider: "gcp"},
	} {
		t.Run(name, func(t *testing.T) {
			spots := &fakeSpotHourly{hourly: 0.03}
			calc := spotRecommendationCalculator(spots, &fakeOnDemand{hourly: 0.10, ok: true})
			_, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{
				Filter:          filter,
				TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")},
			})
			require.NoError(t, err)
			require.Zero(t, spots.calls)
		})
	}
}
