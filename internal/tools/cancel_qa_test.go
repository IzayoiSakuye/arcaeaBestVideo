//go:build linux

package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestQACancelTerminatesFFmpegChild(t *testing.T) {
	f, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 ffmpeg")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "sidecar")
	pidFile := filepath.Join(dir, "child.pid")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n\"$QA_FFMPEG\" -v error -re -f lavfi -i sine=frequency=440 -t 60 -f null - >/dev/null 2>&1 &\nchild=$!\necho \"$child\" > \"$QA_CHILD_PID\"\nwait \"$child\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, script)
	cmd.Env = append(os.Environ(), "QA_FFMPEG="+f, "QA_CHILD_PID="+pidFile)
	CancelProcessTree(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		cancel()
		if !waited {
			_ = cmd.Wait()
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	var pid int
	for pid == 0 {
		b, err := os.ReadFile(pidFile)
		if err == nil {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(b)))
		}
		if time.Now().After(deadline) {
			t.Fatal("没有启动子进程")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	err = cmd.Wait()
	waited = true
	if err == nil {
		t.Fatal("取消后命令仍成功")
	}
	deadline = time.Now().Add(5 * time.Second)
	for {
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(b))
		if len(fields) > 2 && fields[2] == "Z" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("FFmpeg 子进程 %d 仍在运行：%s", pid, b)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
