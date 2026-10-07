package conformance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestActualCostProtocol(t *testing.T) {
	id := fmt.Sprintf("i-%017x", time.Now().UnixNano())
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Amz-Target") != "AWSInsightsIndexService.GetCostAndUsageWithResources" {
			t.Errorf("operation=%s", r.Header.Get("X-Amz-Target"))
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = fmt.Fprintf(w, `{"ResultsByTime":[{"TimePeriod":{"Start":"2026-09-20","End":"2026-09-21"},"Groups":[{"Keys":["%s"],"Metrics":{"UnblendedCost":{"Amount":"1.75","Unit":"USD"}}}]}]}`, id)
	}))
	defer fake.Close()
	server := testutil.StartPlugin(t, map[string]string{
		"AWS_ENDPOINT_URL_COST_EXPLORER": fake.URL, "AWS_ENDPOINT_URL": fake.URL,
		"AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "false", "AWS_REGION": "us-east-1",
		"AWS_PROFILE": "", "AWS_ACCESS_KEY_ID": "test", "AWS_SECRET_ACCESS_KEY": "test",
		"AWS_SESSION_TOKEN": "", "AWS_EC2_METADATA_DISABLED": "true",
		"FINFOCUS_LOG_FILE": "", "FINFOCUS_PLUGIN_PORT": "0",
		"FINFOCUS_AWS_CE_MAX_BATCH_SIZE": "100", "FINFOCUS_AWS_CE_BATCH_WORKERS": "10",
	}, "--port", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Explicit request credentials bypass persistent cache reads and writes.
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pluginsdk.CredentialMetadataPrefix+"access_key_id", "test", pluginsdk.CredentialMetadataPrefix+"secret_access_key", "test"))
	start := time.Now().UTC().Add(-48 * time.Hour)
	req := &pbc.GetActualCostRequest{ResourceId: id, Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: id}, Start: timestamppb.New(start), End: timestamppb.New(start.Add(24 * time.Hour)), BillingAccountId: "conformance-account"}
	wire, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	restored := &pbc.GetActualCostRequest{}
	if err := proto.Unmarshal(wire, restored); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(req, restored) {
		t.Fatalf("request lost fields: %v", restored)
	}
	response, err := server.Client.GetActualCost(ctx, restored)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.GetResults()) != 1 {
		t.Fatalf("results=%v", response)
	}
	row := response.GetResults()[0]
	if row.GetCost() != 1.75 || row.GetSource() == "" || row.GetTimestamp() == nil || !row.GetTimestamp().IsValid() || row.GetExpiresAt() == nil || !row.GetExpiresAt().IsValid() {
		t.Fatalf("missing required cost/source/timestamp or cache hint: %v", row)
	}
	if row.GetFocusRecord().GetBillingAccountId() != "conformance-account" || row.GetFocusRecord().GetBillingCurrency() != "USD" {
		t.Fatalf("FOCUS financial fields=%v", row.GetFocusRecord())
	}
	if err := pluginsdk.ValidateFocusRecord(row.GetFocusRecord()); err != nil {
		t.Fatal(err)
	}
	wire, err = proto.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	restoredResponse := &pbc.GetActualCostResponse{}
	if err := proto.Unmarshal(wire, restoredResponse); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(response, restoredResponse) {
		t.Fatal("response lost fields on protobuf roundtrip")
	}
	req.BillingAccountId = ""
	req.Resource = nil
	noAccount, err := server.Client.GetActualCost(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if noAccount.GetResults()[0].GetFocusRecord() != nil || noAccount.GetResults()[0].GetCost() != 1.75 {
		t.Fatalf("optional descriptor/account response=%v", noAccount)
	}
	if calls.Load() != 2 {
		t.Fatalf("per-request calls=%d, want two uncached requests", calls.Load())
	}
	before := calls.Load()
	for _, tc := range []struct {
		name   string
		mutate func(*pbc.GetActualCostRequest)
		want   pbc.ErrorCode
	}{
		{"empty resource", func(r *pbc.GetActualCostRequest) { r.ResourceId = "" }, pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE},
		{"missing start", func(r *pbc.GetActualCostRequest) { r.Start = nil }, pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE},
		{"missing end", func(r *pbc.GetActualCostRequest) { r.End = nil }, pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE},
		{"reversed range", func(r *pbc.GetActualCostRequest) { r.Start = r.End }, pbc.ErrorCode_ERROR_CODE_INVALID_TIME_RANGE},
		{"unsupported provider", func(r *pbc.GetActualCostRequest) {
			r.Resource = &pbc.ResourceDescriptor{Provider: "azure", ResourceType: "vm", Id: "vm-id"}
		}, pbc.ErrorCode_ERROR_CODE_INVALID_RESOURCE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invalid := proto.Clone(req).(*pbc.GetActualCostRequest)
			tc.mutate(invalid)
			_, err := server.Client.GetActualCost(ctx, invalid)
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("gRPC code=%v want InvalidArgument: %v", status.Code(err), err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("details=%v", details)
			}
			detail, ok := details[0].(*pbc.ErrorDetail)
			if !ok || detail.GetCode() != tc.want || detail.GetMessage() == "" {
				t.Fatalf("proto error=%v want %v", details, tc.want)
			}
		})
	}
	if calls.Load() != before {
		t.Fatal("invalid requests reached Cost Explorer")
	}
}
