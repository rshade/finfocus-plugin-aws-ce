package pricing

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mixedRecommendationAPI struct {
	recommendationAPI
	anomalyTokens []string
	anomalyErr    error
	repeat        bool
	endless       bool
}

func (m *mixedRecommendationAPI) GetAnomalies(_ context.Context, in *costexplorer.GetAnomaliesInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetAnomaliesOutput, error) {
	token := aws.ToString(in.NextPageToken)
	m.anomalyTokens = append(m.anomalyTokens, token)
	if m.anomalyErr != nil {
		return nil, m.anomalyErr
	}
	if m.endless {
		return &costexplorer.GetAnomaliesOutput{NextPageToken: aws.String(token + "x")}, nil
	}
	next := "aws-next"
	if token != "" && !m.repeat {
		next = ""
	}
	id := "anomaly-first"
	if token != "" {
		id = "anomaly-second"
	}
	return &costexplorer.GetAnomaliesOutput{Anomalies: []types.Anomaly{{AnomalyId: aws.String(id), Impact: &types.Impact{TotalActualSpend: aws.Float64(40), TotalExpectedSpend: aws.Float64(30), TotalImpact: 10}}}, NextPageToken: aws.String(next)}, nil
}

func TestRecommendationsCombinedSources(t *testing.T) {
	for _, source := range []string{"", "aws-cost-explorer"} {
		t.Run(source, func(t *testing.T) {
			api := &mixedRecommendationAPI{recommendationAPI: recommendationAPI{rights: rightsizingFixture()}}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
			req := &pbc.GetRecommendationsRequest{ProjectionPeriod: "annual", Filter: &pbc.RecommendationFilter{Source: source}}
			resp, err := calc.GetRecommendations(context.Background(), req)
			require.NoError(t, err)
			require.Len(t, resp.GetRecommendations(), 5)
			require.Equal(t, []string{"ec2", "COMPUTE_SP", "EC2_INSTANCE_SP"}, api.calls)
			require.Equal(t, []string{"", "aws-next"}, api.anomalyTokens)
			require.Equal(t, 7350.0, resp.GetSummary().GetTotalEstimatedSavings())
			for _, rec := range resp.GetRecommendations() {
				if rec.GetCategory() == pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY {
					require.Equal(t, 40.0, rec.GetImpact().GetCurrentCost())
					require.Equal(t, 30.0, rec.GetImpact().GetProjectedCost())
					require.Zero(t, rec.GetImpact().GetEstimatedSavings())
					require.Equal(t, "anomaly", rec.GetImpact().GetProjectionPeriod())
				}
			}
			req.PageSize = 1
			var ids []string
			for {
				page, err := calc.GetRecommendations(context.Background(), req)
				require.NoError(t, err)
				require.Len(t, page.GetRecommendations(), 1)
				ids = append(ids, page.GetRecommendations()[0].GetId())
				if page.GetNextPageToken() == "" {
					break
				}
				req.PageToken = page.GetNextPageToken()
				require.Less(t, len(ids), 6)
			}
			require.Len(t, ids, 5)
			require.Contains(t, ids, "anomaly-first")
			require.Contains(t, ids, "anomaly-second")
			req.PageToken = ""
			req.PageSize = 0
			req.ExcludedRecommendationIds = []string{"anomaly-first"}
			resp, err = calc.GetRecommendations(context.Background(), req)
			require.NoError(t, err)
			require.Len(t, resp.GetRecommendations(), 4)
			api.anomalyTokens = nil
			req.Filter.Category = pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST
			resp, err = calc.GetRecommendations(context.Background(), req)
			require.NoError(t, err)
			require.Len(t, resp.GetRecommendations(), 3)
			require.Empty(t, api.anomalyTokens)
		})
	}
}

func TestRecommendationsCombinedAnomalyPageGuards(t *testing.T) {
	for _, tc := range []struct {
		name            string
		repeat, endless bool
		code            codes.Code
		calls           int
	}{
		{name: "repeat", repeat: true, code: codes.Internal, calls: 2},
		{name: "limit", endless: true, code: codes.ResourceExhausted, calls: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &mixedRecommendationAPI{repeat: tc.repeat, endless: tc.endless}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
			_, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
			require.Equal(t, tc.code, status.Code(err))
			require.Len(t, api.anomalyTokens, tc.calls)
		})
	}
}

func TestRecommendationsTargetSkipsAnomalies(t *testing.T) {
	api := &mixedRecommendationAPI{recommendationAPI: recommendationAPI{rights: rightsizingFixture()}}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	_, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "ec2"}}})
	require.NoError(t, err)
	require.Equal(t, []string{"ec2"}, api.calls)
	require.Empty(t, api.anomalyTokens)
}

func TestRecommendationsInvestigateUsesAWSToken(t *testing.T) {
	api := &mixedRecommendationAPI{}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageToken: "aws-next", Filter: &pbc.RecommendationFilter{ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_INVESTIGATE}})
	require.NoError(t, err)
	require.Len(t, resp.GetRecommendations(), 1)
	require.Equal(t, "anomaly-second", resp.GetRecommendations()[0].GetId())
	require.Equal(t, []string{"aws-next"}, api.anomalyTokens)
	require.Empty(t, api.calls)
}

func TestRecommendationsIncompatibleAnomalyFilters(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter *pbc.RecommendationFilter
		count  int
	}{
		{"resource ID", &pbc.RecommendationFilter{ResourceId: "i-123"}, 1},
		{"minimum savings", &pbc.RecommendationFilter{MinEstimatedSavings: 100}, 2},
		{"EC2 type", &pbc.RecommendationFilter{ResourceType: "ec2"}, 1},
		{"EC2 canonical type", &pbc.RecommendationFilter{ResourceType: "ec2:Instance"}, 1},
		{"EC2 Pulumi type", &pbc.RecommendationFilter{ResourceType: "aws:ec2/instance:Instance"}, 1},
		{"Savings Plans type", &pbc.RecommendationFilter{ResourceType: "savingsplans:Account"}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &mixedRecommendationAPI{recommendationAPI: recommendationAPI{rights: rightsizingFixture()}, anomalyErr: &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "anomaly access denied"}}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
			resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: tc.filter})
			require.NoError(t, err)
			require.Len(t, resp.GetRecommendations(), tc.count)
			require.Empty(t, api.anomalyTokens, "incompatible anomalies must not need unrelated IAM permissions")
			require.NotEmpty(t, api.calls)
		})
	}
}

func TestRecommendationsProviderRetainsRootlessAnomalies(t *testing.T) {
	api := &mixedRecommendationAPI{recommendationAPI: recommendationAPI{rights: rightsizingFixture()}}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{Provider: "aws"}})
	require.NoError(t, err)
	require.Len(t, resp.GetRecommendations(), 5)
	anomalies := 0
	for _, rec := range resp.GetRecommendations() {
		require.Equal(t, "aws", rec.GetResource().GetProvider())
		if rec.GetCategory() == pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY {
			anomalies++
		}
	}
	require.Equal(t, 2, anomalies)
}

func TestRecommendationsCombinedSourcesShareClient(t *testing.T) {
	calc := NewCalculator()
	var apis []*mixedRecommendationAPI
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		api := &mixedRecommendationAPI{recommendationAPI: recommendationAPI{rights: rightsizingFixture()}}
		apis = append(apis, api)
		return client.NewClientWithAPI(api, "us-east-1"), nil
	}
	ctx := credentialContext(t, map[string]string{"access_key_id": "synthetic-access", "secret_access_key": "synthetic-secret", "session_token": "synthetic-token", "role_arn": credentialRole})
	resp, err := calc.GetRecommendations(ctx, &pbc.GetRecommendationsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetRecommendations(), 5)
	require.Len(t, apis, 1)
	require.Equal(t, []string{"ec2", "COMPUTE_SP", "EC2_INSTANCE_SP"}, apis[0].calls)
	require.Equal(t, []string{"", "aws-next"}, apis[0].anomalyTokens)
}
