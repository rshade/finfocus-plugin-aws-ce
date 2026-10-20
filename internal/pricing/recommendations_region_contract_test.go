package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the documented regional request through the handler and real AWS SDK.
func TestRecommendationsRegionalAWSContract(t *testing.T) {
	for _, account := range []string{"", "123"} {
		t.Run("account="+account, func(t *testing.T) {
			calls := 0
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					http.Error(w, "bad body", http.StatusBadRequest)
					return
				}
				assert.Equal(t, "AWSInsightsIndexService.GetRightsizingRecommendation", r.Header.Get("X-Amz-Target"))
				if account == "" {
					assert.NotContains(t, body, "Filter")
				} else {
					assert.Equal(t, map[string]any{"Dimensions": map[string]any{"Key": "LINKED_ACCOUNT", "Values": []any{account}}}, body["Filter"])
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				_, _ = fmt.Fprint(w, `{"RightsizingRecommendations":[{"AccountId":"123","RightsizingType":"TERMINATE","CurrentInstance":{"ResourceId":"i-east","ResourceDetails":{"EC2ResourceDetails":{"InstanceType":"m5.large","Region":"us-east-1"}}},"TerminateRecommendationDetail":{"EstimatedMonthlySavings":"12.50","CurrencyCode":"USD"}},{"AccountId":"123","RightsizingType":"TERMINATE","CurrentInstance":{"ResourceId":"i-west","ResourceDetails":{"EC2ResourceDetails":{"InstanceType":"m5.large","Region":"us-west-2"}}},"TerminateRecommendationDetail":{"EstimatedMonthlySavings":"30","CurrencyCode":"USD"}}]}`)
			}))
			defer endpoint.Close()
			ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: endpoint.URL, AccessKeyID: "synthetic-access", SecretAccessKey: "synthetic-secret"})
			require.NoError(t, err)
			result, err := NewCalculatorWithClient(ce).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_TERMINATE, AccountId: account, Region: "us-east-1"}, ProjectionPeriod: "annual"})
			require.NoError(t, err)
			require.Len(t, result.Recommendations, 1)
			require.Equal(t, "i-east", result.Recommendations[0].Resource.Id)
			require.Equal(t, "us-east-1", result.Recommendations[0].Resource.Region)
			require.Equal(t, 150.0, result.Recommendations[0].Impact.EstimatedSavings)
			require.Equal(t, 1, calls)
		})
	}
}
