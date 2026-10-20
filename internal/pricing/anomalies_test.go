package pricing

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const anomalyPage = `{"Anomalies":[{"AnomalyId":"a-1","AnomalyScore":{"CurrentScore":0.8,"MaxScore":0.9},"Impact":{"MaxImpact":12,"TotalImpact":10,"TotalActualSpend":40,"TotalExpectedSpend":30},"MonitorArn":"arn:aws:ce::123456789012:anomalymonitor/m","AnomalyEndDate":"2026-10-02","DimensionValue":"Amazon Simple Storage Service","RootCauses":[{"Service":"Amazon Simple Storage Service","Region":"us-east-1"}]}],"NextPageToken":"page-2"}`

func TestGetRecommendationsAnomalies(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if strings.Contains(string(body), "page-2") {
			_, _ = w.Write([]byte(`{"Anomalies":[]}`))
			return
		}
		_, _ = w.Write([]byte(anomalyPage))
	}))
	t.Cleanup(server.Close)
	ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculatorWithClient(ce)

	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetRecommendations()) != 1 || resp.GetNextPageToken() != "page-2" {
		t.Fatalf("resp=%v", resp)
	}
	rec := resp.GetRecommendations()[0]
	if rec.GetId() != "a-1" || rec.GetCategory() != pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY || rec.GetActionType() != pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_INVESTIGATE {
		t.Fatalf("recommendation=%v", rec)
	}
	if rec.GetImpact().GetCurrentCost() != 40 || rec.GetMetadata()["total_impact"] != "10" {
		t.Fatalf("impact=%v metadata=%v", rec.GetImpact(), rec.GetMetadata())
	}
	if rec.GetConfidenceScore() != 0.8 {
		t.Fatalf("confidence=%v", rec.GetConfidenceScore())
	}

	next, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageToken: resp.GetNextPageToken()})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.GetRecommendations()) != 0 || next.GetNextPageToken() != "" {
		t.Fatalf("next=%v", next)
	}
	if len(bodies) != 2 || !strings.Contains(bodies[1], "page-2") {
		t.Fatalf("bodies=%v", bodies)
	}
}

func TestGetRecommendationsEmptyAnomalies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"Anomalies":[]}`))
	}))
	t.Cleanup(server.Close)
	ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: server.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculatorWithClient(ce)
	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		Filter: &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_ANOMALY},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetRecommendations()) != 0 || resp.GetSummary().GetTotalRecommendations() != 0 {
		t.Fatalf("resp=%v", resp)
	}
}

func TestGetRecommendationsSkipsNonAnomalyCategory(t *testing.T) {
	calc := NewCalculator()
	resp, err := calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		Filter: &pbc.RecommendationFilter{Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetRecommendations()) != 0 {
		t.Fatalf("resp=%v", resp)
	}
	_, err = calc.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		TargetResources: []*pbc.ResourceDescriptor{{Provider: "aws", ResourceType: "aws:s3/bucket:Bucket"}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("target resources: %v", err)
	}
}
