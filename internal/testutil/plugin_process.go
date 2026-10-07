// Package testutil provides subprocess helpers for plugin integration tests.
package testutil

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type lockedBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *lockedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.data.String() }

// PluginProcess owns a built binary, connection, output and process lifecycle.
type PluginProcess struct {
	Client  pbc.CostSourceServiceClient
	Address string
	cmd     *exec.Cmd
	conn    *grpc.ClientConn
	logs    *lockedBuffer
	done    chan error
	cancel  context.CancelFunc
	stop    sync.Once
	exitErr error
}

// StartPlugin builds and starts cmd/plugin with an isolated environment overlay.
func StartPlugin(t testing.TB, env map[string]string, args ...string) *PluginProcess {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository")
	}
	root := filepath.Join(filepath.Dir(source), "..", "..")
	binary := filepath.Join(t.TempDir(), "plugin")
	build := exec.Command("go", "build", "-o", binary, "./cmd/plugin")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build plugin: %v\n%s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	cmd := exec.CommandContext(ctx, binary, args...)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, override := env[key]; !override {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	logs := &lockedBuffer{}
	cmd.Stderr = logs
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	proc := &PluginProcess{cmd: cmd, logs: logs, done: make(chan error, 1), cancel: cancel}
	t.Cleanup(func() { _ = proc.Stop() })
	go func() { proc.done <- cmd.Wait() }()
	ports := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		sent := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PORT=") && !sent {
				ports <- strings.TrimPrefix(line, "PORT=")
				sent = true
			}
		}
		if !sent {
			ports <- ""
		}
	}()
	var port string
	select {
	case port = <-ports:
	case <-ctx.Done():
		t.Fatalf("plugin startup: %v\n%s", ctx.Err(), logs.String())
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		t.Fatalf("invalid PORT announcement %q\n%s", port, logs.String())
	}
	proc.Address = fmt.Sprintf("127.0.0.1:%d", number)
	proc.conn, err = grpc.NewClient(proc.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	proc.Client = pbc.NewCostSourceServiceClient(proc.conn)
	return proc
}

// Stop sends SIGTERM, waits for clean context cancellation, and bounds cleanup.
func (p *PluginProcess) Stop() error {
	p.stop.Do(func() {
		if p.conn != nil {
			_ = p.conn.Close()
		}
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case p.exitErr = <-p.done:
		case <-time.After(5 * time.Second):
			p.cancel()
			p.exitErr = <-p.done
		}
		p.cancel()
	})
	return p.exitErr
}

// Logs returns a synchronized snapshot of process stderr.
func (p *PluginProcess) Logs() string { return p.logs.String() }
