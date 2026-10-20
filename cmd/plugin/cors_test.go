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

func TestCORSPreflightAllowedOrigin(t *testing.T) {
	proc := testutil.StartPlugin(t, pluginEnv(map[string]string{
		pricing.HealthEndpointEnv:       "false",
		pricing.WebEnabledEnv:           "true",
		pricing.CORSAllowedOriginsEnv:   "http://localhost:3000",
		pricing.CORSAllowCredentialsEnv: "true",
		pricing.CORSAllowedHeadersEnv:   "Content-Type, X-Request-Id",
	}), "--port", "0")

	resp := corsPreflight(t, proc, "http://localhost:3000")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("preflight = %d, want 204; body %q; logs %s", resp.StatusCode, body, proc.Logs())
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials = %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Request-Id" {
		t.Fatalf("allow-headers = %q", got)
	}
}

func TestCORSPreflightDisallowedOrigin(t *testing.T) {
	proc := testutil.StartPlugin(t, pluginEnv(map[string]string{
		pricing.HealthEndpointEnv:     "false",
		pricing.WebEnabledEnv:         "true",
		pricing.CORSAllowedOriginsEnv: "http://localhost:3000",
	}), "--port", "0")

	resp := corsPreflight(t, proc, "http://evil.example")
	defer func() { _ = resp.Body.Close() }()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("disallowed origin received allow-origin %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if resp.StatusCode == http.StatusNoContent {
		t.Fatal("disallowed origin received a successful preflight")
	}
}

func TestCORSWildcardCredentialsExitNonzero(t *testing.T) {
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
		pricing.WebEnabledEnv+"=true",
		pricing.CORSAllowedOriginsEnv+"=*",
		pricing.CORSAllowCredentialsEnv+"=true",
	)
	out, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("wildcard credentials exited successfully: %s", out)
	}
	if !strings.Contains(string(out), "AllowCredentials cannot be used with wildcard") {
		t.Fatalf("missing SDK CORS rejection: %s", out)
	}
}

func corsPreflight(t *testing.T, proc *testutil.PluginProcess, origin string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodOptions, "http://"+proc.Address+"/finfocus.v1.CostSourceService/GetPluginInfo", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("preflight: %v\n%s", err, proc.Logs())
	}
	return resp
}
