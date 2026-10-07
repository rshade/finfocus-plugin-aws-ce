package e2e

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var pluginBinary = flag.String("plugin-binary", "", "Plugin binary; empty builds cmd/plugin")

func e2eEnabled() bool {
	if value, set := os.LookupEnv("FINFOCUS_E2E"); set {
		return value == "true"
	}
	value, _ := os.LookupEnv("finfocus_E2E")
	return value == "true"
}

func TestE2EGate(t *testing.T) {
	for _, tc := range []struct {
		name, value, legacy string
		unset, want         bool
	}{
		{name: "uppercase", value: "true", want: true},
		{name: "legacy fallback", legacy: "true", unset: true, want: true},
		{name: "explicit false overrides legacy", value: "false", legacy: "true"},
		{name: "disabled", unset: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FINFOCUS_E2E", tc.value)
			t.Setenv("finfocus_E2E", tc.legacy)
			if tc.unset {
				if err := os.Unsetenv("FINFOCUS_E2E"); err != nil {
					t.Fatal(err)
				}
			}
			if got := e2eEnabled(); got != tc.want {
				t.Fatalf("enabled=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestE2E(t *testing.T) {
	if !e2eEnabled() {
		t.Skip("Set FINFOCUS_E2E=true for subprocess E2E against a local fake CE endpoint")
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != "AWSInsightsIndexService.GetCostAndUsage" {
			t.Errorf("unexpected AWS operation %s", r.Header.Get("X-Amz-Target"))
		}
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		_, _ = w.Write([]byte(`{"ResultsByTime":[{"Estimated":false,"TimePeriod":{"Start":"2026-09-20","End":"2026-09-21"},"Groups":[{"Keys":["Amazon Simple Storage Service"],"Metrics":{"UnblendedCost":{"Amount":"1.25","Unit":"USD"}}}]}]}`))
	}))
	defer fake.Close()
	binary := *pluginBinary
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "plugin")
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/plugin")
		cmd.Dir = filepath.Join("..", "..")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, output)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--port", "0")
	cmd.Env = append(os.Environ(), "AWS_ENDPOINT_URL_COST_EXPLORER="+fake.URL, "AWS_ENDPOINT_URL="+fake.URL, "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS=false", "AWS_REGION=us-east-1", "AWS_ACCESS_KEY_ID=test", "AWS_SECRET_ACCESS_KEY=test", "AWS_SESSION_TOKEN=", "AWS_PROFILE=", "AWS_EC2_METADATA_DISABLED=true", "FINFOCUS_PLUGIN_PORT=0", "FINFOCUS_LOG_FILE=")
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			t.Errorf("plugin exit: %v", err)
		}
	}()
	ports := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if line := scanner.Text(); strings.HasPrefix(line, "PORT=") {
				ports <- strings.TrimPrefix(line, "PORT=")
				return
			}
		}
		ports <- ""
	}()
	var port string
	select {
	case port = <-ports:
	case <-ctx.Done():
		t.Fatal("plugin startup timed out")
	}
	if number, err := strconv.Atoi(port); err != nil || number < 1 {
		t.Fatalf("invalid port announcement %q", port)
	}
	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%s", port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	end := time.Now().UTC().Add(-24 * time.Hour)
	start := end.Add(-24 * time.Hour)
	resp, err := pbc.NewCostSourceServiceClient(conn).GetActualCost(ctx, &pbc.GetActualCostRequest{ResourceId: "e2e-service-totals", Start: timestamppb.New(start), End: timestamppb.New(end)})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetResults()) != 1 || resp.GetResults()[0].GetCost() != 1.25 {
		t.Fatalf("actual costs=%v", resp)
	}
}
