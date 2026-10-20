package main

import (
	"context"
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

func pluginEnv(extra map[string]string) map[string]string {
	env := map[string]string{
		"FINFOCUS_PLUGIN_PORT":           "0",
		"FINFOCUS_LOG_FILE":              "",
		"FINFOCUS_LOG_LEVEL":             "error",
		"FINFOCUS_AWS_CE_MAX_BATCH_SIZE": "100",
		"FINFOCUS_AWS_CE_BATCH_WORKERS":  "10",
		pricing.HealthEndpointEnv:        "",
		pricing.WebEnabledEnv:            "",
	}
	for key, value := range extra {
		env[key] = value
	}
	return env
}

func TestHealthEndpointEnabled(t *testing.T) {
	proc := testutil.StartPlugin(t, pluginEnv(map[string]string{
		pricing.HealthEndpointEnv: "true",
	}), "--port", "0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+proc.Address+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /healthz: %v\n%s", err, proc.Logs())
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200; body %q", resp.StatusCode, body)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestHealthEndpointDefaultOff(t *testing.T) {
	proc := testutil.StartPlugin(t, pluginEnv(nil), "--port", "0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+proc.Address+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("default /healthz returned 200")
	}
}

func TestHealthEndpointAbsentWhenDisabled(t *testing.T) {
	proc := testutil.StartPlugin(t, pluginEnv(map[string]string{
		pricing.HealthEndpointEnv: "false",
		pricing.WebEnabledEnv:     "true",
	}), "--port", "0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+proc.Address+"/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusOK {
		t.Fatal("disabled /healthz returned 200")
	}
}

func TestInvalidHealthSettingExitsNonzero(t *testing.T) {
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
		pricing.HealthEndpointEnv+"=maybe",
	)
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("invalid health setting exited successfully: %s", out)
	}
	if !strings.Contains(string(out), pricing.HealthEndpointEnv) {
		t.Fatalf("missing setting name in startup error: %s", out)
	}
}
