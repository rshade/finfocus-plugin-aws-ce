package integration

import (
	"connectrpc.com/connect"
	"context"
	"encoding/json"
	pbcconnect "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1/pbcconnect"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/pricing"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func historicalQuery() *pbc.GetCarbonEmissionsRequest {
	return &pbc.GetCarbonEmissionsRequest{Start: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), End: timestamppb.New(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))}
}
func carbonEndpoint(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var count atomic.Int32
	fixture, err := os.ReadFile("../../internal/client/testdata/carbon/monthly.json")
	if err != nil {
		t.Fatal(err)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if !strings.Contains(r.Header.Get("Authorization"), "fixture-key/") {
			t.Error("missing fixture signing key")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["Granularity"] != "MONTHLY" {
			t.Errorf("wrong grouping: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	t.Cleanup(fake.Close)
	return fake.URL, &count
}
func TestCarbonSubprocess(t *testing.T) {
	endpoint, calls := carbonEndpoint(t)
	for _, enabled := range []string{"", "false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			proc := testutil.StartPlugin(t, map[string]string{"AWS_ENDPOINT_URL_SUSTAINABILITY": endpoint, "AWS_ACCESS_KEY_ID": "fixture-key", "AWS_SECRET_ACCESS_KEY": "fixture-secret", "AWS_SESSION_TOKEN": "", "AWS_PROFILE": "", "AWS_REGION": "us-east-1", "AWS_EC2_METADATA_DISABLED": "true", "FINFOCUS_AWS_CARBON_ENABLED": enabled, "FINFOCUS_LOG_FILE": "", "FINFOCUS_PLUGIN_PORT": "0"}, "--port", "0")
			before := calls.Load()
			response, err := proc.Client.GetCarbonEmissions(context.Background(), historicalQuery())
			if enabled != "true" {
				if status.Code(err) != codes.FailedPrecondition || calls.Load() != before {
					t.Fatalf("disabled: %v calls %d", err, calls.Load())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(response.Records) != 1 || response.Records[0].GetValue() != 1250000 || response.Records[0].ModelVersion != "2.0.0" || response.NextPageToken == "aws-next" {
				t.Fatalf("response: %v", response)
			}
			if calls.Load() != before+1 {
				t.Fatalf("calls %d", calls.Load())
			}
		})
	}
}
func TestCarbonConnectOfficialJSON(t *testing.T) {
	endpoint, _ := carbonEndpoint(t)
	t.Setenv("AWS_ENDPOINT_URL_SUSTAINABILITY", endpoint)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "fixture-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	handler := pluginsdk.NewConnectHandler(pluginsdk.NewServer(pricing.NewCalculator(pricing.WithCarbonEnabled(true))))
	_, httpHandler := pbcconnect.NewCostSourceServiceHandler(handler)
	server := httptest.NewServer(httpHandler)
	defer server.Close()
	client := pbcconnect.NewCostSourceServiceClient(server.Client(), server.URL)
	response, err := client.GetCarbonEmissions(context.Background(), connect.NewRequest(historicalQuery()))
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Msg.Records) != 1 || response.Msg.Records[0].GetValue() != 1250000 {
		t.Fatalf("response: %v", response)
	}
}
