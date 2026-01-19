package e2e

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	pluginBinary = flag.String("plugin-binary", "", "Path to the plugin binary. If empty, runs 'go run cmd/plugin/main.go'")
)

// Feature Toggles - Enable these as features are implemented
const (
	FeatureActualCost      = true  // Implemented
	FeatureForecasting     = false // Issue #25
	FeatureBudgets         = false // Issue #24
	FeatureAnomalies       = false // Issue #26
	FeatureRightsizing     = false // Issue #27
	FeatureSavingsPlans    = false // Issue #32
	FeatureReservedInst    = false // Issue #33
	FeatureEstimateCost    = false // Issue #30
	FeatureGreenops        = false // Issue #29
)

func TestE2E(t *testing.T) {
	if os.Getenv("PULUMICOST_E2E") != "true" {
		t.Skip("Skipping E2E tests. Set PULUMICOST_E2E=true to run.")
	}

	// 1. Setup Plugin Server
	port := 50055 // Arbitrary test port
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cleanup := startPluginServer(t, ctx, port)
	defer cleanup()

	// 2. Connect Client
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to connect to plugin: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := pbc.NewCostSourceServiceClient(conn)

	// 3. Run Test Suites
	t.Run("GetActualCost", func(t *testing.T) {
		if !FeatureActualCost {
			t.Skip("Feature toggle disabled")
		}
		testActualCost(t, client)
	})

	t.Run("GetProjectedCost", func(t *testing.T) {
		if !FeatureForecasting {
			t.Skip("Feature toggle disabled")
		}
		testForecasting(t, client)
	})

	t.Run("GetBudgets", func(t *testing.T) {
		if !FeatureBudgets {
			t.Skip("Feature toggle disabled")
		}
		testBudgets(t, client)
	})

	t.Run("GetAnomalies", func(t *testing.T) {
		if !FeatureAnomalies {
			t.Skip("Feature toggle disabled")
		}
		testAnomalies(t, client)
	})

	t.Run("GetRecommendations", func(t *testing.T) {
		if !FeatureRightsizing && !FeatureSavingsPlans && !FeatureReservedInst {
			t.Skip("All recommendation features disabled")
		}
		testRecommendations(t, client)
	})

	t.Run("EstimateCost", func(t *testing.T) {
		if !FeatureEstimateCost {
			t.Skip("Feature toggle disabled")
		}
		testEstimateCost(t, client)
	})
}

// startPluginServer starts the plugin process
func startPluginServer(t *testing.T, ctx context.Context, port int) func() {
	var cmd *exec.Cmd
	
	if *pluginBinary != "" {
		cmd = exec.CommandContext(ctx, *pluginBinary, "--port", fmt.Sprintf("%d", port))
	} else {
		// locate project root
		wd, _ := os.Getwd()
		projectRoot := filepath.Dir(filepath.Dir(wd)) // assuming test/e2e/e2e_test.go
		mainPath := filepath.Join(projectRoot, "cmd", "plugin", "main.go")
		
		cmd = exec.CommandContext(ctx, "go", "run", mainPath, "--port", fmt.Sprintf("%d", port))
		// Set working dir to project root so it finds .env or other files if needed
		cmd.Dir = projectRoot
	}

	// Capture output for debugging
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	// Set Env vars for testing
	cmd.Env = append(os.Environ(), 
		"PULUMICOST_LOG_LEVEL=debug",
		"PULUMICOST_TEST_MODE=true",
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start plugin server: %v", err)
	}

	// Give it a moment to start
	time.Sleep(2 * time.Second)

	return func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
}

// --- Test Implementation for Each Feature ---

func testActualCost(t *testing.T, client pbc.CostSourceServiceClient) {
	ctx := context.Background()
	now := time.Now()
	
	// Query last 7 days
	start := now.AddDate(0, 0, -7)
	end := now

	req := &pbc.GetActualCostRequest{
		ResourceId: "i-1234567890abcdef0", // Dummy ID, plugin should handle gracefully or use real ID if env setup
		Start:      timestamppb.New(start),
		End:        timestamppb.New(end),
		Tags: map[string]string{
			"Name": "e2e-test-instance",
		},
		// Granularity removed
	}

	resp, err := client.GetActualCost(ctx, req)
	if err != nil {
		t.Errorf("GetActualCost failed: %v", err)
		return
	}

	// Basic validation
	if resp == nil {
		t.Error("Received nil response")
		return
	}

	// Log fallback hint
	t.Logf("Fallback Hint: %v", resp.FallbackHint)
	t.Logf("Result Count: %d", len(resp.Results))
}

func testForecasting(t *testing.T, client pbc.CostSourceServiceClient) {
	ctx := context.Background()
	// now := time.Now()
	
	// Forecast next 30 days
	// start := now.AddDate(0, 0, 1)
	// end := now.AddDate(0, 0, 31)

	req := &pbc.GetProjectedCostRequest{
		Resource: &pbc.ResourceDescriptor{
			ResourceType: "aws:ec2/instance:Instance",
			Tags: map[string]string{
				"Environment": "Production",
			},
		},
		// Start: timestamppb.New(start), // Removed based on proto inspection
		// End:   timestamppb.New(end),   // Removed based on proto inspection
		// Granularity: "DAILY",         // Removed based on proto inspection
	}

	resp, err := client.GetProjectedCost(ctx, req)
	if err != nil {
		t.Errorf("GetProjectedCost failed: %v", err)
		return
	}

	if resp == nil {
		t.Error("Received nil response")
		return
	}
	
	t.Logf("Forecasted Cost Per Month: %f %s", resp.CostPerMonth, resp.Currency)
}

func testBudgets(t *testing.T, client pbc.CostSourceServiceClient) {
	// Requires RPC definition in newer spec, assume client has it
	// If method doesn't exist yet in the generated client code in this repo, 
	// this might fail compile. 
	// TODO: Uncomment once proto is updated/generated with GetBudgets
	
	/*
	ctx := context.Background()
	req := &pbc.GetBudgetsRequest{}
	resp, err := client.GetBudgets(ctx, req)
	if err != nil {
		t.Errorf("GetBudgets failed: %v", err)
		return
	}
	t.Logf("Budgets Found: %d", len(resp.Budgets))
	*/
	t.Log("GetBudgets test placeholder (proto update required)")
}

func testAnomalies(t *testing.T, client pbc.CostSourceServiceClient) {
	// TODO: Uncomment once proto is updated/generated with GetAnomalies
	/*
	ctx := context.Background()
	req := &pbc.GetAnomaliesRequest{
		Start: timestamppb.New(time.Now().AddDate(0, 0, -30)),
		End:   timestamppb.New(time.Now()),
	}
	resp, err := client.GetAnomalies(ctx, req)
	if err != nil {
		t.Errorf("GetAnomalies failed: %v", err)
		return
	}
	t.Logf("Anomalies Found: %d", len(resp.Anomalies))
	*/
	t.Log("GetAnomalies test placeholder (proto update required)")
}

func testRecommendations(t *testing.T, client pbc.CostSourceServiceClient) {
	// TODO: Uncomment once proto is updated/generated with GetRecommendations
	/*
	ctx := context.Background()
	req := &pbc.GetRecommendationsRequest{
		Category: pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_RIGHTSIZING,
	}
	resp, err := client.GetRecommendations(ctx, req)
	if err != nil {
		t.Errorf("GetRecommendations failed: %v", err)
		return
	}
	t.Logf("Recommendations Found: %d", len(resp.Recommendations))
	*/
	t.Log("GetRecommendations test placeholder (proto update required)")
}

func testEstimateCost(t *testing.T, client pbc.CostSourceServiceClient) {
	// TODO: Uncomment once proto is updated/generated with EstimateCost
	/*
	ctx := context.Background()
	req := &pbc.EstimateCostRequest{
		Resource: &pbc.ResourceDescriptor{
			ResourceType: "aws:ec2/instance:Instance",
			Inputs: map[string]string{
				"instanceType": "t3.micro",
				"region": "us-east-1",
			},
		},
	}
	resp, err := client.EstimateCost(ctx, req)
	if err != nil {
		t.Errorf("EstimateCost failed: %v", err)
		return
	}
	t.Logf("Estimated Cost: %f %s", resp.TotalCost, resp.Currency)
	*/
	t.Log("EstimateCost test placeholder (proto update required)")
}
