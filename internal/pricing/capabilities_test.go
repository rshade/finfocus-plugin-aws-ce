package pricing

import (
	"context"
	"testing"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

func TestWireCapabilities(t *testing.T) {
	srv := pluginsdk.NewTestServer(t, NewCalculator())
	defer srv.Close()
	ctx := context.Background()
	info, err := srv.Client().GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.GetCapabilities()) != 4 || info.GetCapabilities()[2] != pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS || info.GetCapabilities()[3] != pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS {
		t.Fatalf("expected actual costs, projected costs, budgets, recommendations; info capabilities=%v", info.GetCapabilities())
	}
	s3, err := srv.Client().Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:s3/bucket:Bucket"}})
	if err != nil || !s3.GetSupported() {
		t.Fatalf("s3 type support=%v err=%v", s3.GetSupported(), err)
	}
	if len(s3.GetCapabilitiesEnum()) != 4 || s3.GetCapabilitiesEnum()[2] != pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS || s3.GetCapabilitiesEnum()[3] != pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS {
		t.Fatalf("expected all four plugin capabilities; s3 capabilities=%v", s3.GetCapabilitiesEnum())
	}
	for _, provider := range []string{"aws", "azure"} {
		resp, err := srv.Client().Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: provider, ResourceType: "ec2", Id: bareInstanceID}})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.GetCapabilitiesEnum()) != 4 || resp.GetCapabilitiesEnum()[2] != pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS || resp.GetCapabilitiesEnum()[3] != pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS {
			t.Fatalf("expected all four plugin capabilities; %s typed capabilities=%v", provider, resp.GetCapabilitiesEnum())
		}
		legacy := resp.GetCapabilities()
		if len(legacy) != 4 || !legacy["supports_actual_costs"] || !legacy["supports_projected_costs"] || !legacy["supports_budgets"] || !legacy["supports_recommendations"] {
			t.Fatalf("expected all four legacy capability flags; %s legacy capabilities=%v", provider, legacy)
		}
	}
}
