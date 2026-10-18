package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Check 实际启动工具测试
func Check(name, path string) (string, error) {
	if !isFile(path) {
		return "", fmt.Errorf("%s 不可用：%s", name, path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	if name == "edge" {
		dir, err := os.MkdirTemp("", "rhythm-edge-check-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)
		shot := filepath.Join(dir, "check.png")
		cmd := exec.CommandContext(ctx, path, "--headless=new", "--disable-gpu", "--no-first-run",
			"--user-data-dir="+filepath.Join(dir, "profile"), "--screenshot="+shot, "--window-size=16,16", "about:blank")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("Edge 截图测试失败：%w：%s", err, strings.TrimSpace(string(out)))
		}
		if fi, err := os.Stat(shot); err != nil || fi.Size() == 0 {
			return "", fmt.Errorf("Edge 截图测试未生成图片")
		}
		return "截图测试通过", nil
	}
	var flag, prefix string
	switch name {
	case "ffmpeg", "ffprobe":
		flag, prefix = "-version", name+" version"
	case "yt-dlp":
		flag = "--version"
	default:
		return "", fmt.Errorf("未知工具：%s", name)
	}
	out, err := exec.CommandContext(ctx, path, flag).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s 测试失败：%w：%s", name, err, strings.TrimSpace(string(out)))
	}
	first := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if first == "" || (prefix != "" && !strings.HasPrefix(first, prefix)) {
		return "", fmt.Errorf("%s 版本输出不符合预期：%s", name, first)
	}
	return first, nil
}
