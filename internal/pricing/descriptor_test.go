package pricing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestActualCostResourceDescriptor(t *testing.T) {
	start, end := recentCostWindow()
	for _, tc := range []struct {
		name     string
		id, arn  string
		resource *pbc.ResourceDescriptor
		want     string
		code     codes.Code
	}{
		{name: "legacy fallback", id: bareInstanceID, want: bareInstanceID},
		{name: "descriptor id wins", id: "i-11111111", arn: "arn:aws:s3:::old", resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: bareInstanceID}, want: bareInstanceID},
		{name: "descriptor arn wins", id: "i-11111111", resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Arn: resourceQueryEC2ARN}, want: bareInstanceID},
		{name: "descriptor without identity uses legacy", id: bareInstanceID, resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2"}, want: bareInstanceID},
		{name: "unsupported provider", id: bareInstanceID, resource: &pbc.ResourceDescriptor{Provider: "azure", ResourceType: "vm", Id: bareInstanceID}, code: codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeCE(t, []json.RawMessage{json.RawMessage(okPage)})
			ce, err := client.NewClient(context.Background(), client.Config{Region: "us-east-1", BaseEndpoint: f.srv.URL, AccessKeyID: "test", SecretAccessKey: "test"})
			if err != nil {
				t.Fatal(err)
			}
			calc := NewCalculatorWithClient(ce)
			calc.cache = nil
			server := pluginsdk.NewTestServer(t, calc)
			defer server.Close()
			req := costRequest(tc.id, tc.arn, start, end, nil)
			req.Resource = tc.resource
			_, err = server.Client().GetActualCost(context.Background(), req)
			if status.Code(err) != tc.code {
				t.Fatalf("code=%s want=%s: %v", status.Code(err), tc.code, err)
			}
			calls := f.snapshot()
			if tc.code != codes.OK {
				if len(calls) != 0 {
					t.Fatal("invalid descriptor reached CE")
				}
				return
			}
			if len(calls) != 1 || operationName(calls[0].Target) != "GetCostAndUsageWithResources" || !strings.Contains(string(calls[0].Body), tc.want) {
				t.Fatalf("wrong CE query: %#v", calls)
			}
		})
	}
}

func TestActualCostBillingAccount(t *testing.T) {
	calc, _ := newClassifiedCalc()
	calc.cache = nil
	start, end := recentCostWindow()
	req := costRequest(bareInstanceID, "", start, end, nil)
	for _, account := range []string{"", "owner-account-a", "owner-account-b"} {
		req.BillingAccountId = account
		resp, err := calc.GetActualCost(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		row := resp.GetResults()[0]
		if account == "" {
			if row.FocusRecord != nil {
				t.Fatal("invented billing account when caller supplied none")
			}
			continue
		}
		if row.GetFocusRecord().GetBillingAccountId() != account {
			t.Fatalf("account=%q want=%q", row.GetFocusRecord().GetBillingAccountId(), account)
		}
	}
}
