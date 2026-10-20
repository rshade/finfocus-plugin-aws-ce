package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rshade/finfocus-plugin-aws-ce/internal/pricing"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
)

func TestConnectGetPluginInfo(t *testing.T) {
	proc := testutil.StartPlugin(t, pluginEnv(map[string]string{
		pricing.HealthEndpointEnv: "false",
		pricing.WebEnabledEnv:     "true",
	}), "--port", "0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+proc.Address+"/finfocus.v1.CostSourceService/GetPluginInfo", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Connect GetPluginInfo: %v\n%s", err, proc.Logs())
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode: %v body %s", err, body)
	}
	if decoded["name"] != "aws-ce" {
		t.Fatalf("name = %v, body %s", decoded["name"], body)
	}
}

func TestInvalidWebSettingExitsNonzero(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "plugin")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "--port", "0")
	command.Env = append(os.Environ(),
		"FINFOCUS_LOG_FILE=",
		"FINFOCUS_PLUGIN_PORT=0",
		"FINFOCUS_AWS_CE_MAX_BATCH_SIZE=100",
		"FINFOCUS_AWS_CE_BATCH_WORKERS=10",
		pricing.HealthEndpointEnv+"=false",
		pricing.WebEnabledEnv+"=maybe",
	)
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid web setting exited successfully: %s", out)
	}
	if !strings.Contains(string(out), pricing.WebEnabledEnv) {
		t.Fatalf("missing setting name in startup error: %s", out)
	}
}
