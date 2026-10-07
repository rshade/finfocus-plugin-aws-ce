package pricing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestActualCostMetadata(t *testing.T) {
	field := (&pbc.FocusCostRecord{}).ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name("extended_columns"))
	if field == nil || !field.IsMap() {
		t.Fatal("spec has no FOCUS extended_columns map")
	}
	if (&pbc.ActualCostResult{}).ProtoReflect().Descriptor().Fields().ByName("metadata") != nil {
		t.Fatal("unexpected response metadata carrier")
	}
	start, end := recentCostWindow()
	for _, tc := range []struct{ id, lookback string }{{"contract-metadata", "14_months"}, {bareInstanceID, "14_days"}} {
		t.Run(tc.lookback, func(t *testing.T) {
			page := strings.ReplaceAll(okPage, `"Estimated":false`, `"Estimated":true`)
			f := newFakeCE(t, []json.RawMessage{json.RawMessage(page)})
			ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: f.srv.URL, AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			calc := NewCalculatorWithClient(ce)
			calc.cache = nil
			server := pluginsdk.NewTestServer(t, calc)
			defer server.Close()
			req := costRequest(tc.id, "", start, end, nil)
			req.BillingAccountId = "metadata-account"
			resp, err := server.Client().GetActualCost(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			record := resp.GetResults()[0].GetFocusRecord()
			if err := pluginsdk.ValidateFocusRecord(record); err != nil {
				t.Fatalf("invalid FOCUS record: %v", err)
			}
			for key, want := range map[string]string{"data_source": "AWS Cost Explorer", "granularity": "DAILY", "metric": "UnblendedCost", "estimated": "true", "lookback": tc.lookback} {
				if got := record.GetExtendedColumns()[key]; got != want {
					t.Errorf("%s=%q want=%q", key, got, want)
				}
			}
			if record.GetBillingCurrency() != "USD" {
				t.Fatalf("currency=%q", record.GetBillingCurrency())
			}
		})
	}
}
