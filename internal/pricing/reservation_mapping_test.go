package pricing

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
)

func reservationFixture() []costexplorer.GetReservationPurchaseRecommendationOutput {
	return []costexplorer.GetReservationPurchaseRecommendationOutput{{
		Metadata: &types.ReservationPurchaseRecommendationMetadata{GenerationTimestamp: aws.String("2026-10-20T15:00:00Z"), RecommendationId: aws.String("set-1")},
		Recommendations: []types.ReservationPurchaseRecommendation{{
			AccountScope: types.AccountScopePayer, LookbackPeriodInDays: types.LookbackPeriodInDaysThirtyDays,
			TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront,
			RecommendationSummary: &types.ReservationPurchaseRecommendationSummary{CurrencyCode: aws.String("USD")},
			RecommendationDetails: []types.ReservationPurchaseRecommendationDetail{{
				AccountId: aws.String("123456789012"), EstimatedMonthlySavingsAmount: aws.String("25.50"),
				RecommendedNumberOfInstancesToPurchase: aws.String("2"), UpfrontCost: aws.String("125.75"),
				EstimatedBreakEvenInMonths: aws.String("4.5"), RecurringStandardMonthlyCost: aws.String("70"),
				InstanceDetails: &types.InstanceDetails{RDSInstanceDetails: &types.RDSInstanceDetails{
					InstanceType: aws.String("db.m5.large"), Region: aws.String("us-east-1"),
					DatabaseEngine: aws.String("MySQL"), DeploymentOption: aws.String("Single-AZ"),
				}},
			}},
		}},
	}}
}

func TestMapReservationRecommendations(t *testing.T) {
	for _, tc := range []struct {
		code, resourceType, sku string
		details                 *types.InstanceDetails
	}{
		{"AmazonRDS", "rds:Account", "db.m5.large", reservationFixture()[0].Recommendations[0].RecommendationDetails[0].InstanceDetails},
		{"AmazonRedshift", "redshift:Account", "dc2.large", &types.InstanceDetails{RedshiftInstanceDetails: &types.RedshiftInstanceDetails{NodeType: aws.String("dc2.large"), Region: aws.String("us-east-1")}}},
		{"AmazonElastiCache", "elasticache:Account", "cache.m5.large", &types.InstanceDetails{ElastiCacheInstanceDetails: &types.ElastiCacheInstanceDetails{NodeType: aws.String("cache.m5.large"), Region: aws.String("us-east-1")}}},
		{"AmazonES", "es:Account", "m5.large", &types.InstanceDetails{ESInstanceDetails: &types.ESInstanceDetails{InstanceClass: aws.String("m5"), InstanceSize: aws.String("large"), Region: aws.String("us-east-1")}}},
	} {
		t.Run(tc.code, func(t *testing.T) {
			pages := reservationFixture()
			pages[0].Recommendations[0].RecommendationDetails[0].InstanceDetails = tc.details
			recs, err := mapReservationRecommendations(tc.code, pages)
			require.NoError(t, err)
			require.Len(t, recs, 1)
			r := recs[0]
			require.NoError(t, pluginsdk.ValidateRecommendation(r))
			require.Equal(t, pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT, r.ActionType)
			require.Equal(t, "reserved_instance", r.GetCommitment().GetCommitmentType())
			require.Equal(t, 2.0, r.GetCommitment().GetRecommendedQuantity())
			require.Equal(t, "ONE_YEAR", r.GetCommitment().GetTerm())
			require.Equal(t, "NO_UPFRONT", r.GetCommitment().GetPaymentOption())
			require.Equal(t, "account", r.GetCommitment().GetScope())
			require.Equal(t, tc.resourceType, r.Resource.ResourceType)
			require.Equal(t, tc.sku, r.Resource.Sku)
			require.Equal(t, "us-east-1", r.Resource.Region)
			require.Equal(t, "account:123456789012:reserved-instance:"+tc.code, r.Resource.Id)
			require.Equal(t, tc.code, r.Metadata["service"])
			require.Equal(t, "25.50", r.Metadata["monthly_savings"])
			require.Equal(t, "125.75", r.Metadata["upfront_cost"])
			require.Equal(t, "4.5", r.Metadata["estimated_break_even_months"])
			require.Equal(t, 25.5, r.Impact.EstimatedSavings)
			require.Equal(t, "USD", r.Impact.Currency)
			require.Equal(t, "monthly", r.Impact.ProjectionPeriod)
			require.NotNil(t, r.CreatedAt)
			require.Nil(t, r.ConfidenceScore)
		})
	}
}

func TestReservationRecommendationIdentity(t *testing.T) {
	pages := reservationFixture()
	first, err := mapReservationRecommendations("AmazonRDS", pages)
	require.NoError(t, err)
	pages[0].Metadata.GenerationTimestamp = aws.String("2026-10-21T15:00:00Z")
	again, err := mapReservationRecommendations("AmazonRDS", pages)
	require.NoError(t, err)
	require.Equal(t, first[0].Id, again[0].Id)
	row := pages[0].Recommendations[0].RecommendationDetails[0]
	row.InstanceDetails = &types.InstanceDetails{RDSInstanceDetails: &types.RDSInstanceDetails{InstanceType: aws.String("db.m5.large"), Region: aws.String("us-east-1"), DatabaseEngine: aws.String("PostgreSQL")}}
	pages[0].Recommendations[0].RecommendationDetails = append(pages[0].Recommendations[0].RecommendationDetails, row)
	recs, err := mapReservationRecommendations("AmazonRDS", pages)
	require.NoError(t, err)
	require.NotEqual(t, recs[0].Id, recs[1].Id, "different database engines must remain distinct")
	pages[0].Recommendations[0].RecommendationDetails[0], pages[0].Recommendations[0].RecommendationDetails[1] = row, pages[0].Recommendations[0].RecommendationDetails[0]
	reordered, err := mapReservationRecommendations("AmazonRDS", pages)
	require.NoError(t, err)
	require.Equal(t, recs[0].Id, reordered[1].Id)
}

func TestReservationRecommendationInvalidData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*types.ReservationPurchaseRecommendationDetail)
	}{
		{"missing instance details", func(d *types.ReservationPurchaseRecommendationDetail) { d.InstanceDetails = nil }},
		{"wrong service details", func(d *types.ReservationPurchaseRecommendationDetail) {
			d.InstanceDetails = &types.InstanceDetails{EC2InstanceDetails: &types.EC2InstanceDetails{}}
		}},
		{"missing type", func(d *types.ReservationPurchaseRecommendationDetail) {
			d.InstanceDetails.RDSInstanceDetails.InstanceType = nil
		}},
		{"missing region", func(d *types.ReservationPurchaseRecommendationDetail) {
			d.InstanceDetails.RDSInstanceDetails.Region = nil
		}},
		{"negative savings", func(d *types.ReservationPurchaseRecommendationDetail) {
			d.EstimatedMonthlySavingsAmount = aws.String("-1")
		}},
		{"nonfinite quantity", func(d *types.ReservationPurchaseRecommendationDetail) {
			d.RecommendedNumberOfInstancesToPurchase = aws.String("NaN")
		}},
		{"missing quantity", func(d *types.ReservationPurchaseRecommendationDetail) { d.RecommendedNumberOfInstancesToPurchase = nil }},
		{"negative upfront", func(d *types.ReservationPurchaseRecommendationDetail) { d.UpfrontCost = aws.String("-1") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pages := reservationFixture()
			tc.mutate(&pages[0].Recommendations[0].RecommendationDetails[0])
			_, err := mapReservationRecommendations("AmazonRDS", pages)
			require.Error(t, err)
		})
	}
}

func TestReservationRecommendationEmptyAndPayer(t *testing.T) {
	for _, pages := range [][]costexplorer.GetReservationPurchaseRecommendationOutput{nil, {{}}, {{Recommendations: []types.ReservationPurchaseRecommendation{{}}}}} {
		recs, err := mapReservationRecommendations("AmazonRDS", pages)
		require.NoError(t, err)
		require.Empty(t, recs)
	}
	pages := reservationFixture()
	pages[0].Recommendations[0].RecommendationDetails[0].AccountId = nil
	recs, err := mapReservationRecommendations("AmazonRDS", pages)
	require.NoError(t, err)
	require.Equal(t, "account:payer:reserved-instance:AmazonRDS", recs[0].Resource.Id)
	require.NotContains(t, recs[0].Metadata, "account_id")
	pages[0].Recommendations[0].AccountScope = types.AccountScopeLinked
	_, err = mapReservationRecommendations("AmazonRDS", pages)
	require.Error(t, err)
}
