package pricing

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/aws/smithy-go"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type reservationTestAPI struct {
	recommendationAPI
	inputs []*costexplorer.GetReservationPurchaseRecommendationInput
	fetch  func(*costexplorer.GetReservationPurchaseRecommendationInput) (*costexplorer.GetReservationPurchaseRecommendationOutput, error)
}

func (m *reservationTestAPI) GetReservationPurchaseRecommendation(_ context.Context, in *costexplorer.GetReservationPurchaseRecommendationInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetReservationPurchaseRecommendationOutput, error) {
	m.inputs = append(m.inputs, in)
	if m.fetch != nil {
		return m.fetch(in)
	}
	return &costexplorer.GetReservationPurchaseRecommendationOutput{}, nil
}

func TestReservationSourceSelection(t *testing.T) {
	all := []string{"Amazon Relational Database Service", "Amazon Redshift", "Amazon ElastiCache", "Amazon Elasticsearch Service"}
	for _, tc := range []struct {
		name    string
		filter  *pbc.RecommendationFilter
		targets []*pbc.ResourceDescriptor
		want    []string
	}{
		{name: "unfiltered", want: all},
		{name: "commitments", filter: &pbc.RecommendationFilter{ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT}, want: all},
		{name: "rds", filter: &pbc.RecommendationFilter{ResourceType: "rds:Account"}, want: all[:1]},
		{name: "rds alias", filter: &pbc.RecommendationFilter{ResourceType: "AmazonRDS"}, want: all[:1]},
		{name: "redshift", filter: &pbc.RecommendationFilter{ResourceType: "redshift:Account"}, want: all[1:2]},
		{name: "cache", filter: &pbc.RecommendationFilter{ResourceType: "elasticache:Account"}, want: all[2:3]},
		{name: "opensearch", filter: &pbc.RecommendationFilter{ResourceType: "es:Account"}, want: all[3:]},
		{name: "target", targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "rds:Account"}}, want: all[:1]},
		{name: "target union", targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "rds:Account"}, {Provider: "aws", ResourceType: "es:Account"}}, want: []string{all[0], all[3]}},
		{name: "conflicting target", filter: &pbc.RecommendationFilter{ResourceType: "rds:Account"}, targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "es:Account"}}},
		{name: "rightsizing", filter: &pbc.RecommendationFilter{ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_RIGHTSIZE}},
		{name: "EC2", filter: &pbc.RecommendationFilter{ResourceType: "ec2"}},
		{name: "plans", filter: &pbc.RecommendationFilter{ResourceType: "savingsplans:Account"}},
		{name: "other provider", filter: &pbc.RecommendationFilter{Provider: "gcp"}},
		{name: "other source", filter: &pbc.RecommendationFilter{Source: "other"}},
		{name: "security", filter: &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_SECURITY}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &reservationTestAPI{}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
			_, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: tc.filter, TargetResources: tc.targets})
			require.NoError(t, err)
			var services []string
			for _, in := range api.inputs {
				services = append(services, aws.ToString(in.Service))
				require.Equal(t, types.AccountScopePayer, in.AccountScope)
				require.Equal(t, types.LookbackPeriodInDaysThirtyDays, in.LookbackPeriodInDays)
				require.Equal(t, types.TermInYearsOneYear, in.TermInYears)
				require.Equal(t, types.PaymentOptionNoUpfront, in.PaymentOption)
				require.Nil(t, in.Filter, "reservation filters have different AWS restrictions")
				require.Nil(t, in.ServiceSpecification, "EC2 specifications must not be sent")
			}
			require.Equal(t, tc.want, services)
			if len(tc.want) == 1 {
				require.Empty(t, api.calls, "service-scoped requests must skip unrelated sources")
			}
		})
	}
}

func TestReservationSDKResponse(t *testing.T) {
	api := &reservationTestAPI{fetch: func(in *costexplorer.GetReservationPurchaseRecommendationInput) (*costexplorer.GetReservationPurchaseRecommendationOutput, error) {
		require.Equal(t, "123456789012", aws.ToString(in.AccountId))
		require.Nil(t, in.Filter)
		page := reservationFixture()[0]
		if aws.ToString(in.NextPageToken) == "" {
			page.NextPageToken = aws.String("aws-next")
		} else {
			require.Equal(t, "aws-next", aws.ToString(in.NextPageToken))
			page.Recommendations[0].RecommendationDetails[0].InstanceDetails.RDSInstanceDetails.Region = aws.String("us-west-2")
		}
		return &page, nil
	}}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	srv := pluginsdk.NewTestServer(t, calc)
	defer srv.Close()
	resp, err := srv.Client().GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		Filter: &pbc.RecommendationFilter{ResourceType: "AmazonRDS", AccountId: "123456789012", Region: "us-east-1"}, ProjectionPeriod: "annual",
	})
	require.NoError(t, err)
	require.Len(t, api.inputs, 2)
	require.Len(t, resp.GetRecommendations(), 1)
	r := resp.GetRecommendations()[0]
	require.NoError(t, pluginsdk.ValidateRecommendation(r))
	require.Equal(t, "rds:Account", r.GetResource().GetResourceType())
	require.Equal(t, "reserved_instance", r.GetCommitment().GetCommitmentType())
	require.Equal(t, 306.0, r.GetImpact().GetEstimatedSavings())
	require.Equal(t, "125.75", r.GetMetadata()["upfront_cost"])
	require.Equal(t, "25.50", r.GetMetadata()["monthly_savings"])
	require.Equal(t, "annual", r.GetImpact().GetProjectionPeriod())
	require.NoError(t, pluginsdk.ValidateRecommendationSummary(resp.GetSummary()))
	require.EqualValues(t, 1, resp.GetSummary().GetTotalRecommendations())
	require.Equal(t, 306.0, resp.GetSummary().GetTotalEstimatedSavings())
	require.Equal(t, "USD", resp.GetSummary().GetCurrency())
	require.Empty(t, resp.GetNextPageToken())
}

func TestReservationFailuresAreAtomic(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{"permission", &smithy.GenericAPIError{Code: "AccessDeniedException", Message: "secret"}, codes.PermissionDenied},
		{"missing dataset", &smithy.GenericAPIError{Code: "DataUnavailableException"}, codes.FailedPrecondition},
		{"budget", client.ErrRateLimited, codes.ResourceExhausted},
		{"cancellation", context.Canceled, codes.Canceled},
		{"invalid payload", nil, codes.DataLoss},
		{"internal", errors.New("secret"), codes.Internal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &reservationTestAPI{recommendationAPI: recommendationAPI{rights: rightsizingFixture()}, fetch: func(*costexplorer.GetReservationPurchaseRecommendationInput) (*costexplorer.GetReservationPurchaseRecommendationOutput, error) {
				page := reservationFixture()[0]
				page.Recommendations[0].RecommendationDetails[0].EstimatedMonthlySavingsAmount = aws.String("NaN")
				return &page, tc.err
			}}
			calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
			resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST}})
			require.Equal(t, tc.code, status.Code(err))
			require.Nil(t, resp, "failure must not expose earlier rightsizing or Savings Plans results")
			require.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestReservationSharesRequestBudget(t *testing.T) {
	api := &reservationTestAPI{}
	calc := NewCalculatorWithClient(client.NewClientWithAPI(api, "us-east-1"))
	calc.maxRequestsPerMinute, calc.maxWait = 4, 0
	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST}})
	require.Nil(t, resp)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Len(t, api.calls, 3)
	require.Len(t, api.inputs, 1, "three existing sources plus one reservation consume the budget")
}

func TestReservationRequestCredentials(t *testing.T) {
	calc := NewCalculator()
	var configs []client.Config
	var apis []*reservationTestAPI
	calc.openClient = func(_ context.Context, cfg client.Config) (*client.Client, error) {
		configs = append(configs, cfg)
		api := &reservationTestAPI{}
		apis = append(apis, api)
		return client.NewClientWithAPI(api, "us-east-1"), nil
	}
	ctx := credentialContext(t, map[string]string{
		"access_key_id": "synthetic-access", "secret_access_key": "synthetic-secret",
		"session_token": "synthetic-token", "role_arn": credentialRole,
	})
	for _, callCtx := range []context.Context{ctx, context.Background()} {
		_, err := calc.GetRecommendations(callCtx, &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{ResourceType: "rds:Account"}})
		require.NoError(t, err)
	}
	require.Len(t, configs, 2)
	require.Equal(t, "synthetic-access", configs[0].AccessKeyID)
	require.Equal(t, "synthetic-secret", configs[0].SecretAccessKey)
	require.Equal(t, "synthetic-token", configs[0].SessionToken)
	require.Equal(t, credentialRole, configs[0].RoleARN)
	require.Empty(t, configs[1].AccessKeyID)
	require.Empty(t, configs[1].SecretAccessKey)
	require.Empty(t, configs[1].SessionToken)
	require.Empty(t, configs[1].RoleARN)
	for _, api := range apis {
		require.Len(t, api.inputs, 1)
		require.Empty(t, api.calls)
	}
}

func TestReservationIncompatibleTargetsAvoidCredentials(t *testing.T) {
	calc := NewCalculator()
	calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
		t.Error("initialized credentials for conflicting filter and target")
		return nil, errors.New("credentials unavailable")
	}
	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		Filter:          &pbc.RecommendationFilter{ResourceType: "rds:Account"},
		TargetResources: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "es:Account"}},
	})
	require.NoError(t, err)
	require.Empty(t, resp.GetRecommendations())
	require.NotNil(t, resp.GetSummary())
}
