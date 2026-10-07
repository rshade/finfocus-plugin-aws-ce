package main

import (
	"context"
	"flag"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/pricing"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/version"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
)

func main() {
	// Parse CLI flags (must be called before accessing flag values)
	flag.Parse()

	// Initialize logger using SDK helpers
	logWriter := pluginsdk.NewLogWriter()
	level := parseLogLevel(pluginsdk.GetLogLevel())
	logger := pluginsdk.NewPluginLogger("aws-ce", version.Version, level, logWriter)

	// Determine port: CLI flag takes precedence over environment variable
	port := pluginsdk.ParsePortFlag()
	portExplicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" {
			portExplicit = true
		}
	})
	if !portExplicit {
		port = pluginsdk.GetPort()
	}
	if port < 0 || port > 65535 {
		logger.Error().Int("port", port).Msg("Failed to serve plugin: port must be between 0 and 65535")
		os.Exit(1)
	}

	maxBatchSize, batchWorkers, err := batchSettingsFromEnv()
	if err != nil {
		logger.Error().Err(err).Msg("Invalid batch configuration")
		os.Exit(1)
	}

	// Create the plugin implementation
	plugin := pricing.NewCalculator()

	// Cancel the server context when the process receives a shutdown signal.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start serving the plugin
	config := pluginsdk.ServeConfig{
		Plugin:       plugin,
		Port:         port,
		MaxBatchSize: maxBatchSize,
		BatchWorkers: batchWorkers,
	}

	// Serve otherwise resolves port zero from the environment a second time.
	if portExplicit && port == 0 {
		listener, listenErr := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
		if listenErr != nil {
			logger.Error().Err(listenErr).Msg("Failed to serve plugin")
			os.Exit(1)
		}
		defer func() { _ = listener.Close() }()
		config.Listener = listener
	}

	logger.Info().Str("plugin_name", plugin.Name()).Int("port", port).Msg("Starting plugin")
	if err := pluginsdk.Serve(ctx, config); err != nil {
		logger.Error().Err(err).Msg("Failed to serve plugin")
		os.Exit(1)
	}
}

// parseLogLevel converts a string log level to zerolog.Level.
// Returns zerolog.InfoLevel as default for unrecognized values.
func parseLogLevel(levelStr string) zerolog.Level {
	switch strings.ToLower(levelStr) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info", "":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}
