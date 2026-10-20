//go:build !windows

package tools

import "os/exec"

// HideWindow 在非 Windows 平台无需操作
func HideWindow(_ *exec.Cmd) {}
