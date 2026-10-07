package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-plugin-aws-ce/internal/testutil"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

func TestParseLogLevel(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  zerolog.Level
	}{
		{"trace", zerolog.TraceLevel}, {"DEBUG", zerolog.DebugLevel}, {"info", zerolog.InfoLevel}, {"", zerolog.InfoLevel}, {"warning", zerolog.WarnLevel}, {"warn", zerolog.WarnLevel}, {"error", zerolog.ErrorLevel}, {"fatal", zerolog.FatalLevel}, {"panic", zerolog.PanicLevel}, {"invalid", zerolog.InfoLevel},
	} {
		t.Run(tc.input, func(t *testing.T) {
			if got := parseLogLevel(tc.input); got != tc.want {
				t.Fatalf("level=%v want %v", got, tc.want)
			}
		})
	}
}

func TestPortConfiguration(t *testing.T) {
	for _, mode := range []string{"auto", "env", "cli", "cli-zero"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }()
			port := strings.TrimPrefix(listener.Addr().String(), "127.0.0.1:")
			env := map[string]string{"FINFOCUS_PLUGIN_PORT": "", "FINFOCUS_LOG_FILE": "", "FINFOCUS_LOG_LEVEL": "info", "FINFOCUS_AWS_CE_MAX_BATCH_SIZE": "100", "FINFOCUS_AWS_CE_BATCH_WORKERS": "10"}
			var args []string
			expected := port
			switch mode {
			case "auto":
				expected = ""
			case "env":
				env["FINFOCUS_PLUGIN_PORT"] = port
				_ = listener.Close()
			case "cli-zero":
				env["FINFOCUS_PLUGIN_PORT"] = port
				args = []string{"--port", "0"}
				expected = ""
			case "cli":
				env["FINFOCUS_PLUGIN_PORT"] = port // Kept occupied: an incorrect fallback cannot bind.
				chosen, e := net.Listen("tcp", "127.0.0.1:0")
				if e != nil {
					t.Fatal(e)
				}
				expected = strings.TrimPrefix(chosen.Addr().String(), "127.0.0.1:")
				_ = chosen.Close()
				args = []string{"--port", expected}
			}
			proc := testutil.StartPlugin(t, env, args...)
			if expected != "" && proc.Address != "127.0.0.1:"+expected {
				t.Fatalf("bound %s want %s", proc.Address, expected)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if _, err := proc.Client.GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{}); err != nil {
				t.Fatal(err)
			}
			if err := proc.Stop(); err != nil {
				t.Fatalf("shutdown: %v\n%s", err, proc.Logs())
			}
		})
	}
}

func TestInvalidPortExitsNonzero(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "plugin")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, port := range []string{"-1", "65536"} {
		t.Run(port, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, "--port", port)
			command.Env = append(os.Environ(), "FINFOCUS_LOG_FILE=", "FINFOCUS_PLUGIN_PORT=0", "FINFOCUS_AWS_CE_MAX_BATCH_SIZE=100", "FINFOCUS_AWS_CE_BATCH_WORKERS=10")
			out, err := command.CombinedOutput()
			if err == nil {
				t.Fatalf("invalid port exited successfully: %s", out)
			}
			if !strings.Contains(string(out), "Failed to serve plugin") {
				t.Fatalf("missing startup error: %s", out)
			}
		})
	}
}
