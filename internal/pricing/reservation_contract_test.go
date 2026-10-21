package pricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the real AWS serializer and deserializer, then the plugin and SDK
// gRPC server. Fixtures contain only synthetic accounts and credentials.
func TestReservationAWSContract(t *testing.T) {
	for _, tc := range []struct {
		kind, service, code, details, sku string
	}{
		{"rds:Account", "Amazon Relational Database Service", "AmazonRDS", `"RDSInstanceDetails":{"InstanceType":"db.m5.large","Region":"us-east-1","DatabaseEngine":"MySQL"}`, "db.m5.large"},
		{"redshift:Account", "Amazon Redshift", "AmazonRedshift", `"RedshiftInstanceDetails":{"NodeType":"dc2.large","Region":"us-east-1"}`, "dc2.large"},
		{"elasticache:Account", "Amazon ElastiCache", "AmazonElastiCache", `"ElastiCacheInstanceDetails":{"NodeType":"cache.m5.large","Region":"us-east-1","ProductDescription":"redis"}`, "cache.m5.large"},
		{"es:Account", "Amazon Elasticsearch Service", "AmazonES", `"ESInstanceDetails":{"InstanceClass":"m5","InstanceSize":"large","Region":"us-east-1"}`, "m5.large"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			calls := 0
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					http.Error(w, "bad JSON", http.StatusBadRequest)
					return
				}
				assert.Equal(t, "AWSInsightsIndexService.GetReservationPurchaseRecommendation", r.Header.Get("X-Amz-Target"))
				assert.Contains(t, r.Header.Get("Authorization"), "synthetic-access")
				for field, value := range map[string]string{"Service": tc.service, "AccountId": "123456789012", "AccountScope": "PAYER", "LookbackPeriodInDays": "THIRTY_DAYS", "TermInYears": "ONE_YEAR", "PaymentOption": "NO_UPFRONT"} {
					assert.Equal(t, value, body[field])
				}
				assert.NotContains(t, body, "Filter")
				assert.NotContains(t, body, "ServiceSpecification")
				next := `,"NextPageToken":"aws-next"`
				if calls == 1 {
					assert.NotContains(t, body, "NextPageToken")
				} else {
					assert.Equal(t, "aws-next", body["NextPageToken"])
					next = ""
				}
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				if calls == 2 {
					_, _ = fmt.Fprint(w, `{"Recommendations":[]}`)
					return
				}
				_, _ = fmt.Fprintf(w, `{"Metadata":{"GenerationTimestamp":"2026-10-20T15:00:00Z","RecommendationId":"set-1"},"Recommendations":[{"AccountScope":"PAYER","LookbackPeriodInDays":"THIRTY_DAYS","TermInYears":"ONE_YEAR","PaymentOption":"NO_UPFRONT","RecommendationSummary":{"CurrencyCode":"USD"},"RecommendationDetails":[{"AccountId":"123456789012","EstimatedMonthlySavingsAmount":"25.50","RecommendedNumberOfInstancesToPurchase":"2","UpfrontCost":"125.75","InstanceDetails":{%s}}]}]%s}`, tc.details, next)
			}))
			defer endpoint.Close()
			ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: endpoint.URL, AccessKeyID: "synthetic-access", SecretAccessKey: "synthetic-secret"})
			require.NoError(t, err)
			srv := pluginsdk.NewTestServer(t, NewCalculatorWithClient(ce))
			defer srv.Close()
			resp, err := srv.Client().GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{ResourceType: tc.kind, AccountId: "123456789012", Region: "us-east-1"}})
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.Len(t, resp.GetRecommendations(), 1)
			rec := resp.GetRecommendations()[0]
			require.NoError(t, pluginsdk.ValidateRecommendation(rec))
			require.Equal(t, tc.kind, rec.GetResource().GetResourceType())
			require.Equal(t, tc.sku, rec.GetResource().GetSku())
			require.Equal(t, tc.code, rec.GetMetadata()["service"])
			require.Equal(t, "125.75", rec.GetMetadata()["upfront_cost"])
			require.Equal(t, 25.5, rec.GetImpact().GetEstimatedSavings())
			require.Equal(t, 2.0, rec.GetCommitment().GetRecommendedQuantity())
			require.NoError(t, pluginsdk.ValidateRecommendationSummary(resp.GetSummary()))
		})
	}
}
