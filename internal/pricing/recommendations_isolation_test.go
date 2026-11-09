package pricing

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func spotIsolationCalculator(api *recommendationAPI) *Calculator {
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	setSpotSeams(calc)
	return calc
}

func setSpotSeams(calc *Calculator) {
	calc.spotComparison = true
	calc.spotQuote = &fakeSpotHourly{hourly: 0.03}
	calc.onDemandQuote = &fakeOnDemand{hourly: 0.10, ok: true}
	calc.spotRisk = fakeRisk{value: spotInterruption{Label: "<5%", Score: 0.05, Known: true}}
}

func spotRows(recs []*pbc.Recommendation) []*pbc.Recommendation {
	var out []*pbc.Recommendation
	for _, rec := range recs {
		if rec.GetSource() == spotRecommendationSource {
			out = append(out, rec)
		}
	}
	return out
}

func TestRecommendationsSpotSurvivesCostExplorerFailure(t *testing.T) {
	unmappable := rightsizingFixture()
	unmappable[0].RightsizingRecommendations[0].RightsizingType = types.RightsizingType("Resize")
	for _, tc := range []struct {
		name string
		api  *recommendationAPI
	}{
		{"mapping error", &recommendationAPI{rights: unmappable}},
		{"access denied", &recommendationAPI{rightFail: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "secret"}}},
		{"data unavailable", &recommendationAPI{rightFail: &smithy.GenericAPIError{Code: "DataUnavailableException"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calc := spotIsolationCalculator(tc.api)
			resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}})
			require.NoError(t, err)
			require.Len(t, spotRows(resp.GetRecommendations()), 1)
			require.Len(t, resp.GetRecommendations(), 1)
		})
	}
}

func TestRecommendationsSpotSurvivesMissingCostExplorerClient(t *testing.T) {
	calc := NewCalculator()
	setSpotSeams(calc)
	opens := 0
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		opens++
		return nil, errors.New("credentials unavailable")
	}
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}})
	require.NoError(t, err)
	require.Len(t, spotRows(resp.GetRecommendations()), 1)
	require.Len(t, resp.GetRecommendations(), 1)
	require.Equal(t, 1, opens)
}

func TestRecommendationsSpotOnlyNeedsNoCostExplorerClient(t *testing.T) {
	calc := NewCalculator()
	setSpotSeams(calc)
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		t.Error("opened a Cost Explorer client for a Spot-only request")
		return nil, errors.New("unexpected client")
	}
	resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{
		Filter:          &pbc.RecommendationFilter{Source: spotRecommendationSource},
		TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")},
	})
	require.NoError(t, err)
	require.Len(t, spotRows(resp.GetRecommendations()), 1)
	require.Len(t, resp.GetRecommendations(), 1)
}

func TestRecommendationsAllSelectedSourcesFailReturnsError(t *testing.T) {
	target := []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}
	failingClient := func(calc *Calculator) {
		calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
			return nil, errors.New("credentials unavailable")
		}
	}
	for _, tc := range []struct {
		name string
		req  *pbc.GetRecommendationsRequest
		spot bool
	}{
		{"all Cost Explorer sources", &pbc.GetRecommendationsRequest{}, false},
		{"Spot enabled with no quote seams", &pbc.GetRecommendationsRequest{TargetResources: target}, true},
		{"Spot-only without quote seams", &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{Source: spotRecommendationSource}, TargetResources: target}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calc := NewCalculator()
			failingClient(calc)
			if tc.spot {
				calc.spotComparison = true
			}
			resp, err := calc.GetRecommendations(t.Context(), tc.req)
			require.Nil(t, resp)
			require.Equal(t, codes.Internal, status.Code(err))
			require.Equal(t, "recommendation client initialization failed", status.Convert(err).Message())
		})
	}
}

func TestRecommendationsHardFailuresStopSpotAndSources(t *testing.T) {
	target := []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}
	for _, tc := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{"budget", client.ErrRateLimited, codes.ResourceExhausted},
		{"cancellation", context.Canceled, codes.Canceled},
		{"deadline", context.DeadlineExceeded, codes.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calc := spotIsolationCalculator(&recommendationAPI{rightFail: tc.err})
			resp, err := calc.GetRecommendations(t.Context(), &pbc.GetRecommendationsRequest{TargetResources: target})
			require.Nil(t, resp)
			require.Equal(t, tc.code, status.Code(err))
		})
	}
}

func TestRecommendationsRequestCredentialFailureStaysHard(t *testing.T) {
	calc := NewCalculator()
	setSpotSeams(calc)
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		return nil, client.ErrRegionMissing
	}
	ctx := credentialContext(t, map[string]string{"access_key_id": "synthetic-access", "secret_access_key": "synthetic-secret", "session_token": "synthetic-token", "role_arn": credentialRole})
	resp, err := calc.GetRecommendations(ctx, &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{spotRecommendationTarget(t, "us-east-1a")}})
	require.Nil(t, resp)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
