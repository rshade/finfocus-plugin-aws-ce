package pricing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type recommendationAPI struct {
	mockCostExplorerAPI
	calls      []string
	fail       error
	rights     []costexplorer.GetRightsizingRecommendationOutput
	rightInput *costexplorer.GetRightsizingRecommendationInput
	planInputs []*costexplorer.GetSavingsPlansPurchaseRecommendationInput
	planFail   error
	rightFail  error
}

func (m *recommendationAPI) GetRightsizingRecommendation(_ context.Context, in *costexplorer.GetRightsizingRecommendationInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetRightsizingRecommendationOutput, error) {
	m.calls = append(m.calls, "ec2")
	m.rightInput = in
	if m.rightFail != nil {
		return nil, m.rightFail
	}
	if m.fail != nil {
		return nil, m.fail
	}
	if len(m.rights) > 0 {
		return &m.rights[0], nil
	}
	return &costexplorer.GetRightsizingRecommendationOutput{}, nil
}
func (m *recommendationAPI) GetSavingsPlansPurchaseRecommendation(_ context.Context, in *costexplorer.GetSavingsPlansPurchaseRecommendationInput, _ ...func(*costexplorer.Options)) (*costexplorer.GetSavingsPlansPurchaseRecommendationOutput, error) {
	m.calls = append(m.calls, string(in.SavingsPlansType))
	m.planInputs = append(m.planInputs, in)
	if m.planFail != nil {
		return nil, m.planFail
	}
	if m.fail != nil {
		return nil, m.fail
	}
	return &costexplorer.GetSavingsPlansPurchaseRecommendationOutput{SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{SavingsPlansType: in.SavingsPlansType, SavingsPlansPurchaseRecommendationDetails: []types.SavingsPlansPurchaseRecommendationDetail{{AccountId: aws.String("123"), RecommendationDetailId: aws.String(string(in.SavingsPlansType)), EstimatedMonthlySavingsAmount: aws.String("300"), CurrencyCode: aws.String("USD")}}}}, nil
}
func recommendationCalculator(m *recommendationAPI) *Calculator {
	return NewCalculatorWithClient(client.NewClientWithAPI(m, "us-east-1"))
}
func TestRecommendationsValidation(t *testing.T) {
	cases := []*pbc.GetRecommendationsRequest{nil, {PageToken: "bad"}, {ProjectionPeriod: "weekly"}, {Filter: &pbc.RecommendationFilter{Category: 999}}, {Filter: &pbc.RecommendationFilter{ActionType: 999}}, {Filter: &pbc.RecommendationFilter{Priority: 999}}, {Filter: &pbc.RecommendationFilter{SortBy: 999}}, {Filter: &pbc.RecommendationFilter{SortOrder: 999}}, {Filter: &pbc.RecommendationFilter{MinEstimatedSavings: math.NaN()}}, {Filter: &pbc.RecommendationFilter{MinConfidenceScore: math.Inf(1)}}, {TargetResources: make([]*pbc.ResourceDescriptor, 101)}, {TargetResources: []*pbc.ResourceDescriptor{nil}}, {TargetResources: []*pbc.ResourceDescriptor{{Provider: "aws"}}}, {TargetResources: []*pbc.ResourceDescriptor{{Provider: "bogus", ResourceType: "ec2"}}}}
	for i, req := range cases {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			c := NewCalculator()
			c.openClient = func(context.Context, client.Config) (*client.Client, error) {
				t.Fatal("opened client before validation")
				return nil, nil
			}
			_, err := c.GetRecommendations(context.Background(), req)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}
func TestRecommendationsSelection(t *testing.T) {
	for _, tc := range []struct {
		name         string
		f            *pbc.RecommendationFilter
		count, calls int
	}{
		{"all", nil, 3, 3}, {"provider", &pbc.RecommendationFilter{Provider: "gcp"}, 0, 0}, {"source", &pbc.RecommendationFilter{Source: "other"}, 0, 0}, {"category", &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_SECURITY}, 0, 0}, {"action", &pbc.RecommendationFilter{ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_RIGHTSIZE}, 1, 1}, {"region", &pbc.RecommendationFilter{Region: "us-east-1"}, 1, 3}, {"sku", &pbc.RecommendationFilter{Sku: "m5.large"}, 1, 3}, {"account", &pbc.RecommendationFilter{AccountId: "456"}, 0, 3}, {"id", &pbc.RecommendationFilter{ResourceId: "i-123"}, 1, 3}, {"confidence absent", &pbc.RecommendationFilter{MinConfidenceScore: .1}, 0, 3}, {"age absent", &pbc.RecommendationFilter{MaxAgeDays: 3}, 0, 3}, {"tag absent empty", &pbc.RecommendationFilter{Tags: map[string]string{"missing": ""}}, 0, 3}, {"minimum", &pbc.RecommendationFilter{MinEstimatedSavings: 100}, 2, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &recommendationAPI{rights: rightsizingFixture()}
			r, e := recommendationCalculator(m).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: tc.f})
			require.NoError(t, e)
			require.Len(t, r.Recommendations, tc.count)
			require.Len(t, m.calls, tc.calls)
		})
	}
}
func TestRecommendationsTargets(t *testing.T) {
	for _, tc := range []struct {
		target *pbc.ResourceDescriptor
		count  int
	}{
		{&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2"}, 1}, {&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:ec2/instance:Instance", Id: "i-123", Sku: "m5.large", Region: "us-east-1"}, 1}, {&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2:Instance", Arn: "arn:awsbogus:ec2:us-east-1:123:instance/i-123"}, 0}, {&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2:Instance", Arn: "arn:aws-cn:ec2:us-east-1:123:instance/i-123"}, 0}, {&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2:Instance", Id: "other"}, 0}, {&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2:Instance", Arn: "arn:aws:ec2:us-east-1:123:instance/i-123"}, 1}, {&pbc.ResourceDescriptor{Provider: "aws", ResourceType: "savingsplans:Account", Region: "us-east-1"}, 0},
	} {
		m := &recommendationAPI{rights: rightsizingFixture()}
		r, e := recommendationCalculator(m).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{tc.target}})
		require.NoError(t, e)
		require.Len(t, r.Recommendations, tc.count)
	}
}
func TestRecommendationsPagination(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture()}
	c := recommendationCalculator(m)
	first, e := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageSize: 1})
	require.NoError(t, e)
	require.Len(t, first.Recommendations, 1)
	require.EqualValues(t, 1, first.Summary.TotalRecommendations)
	second, e := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageSize: 1, PageToken: first.NextPageToken})
	require.NoError(t, e)
	require.NotEqual(t, first.Recommendations[0].Id, second.Recommendations[0].Id)
	excluded, e := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{IncludeDismissed: true, ExcludedRecommendationIds: []string{first.Recommendations[0].Id}})
	require.NoError(t, e)
	require.Len(t, excluded.Recommendations, 2)
	projected, e := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{ProjectionPeriod: "daily", Filter: &pbc.RecommendationFilter{MinEstimatedSavings: 11}})
	require.NoError(t, e)
	require.Empty(t, projected.Recommendations)
	m.rights[0].RightsizingRecommendations[0].CurrentInstance.CurrencyCode = aws.String("EUR")
	_, e = c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageSize: 1})
	require.Equal(t, codes.DataLoss, status.Code(e))
}
func TestRecommendationsFailures(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code codes.Code
	}{{context.Canceled, codes.Canceled}, {context.DeadlineExceeded, codes.DeadlineExceeded}, {client.ErrRateLimited, codes.ResourceExhausted}, {&smithy.GenericAPIError{Code: "AccessDeniedException", Message: "secret"}, codes.PermissionDenied}, {&smithy.GenericAPIError{Code: "DataUnavailableException"}, codes.FailedPrecondition}, {&smithy.GenericAPIError{Code: "ThrottlingException"}, codes.ResourceExhausted}, {errors.New("secret"), codes.Internal}} {
		m := &recommendationAPI{fail: tc.err}
		r, e := recommendationCalculator(m).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
		require.Nil(t, r)
		require.Equal(t, tc.code, status.Code(e))
		require.NotContains(t, e.Error(), "secret")
	}
}

func TestRecommendationsRequestDefaults(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture()}
	c := recommendationCalculator(m)
	_, err := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{AccountId: "123", Region: "us-east-1"}})
	require.NoError(t, err)
	require.Equal(t, "AmazonEC2", aws.ToString(m.rightInput.Service))
	require.True(t, m.rightInput.Configuration.BenefitsConsidered)
	require.Equal(t, types.RecommendationTargetSameInstanceFamily, m.rightInput.Configuration.RecommendationTarget)
	require.NotNil(t, m.rightInput.Filter.Dimensions)
	require.Equal(t, types.DimensionLinkedAccount, m.rightInput.Filter.Dimensions.Key)
	require.Equal(t, []string{"123"}, m.rightInput.Filter.Dimensions.Values)
	require.Len(t, m.planInputs, 2)
	for _, in := range m.planInputs {
		require.Equal(t, types.LookbackPeriodInDaysThirtyDays, in.LookbackPeriodInDays)
		require.Equal(t, types.TermInYearsOneYear, in.TermInYears)
		require.Equal(t, types.PaymentOptionNoUpfront, in.PaymentOption)
		require.Equal(t, types.AccountScopePayer, in.AccountScope)
		require.Equal(t, types.DimensionLinkedAccount, in.Filter.Dimensions.Key)
		require.Equal(t, []string{"123"}, in.Filter.Dimensions.Values)
	}
}
func TestRecommendationsLargePages(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture()}
	row := m.rights[0].RightsizingRecommendations[0]
	m.rights[0].RightsizingRecommendations = nil
	for i := 0; i < 1100; i++ {
		item := row
		current := *row.CurrentInstance
		current.ResourceId = aws.String(fmt.Sprintf("i-%04d", i))
		item.CurrentInstance = &current
		m.rights[0].RightsizingRecommendations = append(m.rights[0].RightsizingRecommendations, item)
	}
	c := recommendationCalculator(m)
	f := &pbc.RecommendationFilter{ResourceType: "ec2", SortBy: pbc.RecommendationSortBy_RECOMMENDATION_SORT_BY_ESTIMATED_SAVINGS}
	first, err := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: f})
	require.NoError(t, err)
	require.Len(t, first.Recommendations, 50)
	require.Equal(t, 625.0, first.Summary.TotalEstimatedSavings)
	second, err := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: f, PageToken: first.NextPageToken})
	require.NoError(t, err)
	require.Len(t, second.Recommendations, 50)
	require.Less(t, first.Recommendations[49].Id, second.Recommendations[0].Id)
	max, err := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: f, PageSize: 2000})
	require.NoError(t, err)
	require.Len(t, max.Recommendations, 1000)
}
func TestRecommendationsStrictMetadata(t *testing.T) {
	r, err := mapRightsizingRecommendations(rightsizingFixture())
	require.NoError(t, err)
	r[0].CreatedAt = timestamppb.New(time.Now().Add(-48 * time.Hour))
	r[0].ConfidenceScore = aws.Float64(.8)
	require.True(t, recommendationStrictMatch(r[0], &pbc.RecommendationFilter{MaxAgeDays: 3, MinConfidenceScore: .5}, nil))
	selected := pluginsdk.ApplyRecommendationFilter(r, &pbc.RecommendationFilter{MaxAgeDays: 1})
	require.Empty(t, selected)
	targets := []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "ec2", Tags: map[string]string{"missing": ""}}}
	require.False(t, recommendationStrictMatch(r[0], nil, targets))
}
func TestRecommendationsNoPartialResponse(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture(), planFail: errors.New("secret")}
	r, err := recommendationCalculator(m).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	require.Nil(t, r)
	require.Equal(t, codes.Internal, status.Code(err))
	require.Len(t, m.calls, 2)
}
func TestRecommendationsInvalidPayload(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture()}
	m.rights[0].RightsizingRecommendations[0].CurrentInstance.CurrencyCode = nil
	r, err := recommendationCalculator(m).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	require.Nil(t, r)
	require.Equal(t, codes.DataLoss, status.Code(err))
}
func TestRecommendationsBudget(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture()}
	c := recommendationCalculator(m)
	c.maxRequestsPerMinute = 1
	r, err := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	require.Nil(t, r)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Len(t, m.calls, 1)
}

func TestRecommendationsLargeAge(t *testing.T) {
	m := &recommendationAPI{rights: rightsizingFixture()}
	m.rights[0].Metadata = &types.RightsizingRecommendationMetadata{GenerationTimestamp: aws.String(time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339))}
	r, err := recommendationCalculator(m).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{MaxAgeDays: 106752}})
	require.NoError(t, err)
	require.Len(t, r.Recommendations, 1)
}

func TestRecommendationsTargetSourceSelection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filter  *pbc.RecommendationFilter
		targets []*pbc.ResourceDescriptor
		calls   []string
		count   int
	}{
		{name: "instance suppresses unavailable plans", targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "ec2:Instance", Id: "i-123"}}, calls: []string{"ec2"}, count: 1},
		{name: "plan suppresses rightsizing", targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "savingsplans:Account"}}, calls: []string{"COMPUTE_SP", "EC2_INSTANCE_SP"}, count: 2},
		{name: "unsupported AWS target", targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "rds:Instance"}}},
		{name: "non AWS target", targets: []*pbc.ResourceDescriptor{{Provider: "gcp", ResourceType: "ec2"}}},
		{name: "contradictory filter", filter: &pbc.RecommendationFilter{ResourceType: "savingsplans:Account"}, targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "ec2"}}},
		{name: "target union", targets: []*pbc.ResourceDescriptor{{Provider: "gcp", ResourceType: "ec2"}, {Provider: "aws", ResourceType: "ec2"}, {Provider: "aws", ResourceType: "savingsplans:Account"}}, calls: []string{"ec2", "COMPUTE_SP", "EC2_INSTANCE_SP"}, count: 3},
		{name: "union intersect filter", filter: &pbc.RecommendationFilter{ResourceType: "ec2"}, targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "savingsplans:Account"}, {Provider: "aws", ResourceType: "ec2"}}, calls: []string{"ec2"}, count: 1},
		{name: "target and filter identity", filter: &pbc.RecommendationFilter{ResourceId: "other"}, targets: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "ec2", Id: "i-123"}}, calls: []string{"ec2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &recommendationAPI{rights: rightsizingFixture()}
			if len(tc.calls) == 1 {
				m.planFail = errors.New("unrelated plans unavailable")
			}
			if len(tc.calls) == 2 {
				m.rightFail = errors.New("unrelated rightsizing unavailable")
			}
			c := recommendationCalculator(m)
			if len(tc.calls) == 0 {
				c = NewCalculator()
				c.openClient = func(context.Context, client.Config) (*client.Client, error) {
					t.Error("initialized client for incompatible targets")
					return nil, errors.New("credentials unavailable")
				}
			}
			r, err := c.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: tc.filter, TargetResources: tc.targets})
			require.NoError(t, err)
			require.Len(t, r.GetRecommendations(), tc.count)
			require.Equal(t, tc.calls, m.calls)
		})
	}
}
