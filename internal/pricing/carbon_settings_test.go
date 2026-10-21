package pricing

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/client"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Removing the descriptor must fail this real SDK discovery contract.
func TestCarbonSettingDescriptorGRPC(t *testing.T) {
	srv := pluginsdk.NewTestServer(t, NewCalculator())
	t.Cleanup(srv.Close)
	info, err := srv.Client().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, setting := range info.GetSettings() {
		if setting.GetKey() != "carbon_enabled" {
			continue
		}
		count++
		if setting.GetEnv() != "FINFOCUS_AWS_CARBON_ENABLED" || setting.GetType() != pbc.PluginSettingType_PLUGIN_SETTING_TYPE_BOOL || setting.GetDefaultValue() != "false" {
			t.Errorf("descriptor = %v", setting)
		}
		description := strings.ToLower(setting.GetDescription())
		if !strings.Contains(description, "historical") || !strings.Contains(description, "aggregate") {
			t.Errorf("description = %q", description)
		}
	}
	if count != 1 {
		t.Fatalf("carbon_enabled descriptor count = %d, want 1", count)
	}
}

func TestCarbonDiscoveryAndDisabledHandler(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "fixture-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fixture-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			calc := NewCalculator(WithCarbonEnabled(enabled))
			// Any cost-client access would fail: configuration and discovery must be lazy.
			calc.openClient = func(context.Context, client.Config) (*client.Client, error) {
				t.Fatal("unexpected AWS client creation")
				return nil, nil
			}
			calc.openCarbon = func(context.Context, client.Config) (carbonReader, error) {
				return carbonReaderFunc(func(context.Context, *pbc.GetCarbonEmissionsRequest, string) (client.CarbonPage, error) {
					return client.CarbonPage{}, nil
				}), nil
			}
			srv := pluginsdk.NewTestServer(t, calc)
			t.Cleanup(srv.Close)
			info, err := srv.Client().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if hasCapability(info.GetCapabilities(), pbc.PluginCapability_PLUGIN_CAPABILITY_HISTORICAL_CARBON) != enabled {
				t.Errorf("capabilities = %v", info.GetCapabilities())
			}
			if hasCapability(info.GetCapabilities(), pbc.PluginCapability_PLUGIN_CAPABILITY_CARBON) {
				t.Fatal("projected CARBON advertised")
			}
			count := 0
			for _, setting := range info.GetSettings() {
				if setting.GetKey() == "carbon_enabled" {
					count++
					if setting.GetType() != pbc.PluginSettingType_PLUGIN_SETTING_TYPE_BOOL || setting.GetEnv() != CarbonEnabledEnv || setting.GetDefaultValue() != "false" {
						t.Errorf("descriptor = %v", setting)
					}
				}
			}
			if count != 1 {
				t.Errorf("descriptor count = %d", count)
			}
			if strings.Contains(info.GetMetadata()["supported_rpcs"], "GetCarbonEmissions") != enabled {
				t.Errorf("RPCs = %v", info.GetMetadata())
			}
			_, err = srv.Client().GetCarbonEmissions(context.Background(), &pbc.GetCarbonEmissionsRequest{Start: timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), End: timestamppb.New(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))})
			if !enabled {
				_, directErr := calc.GetCarbonEmissions(context.Background(), nil)
				if status.Code(directErr) != codes.FailedPrecondition {
					t.Fatalf("disabled direct nil request: %v", directErr)
				}
			}
			want := codes.FailedPrecondition
			if enabled {
				want = codes.OK
			}
			if status.Code(err) != want {
				t.Errorf("code = %v, want %v", status.Code(err), want)
			}
		})
	}
}
