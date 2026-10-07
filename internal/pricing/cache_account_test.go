package pricing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

func TestActualCostCacheSeparatesLinkedAccounts(t *testing.T) {
	first := strings.ReplaceAll(okPage, `"1.00"`, `"10.00"`)
	second := strings.ReplaceAll(okPage, `"1.00"`, `"20.00"`)
	third := strings.ReplaceAll(okPage, `"1.00"`, `"30.00"`)
	f := newFakeCE(t, []json.RawMessage{json.RawMessage(first), json.RawMessage(second), json.RawMessage(third)})
	ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: f.srv.URL, AccessKeyID: "test", SecretAccessKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	calc := NewCalculatorWithClient(ce)
	calc.cache, err = NewCacheManager(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	server := pluginsdk.NewTestServer(t, calc)
	defer server.Close()
	start, end := recentCostWindow()
	arnA := "arn:aws:ec2:us-east-1:111111111111:instance/" + bareInstanceID
	arnB := "arn:aws:ec2:us-east-1:222222222222:instance/" + bareInstanceID
	reqA := costRequest(bareInstanceID, arnA, start, end, nil)
	reqA.BillingAccountId = "caller-a"
	reqB := costRequest(bareInstanceID, arnA, start, end, nil)
	reqB.Resource = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Arn: arnB}
	reqB.BillingAccountId = "caller-b"
	reqC := costRequest(bareInstanceID, "", start, end, nil)
	reqC.BillingAccountId = "caller-c"
	for _, tc := range []struct {
		req  *pbc.GetActualCostRequest
		cost float64
	}{{reqA, 10}, {reqB, 20}, {reqC, 30}, {reqA, 10}, {reqC, 30}} {
		resp, callErr := server.Client().GetActualCost(context.Background(), tc.req)
		if callErr != nil {
			t.Fatal(callErr)
		}
		if len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != tc.cost {
			t.Fatalf("account %s cost=%v want %v", tc.req.GetBillingAccountId(), resp, tc.cost)
		}
		if resp.GetResults()[0].GetFocusRecord().GetBillingAccountId() != tc.req.GetBillingAccountId() {
			t.Fatal("cached billing account differs from caller")
		}
	}
	calls := f.snapshot()
	if len(calls) != 3 {
		t.Fatalf("CE calls=%d want 3", len(calls))
	}
	for i, account := range []string{"111111111111", "222222222222"} {
		if operationName(calls[i].Target) != "GetCostAndUsageWithResources" || !strings.Contains(string(calls[i].Body), `"LINKED_ACCOUNT"`) || !strings.Contains(string(calls[i].Body), account) {
			t.Fatalf("query %d missing account filter: %s", i, calls[i].Body)
		}
	}
	if strings.Contains(string(calls[2].Body), `"LINKED_ACCOUNT"`) {
		t.Fatal("bare instance query unexpectedly filtered by account")
	}
}
