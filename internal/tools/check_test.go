package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCheckRejectsInvalidTool(t *testing.T) {
	if _, err := Check("ffmpeg", filepath.Join(t.TempDir(), "missing.exe")); err == nil {
		t.Fatal("缺失的工具应被拒绝")
	}
	if runtime.GOOS == "windows" {
		return
	}
	path := filepath.Join(t.TempDir(), "fake-tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho not-ffmpeg\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Check("ffmpeg", path); err == nil {
		t.Fatal("错误的版本输出应被拒绝")
	}
}

func TestCheckDetectedFFmpeg(t *testing.T) {
	path := Detect(Tools{}).Tools.FFmpeg
	if path == "" {
		t.Skip("未安装 ffmpeg")
	}
	line, err := Check("ffmpeg", path)
	if err != nil {
		t.Fatal(err)
	}
	if line == "" {
		t.Fatal("没有版本信息")
	}
}

func TestWaitForScreenshotAfterProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "check.png")
	go func() {
		time.Sleep(150 * time.Millisecond)
		_ = os.WriteFile(path, []byte("png"), 0o600)
	}()
	if !waitForScreenshot(context.Background(), path) {
		t.Fatal("未等待延迟写出的截图")
	}
}
