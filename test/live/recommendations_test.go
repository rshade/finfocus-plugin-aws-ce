//go:build awslive

package live

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/status"
)

// TestAWSRecommendations reads existing datasets only; it never generates or purchases plans.
func TestAWSRecommendations(t *testing.T) {
	if os.Getenv("FINFOCUS_AWS_LIVE") != "true" {
		t.Skip("Set FINFOCUS_AWS_LIVE=true with the awslive build tag for AWS recommendation checks")
	}
	for _, key := range []string{"AWS_ENDPOINT_URL", "AWS_ENDPOINT_URL_COST_EXPLORER", "AWS_ENDPOINT_URL_STS"} {
		t.Setenv(key, "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	proc := testutil.StartPluginWithTimeout(t, 4*time.Minute, map[string]string{
		"AWS_REGION": "us-east-1", "AWS_DEFAULT_REGION": "us-east-1",
		"AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "true", "AWS_EC2_METADATA_DISABLED": "true",
		"FINFOCUS_LOG_LEVEL": "error", "FINFOCUS_LOG_FILE": "", "FINFOCUS_PLUGIN_PORT": "0",
	}, "--port", "0")
	// Separate queries explicitly distinguish empty datasets from untested sources.
	for _, tc := range []struct {
		name, resourceType string
		action             pbc.RecommendationActionType
	}{
		{"rightsizing", "", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_RIGHTSIZE},
		{"terminate", "", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_TERMINATE},
		{"all commitments", "", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT},
		{"RDS reservations", "rds:Account", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT},
		{"Redshift reservations", "redshift:Account", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT},
		{"ElastiCache reservations", "elasticache:Account", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT},
		{"OpenSearch reservations", "es:Account", pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_PURCHASE_COMMITMENT},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &pbc.GetRecommendationsRequest{Filter: &pbc.RecommendationFilter{ActionType: tc.action, ResourceType: tc.resourceType}, PageSize: 1000}
			count := 0
			tokens := map[string]bool{}
			for {
				resp, err := proc.Client.GetRecommendations(ctx, req)
				if err != nil {
					t.Fatalf("recommendation query failed: %s", status.Code(err))
				}
				if resp.GetSummary() == nil {
					t.Fatal("live recommendation response has no summary")
				}
				if err := pluginsdk.ValidateRecommendationSummary(resp.GetSummary()); err != nil {
					t.Fatal("invalid live recommendation summary")
				}
				for _, rec := range resp.GetRecommendations() {
					if err := pluginsdk.ValidateRecommendation(rec); err != nil {
						t.Fatal("invalid live recommendation record")
					}
					if rec.GetActionType() != tc.action || rec.GetSource() != "aws-cost-explorer" || rec.GetResource().GetProvider() != "aws" {
						t.Fatal("live recommendation identity or action differs")
					}
					if tc.resourceType != "" && (rec.GetResource().GetResourceType() != tc.resourceType || rec.GetCommitment().GetCommitmentType() != "reserved_instance") {
						t.Fatal("live reservation service or commitment type differs")
					}
				}
				count += len(resp.GetRecommendations())
				next := resp.GetNextPageToken()
				if next == "" {
					break
				}
				if tokens[next] {
					t.Fatal("repeated recommendation page token")
				}
				tokens[next] = true
				req.PageToken = next
			}
			if count == 0 {
				t.Log("AWS returned an empty matching dataset; record mapping not live-verified")
			} else {
				t.Logf("validated %d existing AWS recommendations", count)
			}
		})
	}
	if err := proc.Stop(); err != nil {
		t.Fatal("plugin shutdown failed")
	}
}
