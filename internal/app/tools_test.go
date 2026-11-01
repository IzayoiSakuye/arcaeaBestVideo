package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"rhythm-game-bvideo/internal/tools"
)

func TestToolPathsPersistAndRejectBadOverride(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("UserConfigDir 的位置与平台有关")
	}
	t.Setenv("APPDATA", t.TempDir())
	want := tools.Tools{FFmpeg: `C:\tools\ffmpeg.exe`}
	if err := saveToolPaths(want); err != nil {
		t.Fatal(err)
	}
	got, err := loadToolPaths()
	if err != nil || got.FFmpeg != want.FFmpeg {
		t.Fatalf("工具设置未保存：%+v, %v", got, err)
	}
	if _, err := (&App{}).SetToolPath("ffmpeg", filepath.Join(t.TempDir(), "missing.exe")); err == nil {
		t.Fatal("不存在的工具不应保存")
	}
	got, err = loadToolPaths()
	if err != nil || got.FFmpeg != want.FFmpeg {
		t.Fatalf("失败设置覆盖了旧路径：%+v, %v", got, err)
	}
	file, err := toolPathsFile()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
}

func TestAutoDetectFailureKeepsSavedPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("UserConfigDir 的位置与平台有关")
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("PATH", "")
	want := tools.Tools{YtDlp: filepath.Join(t.TempDir(), "yt-dlp.exe")}
	if err := saveToolPaths(want); err != nil {
		t.Fatal(err)
	}
	if _, err := (&App{}).SetToolPath("yt-dlp", ""); err == nil {
		t.Fatal("自动检测无结果时应报错")
	}
	got, err := loadToolPaths()
	if err != nil || got.YtDlp != want.YtDlp {
		t.Fatalf("自动检测失败不应删除手动路径：%+v, %v", got, err)
	}
}
