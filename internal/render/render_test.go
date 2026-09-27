package render

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ── fingerprint ──────────────────────────────────────────────────────────────

func TestFingerprintIsSensitiveToEveryField(t *testing.T) {
	base := StreamInfo{
		CodecName: "h264/aac", Width: 1920, Height: 1080,
		PixFmt: "yuv420p", AvgFrameRate: "30/1", SAR: "1:1",
		SampleRate: "48000", Channels: 2,
	}
	// 逐个字段改掉，指纹都必须变 —— 少一个就是校验盲区
	mutate := map[string]func(*StreamInfo){
		"CodecName":    func(s *StreamInfo) { s.CodecName = "h264/mp3" },
		"Width":        func(s *StreamInfo) { s.Width = 1280 },
		"Height":       func(s *StreamInfo) { s.Height = 720 },
		"PixFmt":       func(s *StreamInfo) { s.PixFmt = "yuv444p" },
		"AvgFrameRate": func(s *StreamInfo) { s.AvgFrameRate = "60/1" },
		"SAR":          func(s *StreamInfo) { s.SAR = "4:3" },
		"SampleRate":   func(s *StreamInfo) { s.SampleRate = "44100" },
		"Channels":     func(s *StreamInfo) { s.Channels = 1 },
	}
	want := fingerprint(base)
	for name, fn := range mutate {
		got := base
		fn(&got)
		if fingerprint(got) == want {
			t.Errorf("改掉 %s 之后指纹没变 —— 这个字段没有被校验", name)
		}
	}
}

func TestFingerprintStableForSameInput(t *testing.T) {
	s := StreamInfo{CodecName: "h264/aac", Width: 1920, Height: 1080, PixFmt: "yuv420p",
		AvgFrameRate: "30/1", SAR: "1:1", SampleRate: "48000", Channels: 2}
	if fingerprint(s) != fingerprint(s) {
		t.Error("同样的输入两次指纹不同")
	}
	t.Log(fingerprint(s))
}

// ── concatList ───────────────────────────────────────────────────────────────

func TestConcatListFormat(t *testing.T) {
	list := filepath.Join(t.TempDir(), "concat.txt")
	segs := []string{
		`D:\work\001-a\seg_001.mp4`,
		`D:/work/002-b/seg_002.mp4`,
	}
	if err := concatList(list, segs); err != nil {
		t.Fatalf("concatList: %v", err)
	}
	b, err := os.ReadFile(list)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	t.Logf("产出：\n%s", got)

	// ① 反斜杠必须换成正斜杠（ffmpeg 把 \ 当转义符）
	if strings.Contains(got, `\`) {
		t.Error("清单里还有反斜杠 —— ffmpeg 会把它当转义符")
	}
	// ② 每行是 file '...'
	for _, want := range []string{
		"file 'D:/work/001-a/seg_001.mp4'",
		"file 'D:/work/002-b/seg_002.mp4'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("缺少行 %q", want)
		}
	}
	// ③ 顺序不能乱
	if strings.Index(got, "001-a") > strings.Index(got, "002-b") {
		t.Error("顺序错了")
	}
	// ④ 末尾要有换行
	if !strings.HasSuffix(got, "\n") {
		t.Error("末尾缺换行")
	}
}

func TestConcatListEscapesSingleQuote(t *testing.T) {
	list := filepath.Join(t.TempDir(), "concat.txt")
	if err := concatList(list, []string{`D:\work\it's here\seg.mp4`}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(list)
	got := string(b)
	t.Logf("%q", got)
	if strings.Contains(got, "it's") {
		t.Error("裸单引号没转义，会提前结束 file '...'")
	}
	if !strings.Contains(got, `it'\''s`) {
		t.Errorf("转义形式不对：%q", got)
	}
}

// ── 模板常量 ─────────────────────────────────────────────────────────────────

func TestTemplatesMatchMeasuredConstants(t *testing.T) {
	// 实测值：16:9 与 4:3 两套模板
	cases := []struct {
		wide bool
		want RectWin
		src  string
	}{
		{true, RectWin{20, 157, 934, 526}, "templates/bg16-9.png"},
		{false, RectWin{58, 95, 855, 641}, "templates/bg4-3.png"},
	}
	for _, c := range cases {
		got := Templates[c.wide]
		if got.Win != c.want {
			t.Errorf("Templates[%v].Win = %+v, 期望 %+v", c.wide, got.Win, c.want)
		}
		if got.Source != c.src {
			t.Errorf("Templates[%v].Source = %q, 期望 %q", c.wide, got.Source, c.src)
		}
	}
	if JacketSlot != (RectWin{1289, 312, 160, 159}) {
		t.Errorf("JacketSlot = %+v", JacketSlot)
	}
}

// ── Probe（真跑 ffprobe）─────────────────────────────────────────────────────

func TestProbeRealFile(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("PATH 里没有 ffprobe")
	}
	sample := filepath.Join("..", "..", "dist", "sample", "sample.mp4")
	if _, err := os.Stat(sample); err != nil {
		t.Skipf("没有样片可测（%s）—— 先跑一次 spike", sample)
	}

	info, err := Probe(context.Background(), ffprobe, sample)
	if err != nil {
		t.Fatalf("Probe 失败: %v", err)
	}
	t.Logf("%+v", info)

	if info.CodecName != "h264/aac" {
		t.Errorf("CodecName = %q，期望 h264/aac（视频+音频两条流都要纳入校验）", info.CodecName)
	}
	if info.Width != 1920 || info.Height != 1080 {
		t.Errorf("尺寸 = %dx%d", info.Width, info.Height)
	}
	if info.PixFmt != "yuv420p" {
		t.Errorf("PixFmt = %q", info.PixFmt)
	}
	if info.AvgFrameRate != "30/1" {
		t.Errorf("AvgFrameRate = %q", info.AvgFrameRate)
	}
	if info.SAR != "1:1" {
		t.Errorf("SAR = %q", info.SAR)
	}
	if info.SampleRate != "48000" {
		t.Errorf("SampleRate = %q（注意它是 JSON 字符串）", info.SampleRate)
	}
	if info.Channels != 2 {
		t.Errorf("Channels = %d", info.Channels)
	}
}

// ── 直方图：Probe 的原始输出长什么样（诊断用）─────────────────────────────────

func TestProbeRawShape(t *testing.T) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("PATH 里没有 ffprobe")
	}
	sample := filepath.Join("..", "..", "dist", "sample", "sample.mp4")
	if _, err := os.Stat(sample); err != nil {
		t.Skip("没有样片")
	}
	out, err := exec.Command(ffprobe, "-v", "error", "-print_format", "json",
		"-show_entries", "stream=codec_type,codec_name,sample_rate,channels", sample).Output()
	if err != nil {
		t.Fatalf("ffprobe: %v", err)
	}
	var r map[string]any
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	streams, _ := r["streams"].([]any)
	t.Logf("流数 = %d", len(streams))
	for i, s := range streams {
		m, _ := s.(map[string]any)
		t.Logf("  [%d] codec_type=%v sample_rate=%v(%T) channels=%v(%T)",
			i, m["codec_type"], m["sample_rate"], m["sample_rate"], m["channels"], m["channels"])
	}
}
