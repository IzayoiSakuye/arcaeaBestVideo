//go:build windows

package tools

import (
	"os/exec"
	"syscall"
)

// HideWindow 防止外部工具短暂弹出控制台
func HideWindow(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW 避免控制台程序启动时短暂弹窗
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
