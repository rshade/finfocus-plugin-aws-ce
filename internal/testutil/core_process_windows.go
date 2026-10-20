//go:build windows

package testutil

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func configureCoreProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error { return stopCoreProcess(cmd) }
	cmd.WaitDelay = 3 * time.Second
}

func stopCoreProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// taskkill /T includes descendants, unlike os.Process.Kill on Windows.
	err := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	if err != nil {
		return errors.Join(err, cmd.Process.Kill())
	}
	return nil
}
