package pricing

import (
	"context"
	"testing"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

func TestWireCapabilitiesActualOnly(t *testing.T) {
	srv := pluginsdk.NewTestServer(t, NewCalculator())
	defer srv.Close()
	ctx := context.Background()
	info, err := srv.Client().GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(info.GetCapabilities()) != 2 || info.GetCapabilities()[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS || info.GetCapabilities()[1] != pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS {
		t.Fatalf("info capabilities=%v", info.GetCapabilities())
	}
	s3, err := srv.Client().Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:s3/bucket:Bucket"}})
	if err != nil || !s3.GetSupported() {
		t.Fatalf("s3 type support=%v err=%v", s3.GetSupported(), err)
	}
	if len(s3.GetCapabilitiesEnum()) != 2 || s3.GetCapabilitiesEnum()[1] != pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS {
		t.Fatalf("s3 capabilities=%v", s3.GetCapabilitiesEnum())
	}
	for _, provider := range []string{"aws", "azure"} {
		resp, err := srv.Client().Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: provider, ResourceType: "ec2", Id: bareInstanceID}})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.GetCapabilitiesEnum()) != 2 || resp.GetCapabilitiesEnum()[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS || resp.GetCapabilitiesEnum()[1] != pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS {
			t.Fatalf("%s typed capabilities=%v", provider, resp.GetCapabilitiesEnum())
		}
		legacy := resp.GetCapabilities()
		if len(legacy) != 2 || !legacy["supports_actual_costs"] || !legacy["supports_projected_costs"] {
			t.Fatalf("%s legacy capabilities=%v", provider, legacy)
		}
	}
}
