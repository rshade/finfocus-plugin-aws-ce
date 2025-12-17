package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rshade/pulumicost-plugin-aws-ce/internal/pricing"
	"github.com/rshade/pulumicost-spec/sdk/go/pluginsdk"
)

func main() {
	// Parse CLI flags (must be called before accessing flag values)
	flag.Parse()

	// Initialize logger using SDK helpers
	logWriter := pluginsdk.NewLogWriter()
	level := parseLogLevel(pluginsdk.GetLogLevel())
	logger := pluginsdk.NewPluginLogger("aws-ce", "1.0.0", level, logWriter)

	// Determine port: CLI flag takes precedence over environment variable
	port := pluginsdk.ParsePortFlag()
	if port == 0 {
		port = pluginsdk.GetPort()
	}

	// Create the plugin implementation
	plugin := pricing.NewCalculator()

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info().Msg("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Start serving the plugin
	config := pluginsdk.ServeConfig{
		Plugin: plugin,
		Port:   port,
	}

	logger.Info().Str("plugin_name", plugin.Name()).Int("port", port).Msg("Starting plugin")
	if err := pluginsdk.Serve(ctx, config); err != nil {
		logger.Error().Err(err).Msg("Failed to serve plugin")
		return
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
		// Log warning about unrecognized level - but we can't log yet since logger isn't created
		// This is a chicken-and-egg problem; default to info level
		return zerolog.InfoLevel
	}
}
