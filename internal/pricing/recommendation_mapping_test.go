package pricing

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"testing"
)

func rightsizingFixture() []costexplorer.GetRightsizingRecommendationOutput {
	return []costexplorer.GetRightsizingRecommendationOutput{{
		RightsizingRecommendations: []types.RightsizingRecommendation{{
			AccountId:       aws.String("123"),
			RightsizingType: types.RightsizingTypeModify,
			CurrentInstance: &types.CurrentInstance{
				ResourceId: aws.String("i-123"), CurrencyCode: aws.String("USD"),
				ResourceDetails: &types.ResourceDetails{EC2ResourceDetails: &types.EC2ResourceDetails{
					InstanceType: aws.String("m5.large"), Region: aws.String("us-east-1"),
				}},
			},
			ModifyRecommendationDetail: &types.ModifyRecommendationDetail{TargetInstances: []types.TargetInstance{
				{EstimatedMonthlySavings: aws.String("12.50"), ResourceDetails: &types.ResourceDetails{EC2ResourceDetails: &types.EC2ResourceDetails{InstanceType: aws.String("m5.medium")}}},
				{EstimatedMonthlySavings: aws.String("99")},
			}},
		}},
	}}
}
func TestMapRightsizingRecommendations(t *testing.T) {
	pages := rightsizingFixture()
	recs, err := mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, 12.5, recs[0].GetImpact().GetEstimatedSavings())
	require.Equal(t, "i-123", recs[0].Resource.Id)
	require.Equal(t, "ec2:Instance", recs[0].Resource.ResourceType)
	require.Equal(t, "us-east-1", recs[0].Resource.Region)
	require.Equal(t, "m5.large", recs[0].GetRightsize().CurrentInstanceType)
	require.Equal(t, "12.50", recs[0].Metadata["monthly_savings"])
	require.Equal(t, "123", recs[0].Metadata["account_id"])
	require.NoError(t, pluginsdk.ValidateRecommendation(recs[0]))
	require.Equal(t, "m5.medium", recs[0].GetRightsize().GetRecommendedInstanceType())
	require.Nil(t, recs[0].ConfidenceScore)
	require.Nil(t, recs[0].CreatedAt)
	again, err := mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.Equal(t, recs[0].Id, again[0].Id)
	pages[0].RightsizingRecommendations[0].AccountId = aws.String("456")
	other, err := mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.NotEqual(t, recs[0].Id, other[0].Id)
	pages[0].RightsizingRecommendations[0].RightsizingType = types.RightsizingTypeTerminate
	pages[0].RightsizingRecommendations[0].TerminateRecommendationDetail = &types.TerminateRecommendationDetail{EstimatedMonthlySavings: aws.String("300"), CurrencyCode: aws.String("USD")}
	recs, err = mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.NotNil(t, recs[0].GetTerminate())
}
func TestMapSavingsPlansRecommendations(t *testing.T) {
	for _, kind := range []types.SupportedSavingsPlansType{types.SupportedSavingsPlansTypeComputeSp, types.SupportedSavingsPlansTypeEc2InstanceSp} {
		t.Run(string(kind), func(t *testing.T) {
			pages := []costexplorer.GetSavingsPlansPurchaseRecommendationOutput{{
				SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{SavingsPlansType: kind, TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront,
					SavingsPlansPurchaseRecommendationSummary: &types.SavingsPlansPurchaseRecommendationSummary{CurrencyCode: aws.String("USD")},
					SavingsPlansPurchaseRecommendationDetails: []types.SavingsPlansPurchaseRecommendationDetail{
						{AccountId: aws.String("123"), EstimatedMonthlySavingsAmount: aws.String("300"), HourlyCommitmentToPurchase: aws.String("1.5"), UpfrontCost: aws.String("0")},
						{AccountId: aws.String("456"), EstimatedMonthlySavingsAmount: aws.String("400")}}}}}
			recs, err := mapSavingsPlansRecommendations(pages)
			require.NoError(t, err)
			require.Len(t, recs, 2)
			require.NoError(t, pluginsdk.ValidateRecommendation(recs[0]))
			require.Nil(t, recs[0].ConfidenceScore)
			require.Equal(t, "USD", recs[0].Impact.Currency)
			require.Empty(t, recs[0].Resource.Region)
			d := pages[0].SavingsPlansPurchaseRecommendation.SavingsPlansPurchaseRecommendationDetails
			d[0], d[1] = d[1], d[0]
			again, err := mapSavingsPlansRecommendations(pages)
			require.NoError(t, err)
			require.Equal(t, recs[0].Id, again[1].Id)
		})
	}
}
func TestMapSavingsPlansRecommendationsEmpty(t *testing.T) {
	pages := []costexplorer.GetSavingsPlansPurchaseRecommendationOutput{{
		SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{},
	}}
	recs, err := mapSavingsPlansRecommendations(pages)
	require.NoError(t, err)
	require.Empty(t, recs)
	pages[0].SavingsPlansPurchaseRecommendation.SavingsPlansType = types.SupportedSavingsPlansTypeComputeSp
	recs, err = mapSavingsPlansRecommendations(pages)
	require.NoError(t, err)
	require.Empty(t, recs)
	pages[0].SavingsPlansPurchaseRecommendation.SavingsPlansType = types.SupportedSavingsPlansTypeSagemakerSp
	// Empty datasets have no recommendations to validate, regardless of plan type.
	recs, err = mapSavingsPlansRecommendations(pages)
	require.NoError(t, err)
	require.Empty(t, recs)
	pages[0].SavingsPlansPurchaseRecommendation.SavingsPlansType = ""
	pages[0].SavingsPlansPurchaseRecommendation.SavingsPlansPurchaseRecommendationDetails = []types.SavingsPlansPurchaseRecommendationDetail{{
		AccountId: aws.String("123"), EstimatedMonthlySavingsAmount: aws.String("1"), CurrencyCode: aws.String("USD"),
	}}
	_, err = mapSavingsPlansRecommendations(pages)
	require.Error(t, err)
}
func TestRecommendationMoney(t *testing.T) {
	for _, value := range []string{"", "-1", "no", "NaN", "Inf", "1e309", "0x1p2", "1_000", "+1", " 1"} {
		t.Run(value, func(t *testing.T) {
			p := rightsizingFixture()
			p[0].RightsizingRecommendations[0].ModifyRecommendationDetail.TargetInstances[0].EstimatedMonthlySavings = aws.String(value)
			_, err := mapRightsizingRecommendations(p)
			require.Error(t, err)
		})
	}
	p := rightsizingFixture()
	p[0].RightsizingRecommendations[0].CurrentInstance.CurrencyCode = nil
	_, err := mapRightsizingRecommendations(p)
	require.Error(t, err)
}
func TestRecommendationProjection(t *testing.T) {
	for _, tc := range []struct {
		period string
		want   float64
	}{{"daily", 10}, {"monthly", 300}, {"annual", 3600}} {
		t.Run(tc.period, func(t *testing.T) {
			r := &pbc.Recommendation{Impact: &pbc.RecommendationImpact{EstimatedSavings: 300}, Metadata: map[string]string{"monthly_savings": "300"}}
			require.NoError(t, projectRecommendationSavings([]*pbc.Recommendation{r}, tc.period))
			require.Equal(t, tc.want, r.Impact.EstimatedSavings)
		})
	}
	require.Error(t, projectRecommendationSavings(nil, "weekly"))
}

func TestMapRightsizingRecommendationsMetadata(t *testing.T) {
	pages := rightsizingFixture()
	pages[0].Metadata = &types.RightsizingRecommendationMetadata{GenerationTimestamp: aws.String("2026-10-19T10:00:00-05:00"), RecommendationId: aws.String("set-1")}
	pages[0].RightsizingRecommendations[0].CurrentInstance.Tags = []types.TagValues{{Key: aws.String("team"), Values: []string{"finops"}}}
	recs, err := mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.Equal(t, "finops", recs[0].Resource.Tags["team"])
	require.Equal(t, "2026-10-19T15:00:00Z", recs[0].CreatedAt.AsTime().Format("2006-01-02T15:04:05Z07:00"))
	require.Nil(t, recs[0].Resource.Utilization)
	require.Nil(t, recs[0].GetRightsize().ProjectedUtilization)
	require.Nil(t, recs[0].Impact.ImplementationCost)
	pages[0].Metadata.RecommendationId = aws.String("regenerated-set")
	again, err := mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.Equal(t, recs[0].Id, again[0].Id)
}
func TestMapSavingsPlansRecommendationsMetadata(t *testing.T) {
	pages := []costexplorer.GetSavingsPlansPurchaseRecommendationOutput{{
		Metadata: &types.SavingsPlansPurchaseRecommendationMetadata{GenerationTimestamp: aws.String("2026-10-19T15:00:00Z")},
		SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{SavingsPlansType: types.SupportedSavingsPlansTypeEc2InstanceSp, TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront,
			SavingsPlansPurchaseRecommendationDetails: []types.SavingsPlansPurchaseRecommendationDetail{
				{AccountId: aws.String("123"), RecommendationDetailId: aws.String("aws-id"), CurrencyCode: aws.String("USD"), EstimatedMonthlySavingsAmount: aws.String("300"), HourlyCommitmentToPurchase: aws.String("2.00"), UpfrontCost: aws.String("0"), SavingsPlansDetails: &types.SavingsPlansDetails{Region: aws.String("us-east-1"), InstanceFamily: aws.String("m5")}}}}}}
	recs, err := mapSavingsPlansRecommendations(pages)
	require.NoError(t, err)
	r := recs[0]
	require.Equal(t, "aws-cost-explorer:123:aws-id", r.Id)
	require.Equal(t, "us-east-1", r.Resource.Region)
	require.Equal(t, "m5", r.Resource.Sku)
	require.Equal(t, "2.00", r.Metadata["hourly_commitment"])
	require.Equal(t, "0", r.Metadata["upfront_cost"])
	require.Equal(t, "ONE_YEAR", r.GetCommitment().Term)
	require.Equal(t, "NO_UPFRONT", r.GetCommitment().PaymentOption)
	require.NotNil(t, r.CreatedAt)
	pages[0].SavingsPlansPurchaseRecommendation.SavingsPlansPurchaseRecommendationDetails[0].CurrencyCode = nil
	_, err = mapSavingsPlansRecommendations(pages)
	require.Error(t, err)
}
func TestMapSavingsPlansRecommendationsWithoutAccount(t *testing.T) {
	pages := []costexplorer.GetSavingsPlansPurchaseRecommendationOutput{{
		SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{SavingsPlansType: types.SupportedSavingsPlansTypeComputeSp, AccountScope: types.AccountScopePayer, TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront,
			SavingsPlansPurchaseRecommendationDetails: []types.SavingsPlansPurchaseRecommendationDetail{
				{CurrencyCode: aws.String("USD"), EstimatedMonthlySavingsAmount: aws.String("300"), HourlyCommitmentToPurchase: aws.String("1.5")}}}}}
	recs, err := mapSavingsPlansRecommendations(pages)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, "account:payer:savings-plan", recs[0].Resource.Id)
	require.NotContains(t, recs[0].Metadata, "account_id")
	require.NoError(t, pluginsdk.ValidateRecommendation(recs[0]))
	pages[0].SavingsPlansPurchaseRecommendation.AccountScope = types.AccountScopeLinked
	_, err = mapSavingsPlansRecommendations(pages)
	require.Error(t, err)
}
func TestRecommendationProjectionInvalid(t *testing.T) {
	for _, v := range []string{"", "-1", "NaN", "Inf", "bad", "1e309", "0x1p2", "1_000", "+1", " 1"} {
		t.Run(v, func(t *testing.T) {
			r := &pbc.Recommendation{Impact: &pbc.RecommendationImpact{}, Metadata: map[string]string{"monthly_savings": v}}
			require.Error(t, projectRecommendationSavings([]*pbc.Recommendation{r}, "annual"))
		})
	}
	r := &pbc.Recommendation{Impact: &pbc.RecommendationImpact{}, Metadata: map[string]string{"monthly_savings": "1e308"}}
	require.Error(t, projectRecommendationSavings([]*pbc.Recommendation{r}, "annual"))
}

func TestMapRightsizingDefaultTarget(t *testing.T) {
	pages := rightsizingFixture()
	targets := pages[0].RightsizingRecommendations[0].ModifyRecommendationDetail.TargetInstances
	targets[1] = types.TargetInstance{DefaultTargetInstance: true, EstimatedMonthlySavings: aws.String("5"), ResourceDetails: &types.ResourceDetails{EC2ResourceDetails: &types.EC2ResourceDetails{InstanceType: aws.String("m5.xlarge")}}}
	recs, err := mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, "m5.xlarge", recs[0].GetRightsize().GetRecommendedInstanceType())
	require.Equal(t, 5.0, recs[0].GetImpact().GetEstimatedSavings())
	targets[0].DefaultTargetInstance = true
	recs, err = mapRightsizingRecommendations(pages)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, "m5.medium", recs[0].GetRightsize().GetRecommendedInstanceType())
}

// AWS returns a recommendation object with no plan type and no details when
// it has nothing to recommend. That is an empty result, not invalid data.
func TestMapSavingsPlansRecommendationsAcceptsEmptyResponse(t *testing.T) {
	pages := []costexplorer.GetSavingsPlansPurchaseRecommendationOutput{{
		SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{},
	}}
	recs, err := mapSavingsPlansRecommendations(pages)
	require.NoError(t, err)
	require.Empty(t, recs)
}

func TestMapSavingsPlansRecommendationsStillRejectsUnknownTypeWithDetails(t *testing.T) {
	pages := []costexplorer.GetSavingsPlansPurchaseRecommendationOutput{{
		SavingsPlansPurchaseRecommendation: &types.SavingsPlansPurchaseRecommendation{
			SavingsPlansType: types.SupportedSavingsPlansType("SAGEMAKER_SP"),
			SavingsPlansPurchaseRecommendationDetails: []types.SavingsPlansPurchaseRecommendationDetail{{
				AccountId: aws.String("123"), EstimatedMonthlySavingsAmount: aws.String("1"), CurrencyCode: aws.String("USD"),
			}},
		},
	}}
	_, err := mapSavingsPlansRecommendations(pages)
	require.Error(t, err)
}
