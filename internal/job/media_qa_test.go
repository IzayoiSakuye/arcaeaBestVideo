package job

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"
)

func qaRun(t *testing.T, command string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("媒体命令失败：%v %s", err, out)
	}
	return out
}

func qaPNG(t *testing.T, path string, w, h int, c color.NRGBA) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, im); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func qaRMS(t *testing.T, ffmpeg, path string, at, duration float64) float64 {
	t.Helper()
	b := qaRun(t, ffmpeg, "-v", "error", "-ss", fmt.Sprintf("%.3f", at), "-i", path, "-t", fmt.Sprintf("%.3f", duration), "-map", "0:a:0", "-ac", "1", "-f", "f32le", "-")
	if len(b) < 4 {
		t.Fatal("音频测量没有样本")
	}
	sum := 0.0
	for i := 0; i+4 <= len(b); i += 4 {
		v := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[i : i+4])))
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(b)/4))
}

func TestQAActualSegmentsConcatFadeAndCacheRetention(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("截图替身需要 POSIX shell；不代替 Windows WebView2 验收")
	}
	f, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 ffmpeg")
	}
	p, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("需要 ffprobe")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	work := filepath.Join(cache, "pipeline")
	templates := filepath.Join(root, "templates")
	for _, name := range []string{"bg16-9.png", "bg4-3.png", "text.png", "thought-bg.png"} {
		qaPNG(t, filepath.Join(templates, name), 1920, 1080, color.NRGBA{})
	}
	for _, name := range []string{"text.html", "thought.html"} {
		if err := os.WriteFile(filepath.Join(templates, name), []byte("QA fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	qaPNG(t, filepath.Join(root, "data", "bg.png"), 160, 90, color.NRGBA{255, 255, 255, 255})
	qaPNG(t, filepath.Join(root, "data", "jackets", "256", "complete.jpg"), 160, 159, color.NRGBA{255, 255, 255, 255})
	// 只替换浏览器截图；合成、探测、音频解码和拼接均运行真实 FFmpeg。
	edge := filepath.Join(root, "edge")
	if err := os.WriteFile(edge, []byte("#!/bin/sh\nfor arg in \"$@\"; do\ncase \"$arg\" in --screenshot=*) cp \"$QA_SCREENSHOT\" \"${arg#--screenshot=}\";; esac\ndone\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QA_SCREENSHOT", filepath.Join(templates, "text.png"))
	source := filepath.Join(root, "original.mp4")
	qaRun(t, f, "-y", "-v", "error", "-f", "lavfi", "-i", "color=white:s=160x90:r=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "3", "-c:v", "libx264", "-c:a", "aac", source)
	cata, err := catalog.Load(filepath.Join("..", "catalog", "testdata", "catalog-mini.json"), 1)
	if err != nil {
		t.Fatal(err)
	}
	bg := &resource.BgManifest{Songs: map[string]resource.SongBg{"complete": {Bg: "qa"}}, Bgs: map[string]resource.BgEntry{"qa": {Files: map[string]resource.BgFile{"960": {Path: "data/bg.png"}}}}}
	proj := &score.Project{SchemaVersion: 1, Game: "arcaea", Thoughts: []score.Thought{{ID: "qa", Name: "感想", Texts: []string{"QA"}, DurationMs: 1000, Audio: score.ThoughtAudio{Kind: "custom", Path: source, StartMs: 500, EndMs: 1500}}}}
	for i := 1; i <= 3; i++ {
		song := score.Song{Index: i, SongID: "complete", Difficulty: score.DiffRef{Name: "FTR"}, Video: &score.VideoRef{LocalPath: source}, Clip: &score.ClipRange{StartMs: 500, DurationMs: 1000}}
		song.Clip.ConfirmedKey = score.ClipKey(song)
		proj.Songs = append(proj.Songs, song)
		proj.PlayOrder = append(proj.PlayOrder, score.PlayItem{Kind: "song", SongIndex: i})
	}
	proj.PlayOrder = append(proj.PlayOrder, score.PlayItem{Kind: "thought", ThoughtID: "qa"})
	cfg := Config{Project: proj, Root: root, DataDir: filepath.Join(root, "data"), CacheDir: cache, WorkDir: work, ClipsDir: filepath.Join(cache, "clips"), Output: filepath.Join(root, "result.mp4")}
	deps := Deps{Catalog: cata, Bg: bg, Tools: tools.Tools{FFmpeg: f, FFprobe: p, Edge: edge}}
	confirmed := proj.Songs[0].Clip
	proj.Songs[0].Clip = nil
	if _, err := NewJob(deps, cfg, nil).Run(context.Background()); err == nil || !strings.Contains(err.Error(), "片段尚未保存") {
		t.Fatalf("pipeline 入口未拒绝未确认范围：%v", err)
	}
	proj.Songs[0].Clip = confirmed
	res, err := NewJob(deps, cfg, nil).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Rendered != 3 || len(res.Segments) != 4 {
		t.Fatalf("歌曲或感想未生成：%+v", res)
	}
	for i, path := range res.Segments {
		info, err := render.Probe(context.Background(), p, path)
		if err != nil {
			t.Fatal(err)
		}
		if info.CodecName != "h264/aac" || info.PixFmt != "yuv420p" || info.Width != 1920 || info.Height != 1080 || info.AvgFrameRate != "30/1" || info.SampleRate != "48000" || info.Channels != 2 || info.DurationMs < 950 || info.DurationMs > 1100 {
			t.Errorf("seg %d 参数错误：%+v", i, info)
		}
		start, mid, end := qaRMS(t, f, path, 0, .03), qaRMS(t, f, path, .45, .1), qaRMS(t, f, path, .97, .03)
		if mid < .03 {
			t.Errorf("seg %d 中段音频异常：%f", i, mid)
		}
		if i > 0 && start > mid*.35 || i == 0 && start < mid*.6 {
			t.Errorf("seg %d 音频首边界错误：start=%f mid=%f", i, start, mid)
		}
		if i < 3 && end > mid*.35 || i == 3 && end < mid*.6 {
			t.Errorf("seg %d 音频尾边界错误：end=%f mid=%f", i, end, mid)
		}
		t.Logf("seg %d：%dms；音频 RMS 首/中/尾 %.4f/%.4f/%.4f", i, info.DurationMs, start, mid, end)
	}
	brightness := func(at string) byte {
		b := qaRun(t, f, "-v", "error", "-ss", at, "-i", res.Segments[0], "-frames:v", "1", "-vf", "scale=1:1,format=gray", "-f", "rawvideo", "-")
		if len(b) != 1 {
			t.Fatal("画面测量失败")
		}
		return b[0]
	}
	first, middle, last := brightness("0"), brightness("0.5"), brightness("0.933")
	// 30fps 的 250ms 淡化会取整到帧，尾部取样允许一帧的亮度差。
	if first > 10 || middle < 80 || int(last) > int(middle)*45/100 {
		t.Errorf("短片段画面淡化错误：%d/%d/%d", first, middle, last)
	}
	info, err := render.Probe(context.Background(), p, cfg.Output)
	if err != nil || info.DurationMs < 3950 || info.DurationMs > 4200 {
		t.Fatalf("concat 产物时长错误：%+v %v", info, err)
	}
	for _, at := range []float64{.97, 1.97, 2.97} {
		if rms := qaRMS(t, f, cfg.Output, at, .06); rms > .03 {
			t.Errorf("concat 音频交界未淡化：at=%.2f RMS=%f", at, rms)
		}
	}
	t.Logf("concat 实际时长=%dms，三个音频交界均低于 RMS 0.03", info.DurationMs)
	// 移动中间歌曲不改变首尾音频边界，全部 seg 都应复用。
	proj.PlayOrder[1], proj.PlayOrder[2] = proj.PlayOrder[2], proj.PlayOrder[1]
	second, err := NewJob(deps, cfg, nil).Run(context.Background())
	if err != nil || second.Rendered != 0 {
		t.Fatalf("中间换序不应重渲染：%d %v", second.Rendered, err)
	}
	// 感想移到首位，只改变歌曲 1 的淡入和歌曲 2 的淡出。
	proj.PlayOrder = []score.PlayItem{{Kind: "thought", ThoughtID: "qa"}, {Kind: "song", SongIndex: 1}, {Kind: "song", SongIndex: 3}, {Kind: "song", SongIndex: 2}}
	third, err := NewJob(deps, cfg, nil).Run(context.Background())
	if err != nil || third.Rendered != 2 {
		t.Fatalf("首尾变化应仅重渲染两首：%d %v", third.Rendered, err)
	}
	cfg.Output = filepath.Join(root, "missing-parent", "fail.mp4")
	failed, err := NewJob(deps, cfg, nil).Run(context.Background())
	if err == nil {
		t.Fatal("输出父目录缺失应明确失败")
	}
	if failed.Rendered != 0 {
		t.Error("失败重试不应丢失缓存")
	}
	for _, path := range third.Segments {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("失败后丢失有效 seg：%v", err)
		}
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("原文件被移除")
	}
	if _, err := os.Stat(filepath.Join(root, "result.mp4")); err != nil {
		t.Fatal("先前成片丢失")
	}
	if err := filepath.WalkDir(cache, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), "fade_") || strings.HasSuffix(path, ".mp3") || strings.Contains(entry.Name(), ".part") {
			t.Errorf("出现重复淡化或临时媒体：%s", entry.Name())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
