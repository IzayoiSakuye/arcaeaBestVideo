//go:build windows

package tools

import (
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// HideWindow 防止外部工具短暂弹出控制台
func HideWindow(cmd *exec.Cmd) {
	// CREATE_NO_WINDOW 避免控制台程序启动时短暂弹窗
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// CancelProcessTree 防止 yt-dlp 退出后其 FFmpeg 子进程继续写入临时文件。
func CancelProcessTree(cmd *exec.Cmd) {
	cmd.WaitDelay = 5 * time.Second
	cmd.Cancel = func() error {
		kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		HideWindow(kill)
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
