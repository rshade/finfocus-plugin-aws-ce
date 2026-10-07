package pricing

import (
	"context"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"math/big"
	"testing"
)

func TestUnknownResourceDoesNotReturnAccountCost(t *testing.T) {
	calc, api := newClassifiedCalc()
	start, end := recentCostWindow()
	for _, id := range []string{"my-bucket", "i-fallback"} {
		req := costRequest(id, "", start, end, nil)
		_, err := calc.GetActualCost(context.Background(), req)
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("%s: got %v, want InvalidArgument", id, err)
		}
		supported, err := calc.Supports(context.Background(), &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "aws", Id: id}})
		if err != nil || supported.GetSupported() {
			t.Fatalf("%s: Supports=%v err=%v", id, supported, err)
		}
	}
	assertNoCostExplorerCall(t, api)
}

func TestExplicitServiceQueryIsFiltered(t *testing.T) {
	calc, api := newClassifiedCalc()
	start, end := recentCostWindow()
	req := costRequest("service-total", "", start, end, nil)
	req.Resource = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:service", Id: "Amazon Simple Storage Service"}
	if _, err := calc.GetActualCost(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(api.costCalls) != 1 || api.costCalls[0].Filter == nil || api.costCalls[0].Filter.Dimensions == nil || api.costCalls[0].Filter.Dimensions.Values[0] != "Amazon Simple Storage Service" {
		t.Fatalf("unfiltered service query: %#v", api.costCalls)
	}
}

func TestAggregateMixedUsageUnitsOmitted(t *testing.T) {
	rows := []client.CostResult{
		{ServiceName: "EC2", Currency: "USD", AmountExact: big.NewRat(1, 1), UsageExact: big.NewRat(2, 1), UsageUnit: "Hrs"},
		{ServiceName: "EC2", Currency: "USD", AmountExact: big.NewRat(3, 1), UsageExact: big.NewRat(3, 1), UsageUnit: "GB"},
		{ServiceName: "EC2", Currency: "USD", AmountExact: big.NewRat(5, 1), UsageExact: big.NewRat(4, 1), UsageUnit: "Hrs"},
	}
	out, err := aggregateCosts(rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Amount != 9 || out[0].HasUsage || out[0].UsageUnit != "" || out[0].UsageAmount != 0 {
		t.Fatalf("mixed unit aggregate: %#v", out)
	}
}

func TestExplicitAccountQueryOverridesResourceShapedLabel(t *testing.T) {
	calc, api := newClassifiedCalc()
	start, end := recentCostWindow()
	req := costRequest(bareInstanceID, "", start, end, nil)
	req.Resource = &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:account", Id: bareInstanceID}
	if _, err := calc.GetActualCost(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	assertServiceTotalQuery(t, api)
}
