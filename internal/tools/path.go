// Package tools 定位外部可执行文件（ffmpeg / ffprobe / Edge / yt-dlp）。
package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Tools 是解析出来的可执行文件路径。空串表示没找到。
type Tools struct {
	FFmpeg  string
	FFprobe string
	Edge    string
	// YtDlp 只在下载片段时用得上，缓存命中时不需要
	YtDlp string
}

// Status 是单个工具的探测结果，用于自动检测
type Status struct {
	Name   string
	Path   string // 空 = 没找到
	Source string // override / bundled / path / known / sibling
}

// Detection 是一次完整探测的结果
type Detection struct {
	Tools    Tools
	Statuses []Status
}

// spec 描述工具路径，文件名等
type spec struct {
	name     string
	exe      string   // 自带目录里的文件名（带 .exe）
	lookPath string   // 给 exec.LookPath 的名字；空 = 没有 PATH 名
	known    []string // 候选表；空串会被跳过
	required bool     // 是否必须
}

// toolsDir 用于Bootstrap
func toolsDir() string {
	base, err := os.UserConfigDir() // Windows 上是 %APPDATA%
	if err != nil {
		return ""
	}
	return filepath.Join(base, "rhythm-game-bvideo", "tools")
}

// fromEnv 从环境变量中寻找
func fromEnv(name string, parts ...string) string {
	base := os.Getenv(name)
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, parts...)...)
}

func toolSpecs() []spec {
	return []spec{
		{
			name: "ffmpeg", exe: "ffmpeg.exe", lookPath: "ffmpeg", required: true,
			known: []string{
				`C:\ffmpeg\bin\ffmpeg.exe`,
				fromEnv("LOCALAPPDATA", "Programs", "ffmpeg", "bin", "ffmpeg.exe"),
				fromEnv("LOCALAPPDATA", "Microsoft", "WinGet", "Links", "ffmpeg.exe"),
				fromEnv("ProgramData", "chocolatey", "bin", "ffmpeg.exe"),
				fromEnv("USERPROFILE", "scoop", "shims", "ffmpeg.exe"),
			},
		},
		{
			name: "ffprobe", exe: "ffprobe.exe", lookPath: "", required: true,
		},
		{
			name: "edge", exe: "msedge.exe", lookPath: "", required: true, // ⚠️ 没有 PATH 名
			known: []string{
				`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
				`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			},
		},
		{
			name: "yt-dlp", exe: "yt-dlp.exe", lookPath: "yt-dlp", required: false,
			known: []string{
				filepath.Join(os.TempDir(), "ytdlp-test", "yt-dlp.exe"),
			},
		},
	}
}

func overrideOf(o Tools, name string) string {
	switch name {
	case "ffmpeg":
		return o.FFmpeg
	case "ffprobe":
		return o.FFprobe
	case "edge":
		return o.Edge
	case "yt-dlp":
		return o.YtDlp
	}
	return ""
}

// Detect 用于自动检测工具路径，如果已经输入则直接使用
func Detect(overrides Tools) Detection {
	d := Detection{Statuses: make([]Status, 0, 4)}
	idx := map[string]int{}

	for _, s := range toolSpecs() {
		idx[s.name] = len(d.Statuses)
		d.Statuses = append(d.Statuses, detectOne(s, overrideOf(overrides, s.name)))
	}

	// ffprobe 跟着 ffmpeg 同目录；只有用户显式指定 ffprobe 时才用指定的
	if overrides.FFprobe == "" {
		if ffmpegPath := d.Statuses[idx["ffmpeg"]].Path; ffmpegPath != "" {
			p := filepath.Join(filepath.Dir(ffmpegPath), "ffprobe.exe")
			st := &d.Statuses[idx["ffprobe"]]
			if isFile(p) {
				st.Path, st.Source = p, "sibling"
			}
		}
	}

	for _, st := range d.Statuses {
		switch st.Name {
		case "ffmpeg":
			d.Tools.FFmpeg = st.Path
		case "ffprobe":
			d.Tools.FFprobe = st.Path
		case "edge":
			d.Tools.Edge = st.Path
		case "yt-dlp":
			d.Tools.YtDlp = st.Path
		}
	}
	return d
}

// detectOne 是具体的探测路径，包括
// 1. 用户自己输入   2. Bootstrap   3. 环境变量等路径
func detectOne(s spec, override string) Status {
	st := Status{Name: s.name}

	try := func(path, source string) bool {
		if path == "" {
			return false
		}
		if !isFile(path) {
			return false
		}
		st.Path, st.Source = path, source
		return true
	}

	if try(override, "override") {
		return st
	}

	if dir := toolsDir(); dir != "" {
		if try(filepath.Join(dir, s.exe), "bundled") {
			return st
		}
	}

	if s.lookPath != "" {
		if hit, err := exec.LookPath(s.lookPath); err == nil {
			if abs, err2 := filepath.Abs(hit); err2 == nil && try(abs, "path") {
				return st
			}
		}
	}

	for _, k := range s.known {
		if try(k, "known") {
			return st
		}
	}
	return st
}

// Resolve 在开始渲染前再次检测工具是否完备
func Resolve(d Detection) (Tools, error) {
	required := map[string]bool{}
	for _, s := range toolSpecs() {
		if s.required {
			required[s.name] = true
		}
	}

	var bad []string
	if len(d.Statuses) == 0 {
		for _, x := range []struct{ name, path string }{
			{"ffmpeg", d.Tools.FFmpeg}, {"ffprobe", d.Tools.FFprobe}, {"edge", d.Tools.Edge},
		} {
			if !isFile(x.path) {
				bad = append(bad, fmt.Sprintf("%s 不可用：%q", x.name, x.path))
			}
		}
	} else {
		for _, st := range d.Statuses {
			if !required[st.Name] || isFile(st.Path) {
				continue
			}
			if st.Path == "" {
				bad = append(bad, fmt.Sprintf("%s 未找到", st.Name))
			} else {
				bad = append(bad, fmt.Sprintf("%s 不可用：%s", st.Name, st.Path))
			}
		}
	}

	if len(bad) > 0 {
		return Tools{}, fmt.Errorf(
			"必需工具不可用：%s\n可在设置里点「自动检测」重扫，或手动选择文件", strings.Join(bad, "；"))
	}
	return d.Tools, nil
}

// isFile 判断文件是否存在，且是正常文件而非目录等
func isFile(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false // 不存在 / 没权限 / 路径中间某级不是目录
	}
	return fi.Mode().IsRegular()
}
