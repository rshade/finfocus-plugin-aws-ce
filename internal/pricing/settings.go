package pricing

import (
	"strconv"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

const (
	BatchSizeEnv    = "FINFOCUS_AWS_CE_MAX_BATCH_SIZE"
	BatchWorkersEnv = "FINFOCUS_AWS_CE_BATCH_WORKERS"

	// HealthEndpointEnv enables the SDK /healthz endpoint. The SDK serves it on
	// the plugin listener, and only while Connect is enabled.
	HealthEndpointEnv = "FINFOCUS_HEALTH_ENDPOINT"
	// WebEnabledEnv serves Connect, gRPC-Web, and gRPC on the plugin listener.
	WebEnabledEnv = "FINFOCUS_WEB_ENABLED"
)

// pluginSettings lists the environment variables this plugin reads. AWS
// credential chain variables stay out: the AWS SDK owns them and some are secret.
func pluginSettings() []*pbc.PluginSetting {
	return []*pbc.PluginSetting{
		{
			Key:         "plugin_port",
			Env:         "FINFOCUS_PLUGIN_PORT",
			Type:        pbc.PluginSettingType_PLUGIN_SETTING_TYPE_INT,
			Description: "TCP port for the gRPC server. The --port flag takes precedence.",
		},
		{
			Key:          "log_level",
			Env:          "FINFOCUS_LOG_LEVEL",
			Type:         pbc.PluginSettingType_PLUGIN_SETTING_TYPE_ENUM,
			DefaultValue: "info",
			EnumValues:   []string{"trace", "debug", "info", "warn", "error", "fatal", "panic"},
			Description:  "Minimum level for structured logs.",
		},
		{
			Key:         "log_file",
			Env:         "FINFOCUS_LOG_FILE",
			Type:        pbc.PluginSettingType_PLUGIN_SETTING_TYPE_STRING,
			Description: "Absolute path of a log file. Unset writes logs to standard error.",
		},
		{
			Key:          "max_batch_size",
			Env:          BatchSizeEnv,
			Type:         pbc.PluginSettingType_PLUGIN_SETTING_TYPE_INT,
			DefaultValue: strconv.Itoa(pluginsdk.DefaultMaxBatchSize),
			Description:  "Largest BatchCost request, from 1 to " + strconv.Itoa(pluginsdk.MaxBatchSize) + ".",
		},
		{
			Key:          "batch_workers",
			Env:          BatchWorkersEnv,
			Type:         pbc.PluginSettingType_PLUGIN_SETTING_TYPE_INT,
			DefaultValue: strconv.Itoa(pluginsdk.DefaultBatchWorkers),
			Description:  "Concurrent BatchCost workers, from 1 to " + strconv.Itoa(pluginsdk.MaxBatchWorkers) + ".",
		},
		{
			Key:         "max_requests_per_minute",
			Env:         maxRequestsEnv,
			Type:        pbc.PluginSettingType_PLUGIN_SETTING_TYPE_INT,
			Description: "Cost Explorer page budget per minute. Each page costs $0.01. Unset or 0 means no limit.",
		},
		{
			Key:          "health_endpoint",
			Env:          HealthEndpointEnv,
			Type:         pbc.PluginSettingType_PLUGIN_SETTING_TYPE_BOOL,
			DefaultValue: "false",
			Description:  "Serve SDK /healthz on the plugin listener. Enabling it also enables Connect, because the SDK mounts /healthz only in that mode. Connect does not propagate the gRPC trace header. There is no separate health port.",
		},
		{
			Key:          "web_enabled",
			Env:          WebEnabledEnv,
			Type:         pbc.PluginSettingType_PLUGIN_SETTING_TYPE_BOOL,
			DefaultValue: "false",
			Description:  "Serve Connect and gRPC-Web beside gRPC. Also turns on when the health endpoint is enabled.",
		},
	}
}
