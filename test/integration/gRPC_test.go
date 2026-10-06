package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPluginSubprocessGRPC(t *testing.T) {
	var requests atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("X-Amz-Target") != "AWSInsightsIndexService.GetCostAndUsageWithResources" {
			t.Errorf("unexpected CE operation %s", r.Header.Get("X-Amz-Target"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		wire, _ := json.Marshal(body)
		if !strings.Contains(string(wire), "i-0abc123def4567890") || strings.Contains(string(wire), "instance/") {
			t.Errorf("wrong resource filter: %s", wire)
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"Estimated":true,"TimePeriod":{"Start":"2026-09-20","End":"2026-09-21"},"Groups":[{"Keys":["i-0abc123def4567890"],"Metrics":{"UnblendedCost":{"Amount":"2.50","Unit":"USD"}}}]}]}`))
	}))
	defer fake.Close()
	env := map[string]string{"AWS_ENDPOINT_URL_COST_EXPLORER": fake.URL, "AWS_ENDPOINT_URL": fake.URL, "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "false", "AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test", "AWS_SESSION_TOKEN": "", "AWS_PROFILE": "", "AWS_REGION": "us-east-1", "AWS_EC2_METADATA_DISABLED": "true", "FINFOCUS_LOG_FILE": "", "FINFOCUS_PLUGIN_PORT": "0", "FINFOCUS_LOG_LEVEL": "debug", "FINFOCUS_AWS_CE_MAX_BATCH_SIZE": "2", "FINFOCUS_AWS_CE_BATCH_WORKERS": "1"}
	proc := testutil.StartPlugin(t, env, "--port", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := proc.Client.GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetName() != "aws-ce" || info.GetSpecVersion() != "v0.7.5" {
		t.Fatalf("plugin info: %v", info)
	}
	supported, err := proc.Client.Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: "i-0abc123def4567890"}})
	if err != nil || !supported.GetSupported() {
		t.Fatalf("aws Supports=%v,%v", supported, err)
	}
	unsupported, err := proc.Client.Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "azure", ResourceType: "vm", Id: "vm-id"}})
	if err != nil || unsupported.GetSupported() || unsupported.GetReason() == "" {
		t.Fatalf("azure Supports=%v,%v", unsupported, err)
	}
	trace := "1234567890abcdef1234567890abcdef"
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pluginsdk.TraceIDMetadataKey, trace, pluginsdk.CredentialMetadataPrefix+"access_key_id", "test", pluginsdk.CredentialMetadataPrefix+"secret_access_key", "test"))
	end := time.Now().UTC().Add(-24 * time.Hour)
	start := end.Add(-24 * time.Hour)
	req := &pbc.GetActualCostRequest{ResourceId: "legacy-id", Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: "i-0abc123def4567890"}, Start: timestamppb.New(start), End: timestamppb.New(end), BillingAccountId: "integration-account"}
	resp, err := proc.Client.GetActualCost(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 2.5 {
		t.Fatalf("actual costs=%v", resp)
	}
	if resp.GetResults()[0].GetFocusRecord().GetExtendedColumns()["estimated"] != "true" {
		t.Fatal("estimated metadata missing")
	}
	if requests.Load() != 1 {
		t.Fatalf("CE requests=%d", requests.Load())
	}
	batch, err := proc.Client.BatchCost(ctx, &pbc.BatchCostRequest{})
	if err != nil || batch.GetMaxBatchSize() != 2 {
		t.Fatalf("batch config=%v,%v", batch, err)
	}
	if err := proc.Stop(); err != nil {
		t.Fatalf("graceful shutdown: %v\n%s", err, proc.Logs())
	}
	if !strings.Contains(proc.Logs(), `"trace_id":"`+trace+`"`) {
		t.Fatalf("trace missing in plugin logs:\n%s", proc.Logs())
	}
	t.Logf("server %s: Supports AWS=true Azure=false; GetPluginInfo name=%s spec=%s; actual cost=2.50; batch limit=2; trace propagated; shutdown clean", proc.Address, info.GetName(), info.GetSpecVersion())
}
