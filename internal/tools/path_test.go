package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// selfExe 是当前测试二进制的路径 —— 一个保证存在的常规文件，
// 用来当"用户填了一个有效路径"的替身（不依赖机器上装了什么）。
func selfExe(t *testing.T) string {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatalf("拿不到测试可执行文件路径: %v", err)
	}
	return p
}

func statusOf(d Detection, name string) Status {
	for _, s := range d.Statuses {
		if s.Name == name {
			return s
		}
	}
	return Status{}
}

// 本机 Detect(Tools{}) 应把必需项都找到
func TestDetectFindsRequiredTools(t *testing.T) {
	d := Detect(Tools{})
	for _, st := range d.Statuses {
		t.Logf("%-8s source=%-9s %s", st.Name, st.Source, st.Path)
	}
	for _, name := range []string{"ffmpeg", "ffprobe", "edge"} {
		if st := statusOf(d, name); st.Path == "" {
			t.Errorf("%s 没找到", name)
		}
	}
}

// 返回的路径必须是绝对路径
func TestDetectPathsAreAbsolute(t *testing.T) {
	for _, st := range Detect(Tools{}).Statuses {
		if st.Path != "" && !filepath.IsAbs(st.Path) {
			t.Errorf("%s 的路径不是绝对路径: %q", st.Name, st.Path)
		}
	}
}

// ffprobe 必须跟着 ffmpeg 同目录，而不是自己走 PATH
func TestFfprobeFollowsFfmpeg(t *testing.T) {
	d := Detect(Tools{})
	ff, fp := statusOf(d, "ffmpeg"), statusOf(d, "ffprobe")
	if ff.Path == "" {
		t.Skip("本机没找到 ffmpeg，跳过")
	}
	if fp.Source != "sibling" {
		t.Errorf("ffprobe 的 source 应为 sibling，实际 %q", fp.Source)
	}
	if want := filepath.Join(filepath.Dir(ff.Path), "ffprobe.exe"); fp.Path != want {
		t.Errorf("ffprobe 应跟 ffmpeg 同目录\n  want %s\n  got  %s", want, fp.Path)
	}
}

// 框里填了有效路径 → 直接采用，不走自动查找
func TestOverrideWins(t *testing.T) {
	exe := selfExe(t)
	st := statusOf(Detect(Tools{FFmpeg: exe}), "ffmpeg")
	if st.Path != exe || st.Source != "override" {
		t.Errorf("显式指定应直接采用，实际 path=%q source=%q", st.Path, st.Source)
	}
}

// 框里填了失效路径 → 不采用它，继续自动查找（自愈）
func TestInvalidOverrideFallsThrough(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "不存在.exe")
	st := statusOf(Detect(Tools{FFmpeg: bad}), "ffmpeg")
	if st.Source == "override" {
		t.Error("失效的覆盖不该被采用")
	}
	if st.Path == bad {
		t.Errorf("不该回传失效路径，实际 %q", st.Path)
	}
}

// 用户显式指定 ffprobe 时，不该再被 sibling 规则覆盖
func TestExplicitFfprobeWinsOverSibling(t *testing.T) {
	exe := selfExe(t)
	st := statusOf(Detect(Tools{FFprobe: exe}), "ffprobe")
	if st.Path != exe || st.Source != "override" {
		t.Errorf("显式指定的 ffprobe 不该被覆盖，实际 path=%q source=%q", st.Path, st.Source)
	}
}

// ⚠️ os.Stat 对目录也返回 nil error —— 不能只判 err
func TestIsFileRejectsDirectory(t *testing.T) {
	if isFile(t.TempDir()) {
		t.Error("目录不该被判为文件")
	}
}

func TestIsFileRejectsEmptyAndMissing(t *testing.T) {
	if isFile("") {
		t.Error(`isFile("") 应为 false`)
	}
	if isFile(filepath.Join(t.TempDir(), "nope.exe")) {
		t.Error("不存在的路径应为 false")
	}
}

func TestIsFileAcceptsRegularFile(t *testing.T) {
	if !isFile(selfExe(t)) {
		t.Error("测试二进制本身应是常规文件")
	}
}

// Resolve 是守门：必需项缺了必须报错（哪怕是空值）
func TestResolveRejectsMissingRequired(t *testing.T) {
	if _, err := Resolve(Detection{}); err == nil {
		t.Fatal("三个必需项全缺时应报错")
	}
	// 经过 Detect 的路径也要拦：把 ffmpeg 清空
	d := Detect(Tools{})
	for i := range d.Statuses {
		if d.Statuses[i].Name == "ffmpeg" {
			d.Statuses[i].Path = ""
		}
	}
	d.Tools.FFmpeg = ""
	if _, err := Resolve(d); err == nil {
		t.Fatal("缺 ffmpeg 时应报错")
	}
}

// 只缺 yt-dlp 必须成功
func TestResolveAllowsMissingYtDlp(t *testing.T) {
	exe := selfExe(t)
	got, err := Resolve(Detection{Tools: Tools{FFmpeg: exe, FFprobe: exe, Edge: exe}})
	if err != nil {
		t.Fatalf("只缺 yt-dlp 不该报错: %v", err)
	}
	if got.YtDlp != "" {
		t.Errorf("YtDlp 应为空，实际 %q", got.YtDlp)
	}
}

// 错误信息要点名是哪个工具缺了，并给出出路
func TestResolveErrorNamesTool(t *testing.T) {
	d := Detection{Statuses: []Status{
		{Name: "ffmpeg"},
		{Name: "ffprobe", Path: selfExe(t), Source: "override"},
		{Name: "edge", Path: selfExe(t), Source: "override"},
	}}
	_, err := Resolve(d)
	if err == nil {
		t.Fatal("缺 ffmpeg 时应报错")
	}
	if msg := err.Error(); !strings.Contains(msg, "ffmpeg") || !strings.Contains(msg, "自动检测") {
		t.Errorf("错误信息应含工具名与出路提示，实际：%v", err)
	}
}

// 这一条是"环境体检"：本机齐了就应通过
func TestResolvePassesWhenAllPresent(t *testing.T) {
	got, err := Resolve(Detect(Tools{}))
	if err != nil {
		t.Skipf("本机缺工具，无法验证通过路径：%v", err)
	}
	if got.FFmpeg == "" || got.FFprobe == "" || got.Edge == "" {
		t.Error("Resolve 通过时不该有空路径")
	}
}
