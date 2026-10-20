package pricing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestActualCostEC2FocusIdentity(t *testing.T) {
	page := strings.ReplaceAll(okPage, `"Amazon Simple Storage Service"`, `"`+bareInstanceID+`"`)
	f := newFakeCE(t, []json.RawMessage{json.RawMessage(page)})
	ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: f.srv.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	server := pluginsdk.NewTestServer(t, NewCalculatorWithClient(ce))
	defer server.Close()
	start, end := recentCostWindow()
	req := costRequest(bareInstanceID, resourceQueryEC2ARN, start, end, nil)
	req.BillingAccountId = "123456789012"
	// Both the fresh response and cache hit must retain the same identities.
	for range 2 {
		resp, callErr := server.Client().GetActualCost(context.Background(), req)
		if callErr != nil {
			t.Fatal(callErr)
		}
		record := resp.GetResults()[0].GetFocusRecord()
		if record.GetServiceName() != "Amazon Elastic Compute Cloud - Compute" {
			t.Errorf("service name = %q", record.GetServiceName())
		}
		if record.GetResourceId() != bareInstanceID || record.GetExtendedColumns()["group_key"] != bareInstanceID {
			t.Errorf("lost EC2 identity: %v", record)
		}
		if err := pluginsdk.ValidateFocusRecord(record); err != nil {
			t.Fatal(err)
		}
	}
}

func TestActualCostRejectsInvalidFocusInput(t *testing.T) {
	for _, tc := range []struct{ name, page, resourceID string }{
		{"missing currency", strings.ReplaceAll(okPage, `"Unit":"USD"`, `"Unit":""`), "contract-focus"},
		{"missing group identity", strings.ReplaceAll(okPage, `"Keys":["Amazon Simple Storage Service"]`, `"Keys":[]`), "contract-focus"},
		{"missing EC2 group identity", strings.ReplaceAll(okPage, `"Keys":["Amazon Simple Storage Service"]`, `"Keys":[]`), bareInstanceID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeCE(t, []json.RawMessage{json.RawMessage(tc.page)})
			ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: f.srv.URL, AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			server := pluginsdk.NewTestServer(t, NewCalculatorWithClient(ce))
			defer server.Close()
			start, end := recentCostWindow()
			req := costRequest(tc.resourceID, "", start, end, nil)
			req.BillingAccountId = "123456789012"
			resp, err := server.Client().GetActualCost(context.Background(), req)
			if status.Code(err) != codes.Internal || resp != nil {
				t.Fatalf("invalid FOCUS data must fail: response=%v error=%v", resp, err)
			}
			if !strings.Contains(status.Convert(err).Message(), "FOCUS") {
				t.Fatalf("missing translation error context: %v", err)
			}
		})
	}
}
