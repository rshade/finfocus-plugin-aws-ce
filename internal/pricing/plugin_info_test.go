package pricing

import (
	"context"
	"testing"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

const (
	pluginInfoName        = "aws-ce"
	pluginInfoVersion     = "0.1.0"
	pluginInfoSpecVersion = "v0.7.5"
	pluginInfoRPCs        = "GetActualCost,Supports,GetPluginInfo,GetProjectedCost"
)

func TestGetPluginInfo(t *testing.T) {
	t.Parallel()

	if err := pluginsdk.ValidateSpecVersion(pluginInfoSpecVersion); err != nil {
		t.Fatalf("spec version %q is not accepted by ValidateSpecVersion: %v", pluginInfoSpecVersion, err)
	}

	resp, err := NewCalculator().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo: %v", err)
	}
	assertPluginInfo(t, resp)
}

func TestGetPluginInfo_GRPC(t *testing.T) {
	t.Parallel()

	srv := pluginsdk.NewTestServer(t, NewCalculator())
	t.Cleanup(srv.Close)

	resp, err := srv.Client().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo rpc: %v", err)
	}
	if resp.GetName() != pluginInfoName {
		t.Fatalf("name = %q, want %q", resp.GetName(), pluginInfoName)
	}
	caps := resp.GetCapabilities()
	if !hasCapability(caps, pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS) {
		t.Fatalf("capabilities %v do not contain ACTUAL_COSTS", caps)
	}
	if hasCapability(caps, pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS) {
		t.Fatalf("capabilities %v contain PROJECTED_COSTS", caps)
	}
	if len(caps) != 1 {
		t.Fatalf("capabilities = %v, want only ACTUAL_COSTS", caps)
	}
	if got := resp.GetMetadata()["supported_rpcs"]; got != pluginInfoRPCs {
		t.Fatalf("supported_rpcs = %q, want %q", got, pluginInfoRPCs)
	}
	if got := resp.GetMetadata()[pluginsdk.MetadataSupportsPerRequestCredentials]; got != pluginsdk.ValueTrue {
		t.Fatalf("supports_per_request_credentials = %q, want %q; metadata=%v", got, pluginsdk.ValueTrue, resp.GetMetadata())
	}
}

func assertPluginInfo(t *testing.T, resp *pbc.GetPluginInfoResponse) {
	t.Helper()
	if resp == nil {
		t.Fatal("GetPluginInfo returned nil response")
	}
	if resp.GetName() != pluginInfoName {
		t.Fatalf("name = %q, want %q", resp.GetName(), pluginInfoName)
	}
	if resp.GetVersion() != pluginInfoVersion {
		t.Fatalf("version = %q, want %q", resp.GetVersion(), pluginInfoVersion)
	}
	if resp.GetSpecVersion() != pluginInfoSpecVersion {
		t.Fatalf("spec_version = %q, want %q", resp.GetSpecVersion(), pluginInfoSpecVersion)
	}
	providers := resp.GetProviders()
	if len(providers) != 1 || providers[0] != "aws" {
		t.Fatalf("providers = %v, want [aws]", providers)
	}
	caps := resp.GetCapabilities()
	if len(caps) != 1 || caps[0] != pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS {
		t.Fatalf("capabilities = %v, want only ACTUAL_COSTS", caps)
	}
	if hasCapability(caps, pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS) {
		t.Fatalf("capabilities %v contain PROJECTED_COSTS", caps)
	}
	meta := resp.GetMetadata()
	if len(meta) != 1 {
		t.Fatalf("metadata = %v, want only supported_rpcs", meta)
	}
	if meta["supported_rpcs"] != pluginInfoRPCs {
		t.Fatalf("supported_rpcs = %q, want %q", meta["supported_rpcs"], pluginInfoRPCs)
	}
	if _, ok := meta[pluginsdk.MetadataSupportsPerRequestCredentials]; ok {
		t.Fatalf("direct GetPluginInfo metadata includes %s: %v", pluginsdk.MetadataSupportsPerRequestCredentials, meta)
	}
}
