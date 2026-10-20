package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecommendationsAWSContract(t *testing.T) {
	for _, source := range []string{"rightsizing", "COMPUTE_SP", "EC2_INSTANCE_SP"} {
		t.Run(source, func(t *testing.T) {
			calls := 0
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					http.Error(w, "invalid request body", http.StatusBadRequest)
					return
				}
				if !assert.Contains(t, r.Header.Get("Authorization"), "synthetic-access") {
					http.Error(w, "unexpected AWS request", http.StatusBadRequest)
					return
				}
				if !assert.Equal(t, "application/x-amz-json-1.1", r.Header.Get("Content-Type")) {
					http.Error(w, "unexpected AWS request", http.StatusBadRequest)
					return
				}
				if calls == 1 {
					if !assert.NotContains(t, body, "NextPageToken") {
						http.Error(w, "unexpected AWS request", http.StatusBadRequest)
						return
					}
				} else {
					if !assert.Equal(t, "continue", body["NextPageToken"]) {
						http.Error(w, "unexpected AWS request", http.StatusBadRequest)
						return
					}
				}
				if !assert.Equal(t, map[string]any{"Dimensions": map[string]any{"Key": "LINKED_ACCOUNT", "Values": []any{"123456789012"}}}, body["Filter"]) {
					http.Error(w, "unexpected AWS request", http.StatusBadRequest)
					return
				}
				token := ""
				if calls == 1 {
					token = `,"NextPageToken":"continue"`
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				if source == "rightsizing" {
					if !assert.Equal(t, "AWSInsightsIndexService.GetRightsizingRecommendation", r.Header.Get("X-Amz-Target")) {
						http.Error(w, "unexpected AWS request", http.StatusBadRequest)
						return
					}
					if !assert.Equal(t, "AmazonEC2", body["Service"]) {
						http.Error(w, "unexpected AWS request", http.StatusBadRequest)
						return
					}
					if !assert.Equal(t, map[string]any{"BenefitsConsidered": true, "RecommendationTarget": "SAME_INSTANCE_FAMILY"}, body["Configuration"]) {
						http.Error(w, "unexpected AWS request", http.StatusBadRequest)
						return
					}
					_, _ = fmt.Fprintf(w, `{"RightsizingRecommendations":[{"AccountId":"123456789012","RightsizingType":"TERMINATE","CurrentInstance":{"ResourceId":"i-offline","ResourceDetails":{"EC2ResourceDetails":{"InstanceType":"m5.large","Region":"us-east-1"}}},"TerminateRecommendationDetail":{"EstimatedMonthlySavings":"12.50","CurrencyCode":"USD"}}]%s}`, token)
				} else {
					if !assert.Equal(t, "AWSInsightsIndexService.GetSavingsPlansPurchaseRecommendation", r.Header.Get("X-Amz-Target")) {
						http.Error(w, "unexpected AWS request", http.StatusBadRequest)
						return
					}
					for key, value := range map[string]string{"SavingsPlansType": source, "AccountScope": "PAYER", "LookbackPeriodInDays": "THIRTY_DAYS", "TermInYears": "ONE_YEAR", "PaymentOption": "NO_UPFRONT"} {
						if !assert.Equal(t, value, body[key]) {
							http.Error(w, "unexpected AWS request", http.StatusBadRequest)
							return
						}
					}
					_, _ = fmt.Fprintf(w, `{"SavingsPlansPurchaseRecommendation":{"SavingsPlansType":%q,"SavingsPlansPurchaseRecommendationSummary":{"CurrencyCode":"USD"},"SavingsPlansPurchaseRecommendationDetails":[{"AccountId":"123456789012","RecommendationDetailId":"detail","EstimatedMonthlySavingsAmount":"300","HourlyCommitmentToPurchase":"1.25","SavingsPlansDetails":{"Region":"us-east-1","InstanceFamily":"m5"}}]}%s}`, source, token)
				}
			}))
			defer endpoint.Close()
			c, err := NewClient(context.Background(), Config{Region: "us-east-1", BaseEndpoint: endpoint.URL, AccessKeyID: "synthetic-access", SecretAccessKey: "synthetic-secret"})
			require.NoError(t, err)
			filter := &types.Expression{Dimensions: &types.DimensionValues{Key: types.DimensionLinkedAccount, Values: []string{"123456789012"}}}
			if source == "rightsizing" {
				pages, err := c.ListRightsizingRecommendations(context.Background(), costexplorer.GetRightsizingRecommendationInput{Service: aws.String("AmazonEC2"), Filter: filter, Configuration: &types.RightsizingRecommendationConfiguration{BenefitsConsidered: true, RecommendationTarget: types.RecommendationTargetSameInstanceFamily}})
				require.NoError(t, err)
				require.Len(t, pages, 2)
				require.Equal(t, "12.50", aws.ToString(pages[1].RightsizingRecommendations[0].TerminateRecommendationDetail.EstimatedMonthlySavings))
				require.Equal(t, "i-offline", aws.ToString(pages[0].RightsizingRecommendations[0].CurrentInstance.ResourceId))
			} else {
				pages, err := c.ListSavingsPlansRecommendations(context.Background(), costexplorer.GetSavingsPlansPurchaseRecommendationInput{SavingsPlansType: types.SupportedSavingsPlansType(source), AccountScope: types.AccountScopePayer, LookbackPeriodInDays: types.LookbackPeriodInDaysThirtyDays, TermInYears: types.TermInYearsOneYear, PaymentOption: types.PaymentOptionNoUpfront, Filter: filter})
				require.NoError(t, err)
				require.Len(t, pages, 2)
				detail := pages[1].SavingsPlansPurchaseRecommendation.SavingsPlansPurchaseRecommendationDetails[0]
				require.Equal(t, "300", aws.ToString(detail.EstimatedMonthlySavingsAmount))
				require.Equal(t, "m5", aws.ToString(detail.SavingsPlansDetails.InstanceFamily))
				require.Equal(t, "1.25", aws.ToString(detail.HourlyCommitmentToPurchase))
			}
			require.Equal(t, 2, calls)
		})
	}
}
