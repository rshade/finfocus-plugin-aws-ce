package pricing

import (
	"context"
	"strings"
	"testing"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProjectedCostUnimplemented(t *testing.T) {
	calc := NewCalculator()
	for _, req := range []*pbc.GetProjectedCostRequest{nil, {}, {Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2", Id: bareInstanceID}}} {
		_, err := calc.GetProjectedCost(context.Background(), req)
		if status.Code(err) != codes.Unimplemented || !strings.Contains(status.Convert(err).Message(), "CE-6.10") {
			t.Fatalf("projected error=%v", err)
		}
	}
	srv := pluginsdk.NewTestServer(t, calc)
	defer srv.Close()
	_, err := srv.Client().GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "ec2"}})
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("wire code=%s", status.Code(err))
	}
	info, err := srv.Client().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(info.GetMetadata()["supported_rpcs"], "GetProjectedCost") {
		t.Fatal("unimplemented projected RPC advertised as supported")
	}
}
