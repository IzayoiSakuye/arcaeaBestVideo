package job

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestReusableRejectsFullRangeSegment(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未找到 ffmpeg")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("未找到 ffprobe")
	}
	dir := t.TempDir()
	seg := filepath.Join(dir, "seg.mp4")
	in := SegInput{Clip: "same-input"}
	if err := os.WriteFile(filepath.Join(dir, "input.json"), []byte(in.hash()), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		rangeName string
		reusable  bool
	}{{"pc", false}, {"tv", true}} {
		cmd := exec.Command(ffmpeg, "-y", "-v", "error", "-f", "lavfi", "-i", "color=c=white:s=64x64:r=1",
			"-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-color_range", tc.rangeName, seg)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("生成 %s 测试片段失败：%v：%s", tc.rangeName, err, out)
		}
		if got := reusable(context.Background(), ffprobe, dir, seg, in); got != tc.reusable {
			t.Errorf("%s 范围的缓存复用结果：%v，期望 %v", tc.rangeName, got, tc.reusable)
		}
	}
}

func TestSegmentHashChangesWithTextTemplate(t *testing.T) {
	in := SegInput{TextHash: "old"}
	before := in.hash()
	in.TextHash = "new"
	if before == in.hash() {
		t.Fatal("文字模板修改后应重新渲染片段")
	}
}
