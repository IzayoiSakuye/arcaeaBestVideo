package render

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"
)

func TestThoughtRendersTimedPages(t *testing.T) {
	if testing.Short() {
		t.Skip("需要运行 Edge 和 ffmpeg")
	}
	detected := tools.Detect(tools.Tools{}).Tools
	if detected.FFmpeg == "" || detected.FFprobe == "" || detected.Edge == "" {
		t.Skip("缺少 Edge 或 ffmpeg")
	}
	templateDir, err := filepath.Abs(filepath.Join("..", "..", "templates"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(templateDir, "thought-bg.png")); err != nil {
		t.Skip("尚未提供感想背景")
	}
	workDir := t.TempDir()
	audio := filepath.Join(workDir, "test.wav")
	if out, err := exec.Command(detected.FFmpeg, "-y", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", audio).CombinedOutput(); err != nil {
		t.Fatalf("准备音频失败：%v %s", err, out)
	}
	thought := score.Thought{
		Name: "测试感想", Texts: []string{"第一段", "第二段"}, DurationMs: 2000,
		Audio: score.ThoughtAudio{Kind: "custom", StartMs: 500, EndMs: 2500},
	}
	output, err := Thought(context.Background(), thought, audio, templateDir, workDir, detected.FFmpeg, detected.FFprobe, detected.Edge)
	if err != nil {
		t.Fatal(err)
	}
	info, err := Probe(context.Background(), detected.FFprobe, output)
	if err != nil {
		t.Fatal(err)
	}
	if info.CodecName != "h264/aac" || info.AvgFrameRate != "30/1" || info.DurationMs < 1950 || info.DurationMs > 2050 ||
		info.PixFmt != "yuv420p" || info.SAR != "1:1" || info.SampleRate != "48000" || info.Channels != 2 {
		t.Fatalf("感想片段参数错误：%+v", info)
	}
	frameHash := func(path, at string) string {
		out, err := exec.Command(detected.FFmpeg, "-v", "error", "-ss", at, "-i", path, "-frames:v", "1", "-f", "md5", "-").Output()
		if err != nil {
			t.Fatalf("抽取画面失败：%v", err)
		}
		return string(out)
	}
	if frameHash(output, "0.5") == frameHash(output, "1.5") {
		t.Fatal("两段文字没有在对应时间切换")
	}
	thought.Image = &score.ThoughtImage{Path: filepath.Join(templateDir, "thought-bg.png"), X: 1200, Y: 420, Width: 180}
	withImage, err := Thought(context.Background(), thought, audio, templateDir, workDir, detected.FFmpeg, detected.FFprobe, detected.Edge)
	if err != nil {
		t.Fatal(err)
	}
	if frameHash(output, "0.5") == frameHash(withImage, "0.5") {
		t.Fatal("导入的装饰图没有出现在画面中")
	}
	thought.Image = nil
	thought.Texts = nil
	thought.Pages = []score.ThoughtPage{
		{Text: "第一段", Images: []score.ThoughtImage{
			{Path: filepath.Join(templateDir, "thought-bg.png"), X: 100, Y: 400, Width: 180},
			{Path: filepath.Join(templateDir, "thought-bg.png"), X: 1400, Y: 400, Width: 180},
		}},
		{Text: "第二段"},
	}
	multiImage, err := Thought(context.Background(), thought, audio, templateDir, workDir, detected.FFmpeg, detected.FFprobe, detected.Edge)
	if err != nil {
		t.Fatal(err)
	}
	pageImage := func(video string, index int) image.Image {
		path := filepath.Join(workDir, strings.TrimSuffix(filepath.Base(video), ".mp4"), fmt.Sprintf("%02d.png", index))
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		img, err := png.Decode(file)
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	changed := func(a, b image.Image, x, y, w, h int) bool {
		for py := y; py < y+h; py++ {
			for px := x; px < x+w; px++ {
				ar, ag, ab, aa := a.At(px, py).RGBA()
				br, bg, bb, ba := b.At(px, py).RGBA()
				if ar != br || ag != bg || ab != bb || aa != ba {
					return true
				}
			}
		}
		return false
	}
	baseline := pageImage(output, 0)
	first := pageImage(multiImage, 0)
	if !changed(baseline, first, 110, 420, 70, 60) || !changed(baseline, first, 1410, 420, 150, 60) {
		t.Fatal("两张装饰图没有出现在第一段")
	}
	if changed(pageImage(output, 1), pageImage(multiImage, 1), 0, 0, 1920, 1080) {
		t.Fatal("第一段的装饰图影响了第二段")
	}
	thought.Image = nil
	thought.Audio = score.ThoughtAudio{Kind: "silent"}
	thought.DurationMs = 1000
	thought.Pages = []score.ThoughtPage{{Text: "静音感想"}}
	silent, err := Thought(context.Background(), thought, "", templateDir, workDir, detected.FFmpeg, detected.FFprobe, detected.Edge)
	if err != nil {
		t.Fatal(err)
	}
	silentInfo, err := Probe(context.Background(), detected.FFprobe, silent)
	if err != nil || !validThoughtSegment(silentInfo, 1000) {
		t.Fatalf("静音片段参数错误：%+v，%v", silentInfo, err)
	}
}
