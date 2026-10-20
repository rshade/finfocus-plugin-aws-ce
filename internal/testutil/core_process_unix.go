//go:build !windows

package testutil

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureCoreProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return stopCoreProcess(cmd) }
	cmd.WaitDelay = 3 * time.Second
}

func stopCoreProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
