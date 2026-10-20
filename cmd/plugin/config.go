package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/pricing"
)

func batchSettingsFromEnv() (int, int, error) {
	size, err := batchSetting(pricing.BatchSizeEnv, pluginsdk.DefaultMaxBatchSize, pluginsdk.MaxBatchSize)
	if err != nil {
		return 0, 0, err
	}
	workers, err := batchSetting(pricing.BatchWorkersEnv, pluginsdk.DefaultBatchWorkers, pluginsdk.MaxBatchWorkers)
	if err != nil {
		return 0, 0, err
	}
	return size, workers, nil
}

// webConfigFromEnv reads the health, Connect, and CORS settings.
// Health defaults off. The SDK mounts /healthz only in Connect mode, and
// Connect does not run the gRPC trace interceptor. Turning health on also
// turns Connect on, on the same listener. There is no separate health port.
func webConfigFromEnv() (pluginsdk.WebConfig, error) {
	health, err := envBool(pricing.HealthEndpointEnv, false)
	if err != nil {
		return pluginsdk.WebConfig{}, err
	}
	webEnabled, err := envBool(pricing.WebEnabledEnv, false)
	if err != nil {
		return pluginsdk.WebConfig{}, err
	}
	credentials, err := envBool(pricing.CORSAllowCredentialsEnv, false)
	if err != nil {
		return pluginsdk.WebConfig{}, err
	}
	origins, err := commaList(pricing.CORSAllowedOriginsEnv)
	if err != nil {
		return pluginsdk.WebConfig{}, err
	}
	headers, err := commaList(pricing.CORSAllowedHeadersEnv)
	if err != nil {
		return pluginsdk.WebConfig{}, err
	}
	return pluginsdk.WebConfig{
		Enabled:              health || webEnabled,
		EnableHealthEndpoint: health,
		AllowedOrigins:       origins,
		AllowCredentials:     credentials,
		AllowedHeaders:       headers,
	}, nil
}

// commaList returns nil when the variable is unset or empty so the SDK can
// apply its own default. A blank entry between commas is invalid.
func commaList(name string) ([]string, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return nil, fmt.Errorf("%s contains an empty entry", name)
		}
		out = append(out, item)
	}
	return out, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value, ok := os.LookupEnv(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true or false", name)
	}
}

func batchSetting(name string, fallback, maximum int) (int, error) {
	value, _ := os.LookupEnv(name)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || number > maximum {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", name, maximum)
	}
	return number, nil
}
