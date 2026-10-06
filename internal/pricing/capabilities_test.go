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
	if len(info.GetCapabilities()) != 1 || info.GetCapabilities()[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS {
		t.Fatalf("info capabilities=%v", info.GetCapabilities())
	}
	for _, provider := range []string{"aws", "azure"} {
		resp, err := srv.Client().Supports(ctx, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: provider, ResourceType: "ec2", Id: bareInstanceID}})
		if err != nil {
			t.Fatal(err)
		}
		if len(resp.GetCapabilitiesEnum()) != 1 || resp.GetCapabilitiesEnum()[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS {
			t.Fatalf("%s typed capabilities=%v", provider, resp.GetCapabilitiesEnum())
		}
		if len(resp.GetCapabilities()) != 1 || !resp.GetCapabilities()["supports_actual_costs"] {
			t.Fatalf("%s legacy capabilities=%v", provider, resp.GetCapabilities())
		}
	}
}
