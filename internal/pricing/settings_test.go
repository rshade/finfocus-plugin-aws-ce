package pricing

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	plugintesting "github.com/rshade/finfocus-spec/sdk/go/testing"
)

func TestGetPluginInfo_Settings(t *testing.T) {
	t.Parallel()

	srv := pluginsdk.NewTestServer(t, NewCalculator())
	t.Cleanup(srv.Close)

	resp, err := srv.Client().GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo rpc: %v", err)
	}
	settings := resp.GetSettings()
	if err := plugintesting.ValidatePluginSettings(settings); err != nil {
		t.Fatalf("advertised settings are invalid: %v", err)
	}

	want := map[string]string{
		"FINFOCUS_PLUGIN_PORT":  "",
		"FINFOCUS_LOG_LEVEL":    "info",
		"FINFOCUS_LOG_FILE":     "",
		BatchSizeEnv:            strconv.Itoa(pluginsdk.DefaultMaxBatchSize),
		BatchWorkersEnv:         strconv.Itoa(pluginsdk.DefaultBatchWorkers),
		maxRequestsEnv:          "",
		HealthEndpointEnv:       "false",
		WebEnabledEnv:           "false",
		CORSAllowedOriginsEnv:   "",
		CORSAllowCredentialsEnv: "false",
		CORSAllowedHeadersEnv:   "",
		AccountIDEnv:            "",
	}
	got := make(map[string]string, len(settings))
	for _, setting := range settings {
		got[setting.GetEnv()] = setting.GetDefaultValue()
		if setting.GetSecret() {
			t.Errorf("%s is secret, but this plugin advertises no secret settings", setting.GetEnv())
		}
		if strings.HasPrefix(setting.GetEnv(), "AWS_") {
			t.Errorf("%s belongs to the AWS credential chain, not to this plugin", setting.GetEnv())
		}
	}
	if len(got) != len(want) {
		t.Fatalf("settings env = %v, want %v", got, want)
	}
	for env, def := range want {
		if value, ok := got[env]; !ok || value != def {
			t.Errorf("setting %s default = %q (present %t), want %q", env, value, ok, def)
		}
	}
	for key := range resp.GetMetadata() {
		if strings.Contains(key, "setting") {
			t.Errorf("metadata key %q copies settings into metadata", key)
		}
	}
}
