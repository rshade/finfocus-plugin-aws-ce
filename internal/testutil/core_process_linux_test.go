//go:build linux

package testutil

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoreCancellationStopsPluginChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCoreProcessTreeHelper$")
	cmd.Env = append(os.Environ(), "FINFOCUS_TEST_PROCESS_HELPER=parent")
	configureCoreProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = stopCoreProcess(cmd) })
	childPID := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			childPID <- scanner.Text()
		}
	}()
	var pid int
	select {
	case line := <-childPID:
		pid, err = strconv.Atoi(line)
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("core helper did not start its plugin child")
	}
	child, err := os.FindProcess(pid)
	require.NoError(t, err)
	t.Cleanup(func() { _ = child.Kill() })
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("core cancellation did not complete")
	}
	require.Eventually(t, func() bool {
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return true
		}
		if err != nil {
			return false
		}
		// A killed orphan can remain a zombie until the host init reaps it.
		// Neither a missing process nor a zombie can make further AWS calls.
		fields := strings.Fields(string(stat)[strings.LastIndex(string(stat), ")")+1:])
		return len(fields) > 0 && fields[0] == "Z"
	}, 2*time.Second, 10*time.Millisecond, "plugin child must stop when core is cancelled")
}

func TestCoreProcessTreeHelper(t *testing.T) {
	switch os.Getenv("FINFOCUS_TEST_PROCESS_HELPER") {
	case "parent":
		cmd := exec.Command(os.Args[0], "-test.run=^TestCoreProcessTreeHelper$")
		cmd.Env = append(os.Environ(), "FINFOCUS_TEST_PROCESS_HELPER=child")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		fmt.Println(cmd.Process.Pid)
		time.Sleep(time.Hour)
	case "child":
		time.Sleep(time.Hour)
	}
}
