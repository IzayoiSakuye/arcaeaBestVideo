//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
	"time"
)

// HideWindow 在非 Windows 平台无需操作
func HideWindow(_ *exec.Cmd) {}

func CancelProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 5 * time.Second
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
