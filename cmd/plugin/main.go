package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rshade/pulumicost-plugin-aws-ce/internal/pricing"
	"github.com/rshade/pulumicost-spec/sdk/go/pluginsdk"
)

func main() {
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
		log.Println("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Start serving the plugin
	config := pluginsdk.ServeConfig{
		Plugin: plugin,
		Port:   0, // Let the system choose a port
	}

	log.Printf("Starting %s plugin...", plugin.Name())
	if err := pluginsdk.Serve(ctx, config); err != nil {
		log.Fatalf("Failed to serve plugin: %v", err)
	}
}
