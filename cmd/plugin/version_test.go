package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

// This exercises the release linker flags against both public version consumers.
// A nonexistent linker target or hardcoded version must fail this test.
func TestReleaseVersionInMetadataAndLogs(t *testing.T) {
	const releaseVersion = "9.8.7-test"
	configBytes, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Builds []struct {
			LDFlags []string `yaml:"ldflags"`
		} `yaml:"builds"`
	}
	if err := yaml.Unmarshal(configBytes, &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Builds) != 1 || len(config.Builds[0].LDFlags) == 0 {
		t.Fatal("release must define linker flags")
	}
	flags := strings.NewReplacer("{{.Version}}", releaseVersion, "{{.Commit}}", "test-commit", "{{.Date}}", "2026-10-07").Replace(strings.Join(config.Builds[0].LDFlags, " "))
	binary := filepath.Join(t.TempDir(), "plugin")
	build := exec.Command("go", "build", "-ldflags", flags, "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build release binary: %v\n%s", err, out)
	}
	logFile := filepath.Join(t.TempDir(), "plugin.log")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--port", "0")
	command.Env = append(os.Environ(), "FINFOCUS_LOG_FILE="+logFile, "FINFOCUS_LOG_LEVEL=info", "FINFOCUS_AWS_CE_MAX_BATCH_SIZE=100", "FINFOCUS_AWS_CE_BATCH_WORKERS=10")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = command.Wait()
	})
	ports := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			if port, ok := strings.CutPrefix(scanner.Text(), "PORT="); ok {
				ports <- port
				return
			}
		}
		ports <- ""
	}()
	var port string
	select {
	case port = <-ports:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if port == "" {
		t.Fatal("release binary did not announce a port")
	}
	conn, err := grpc.NewClient("127.0.0.1:"+port, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	info, err := pbc.NewCostSourceServiceClient(conn).GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetVersion() != releaseVersion {
		t.Errorf("metadata version = %q, want %q", info.GetVersion(), releaseVersion)
	}
	logs, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(logs)), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event["message"] == "Starting plugin" {
			if event["plugin_version"] != releaseVersion {
				t.Errorf("startup log version = %v, want %q", event["plugin_version"], releaseVersion)
			}
			return
		}
	}
	t.Fatal("startup log missing")
}
