package app

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type ThoughtMedia struct {
	Path       string `json:"path"`
	PreviewURL string `json:"preview_url"`
	DurationMs int    `json:"duration_ms"`
}

type ThoughtAudioOption struct {
	Kind      string `json:"kind"`
	Available bool   `json:"available"`
	ThoughtMedia
}

// PickThoughtImage 用于用户选择想要加入感想的图片
func (a *App) PickThoughtImage() (ThoughtMedia, error) {
	path, err := runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{Filters: []runtime.FileFilter{
		{DisplayName: "图片 (*.png;*.jpg;*.jpeg)", Pattern: "*.png;*.jpg;*.jpeg"},
	}})
	if err != nil || path == "" {
		return ThoughtMedia{}, err
	}
	return a.ThoughtImage(path)
}

// ThoughtImage 是装饰图逻辑
func (a *App) ThoughtImage(path string) (ThoughtMedia, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ThoughtMedia{}, err
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return ThoughtMedia{}, fmt.Errorf("装饰图只支持 PNG 或 JPEG")
	}
	f, err := os.Open(abs)
	if err != nil {
		return ThoughtMedia{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 20<<20 {
		return ThoughtMedia{}, fmt.Errorf("装饰图必须是小于 20 MB 的普通文件")
	}
	config, format, err := image.DecodeConfig(f)
	if err != nil || (format != "png" && format != "jpeg") || config.Width <= 0 || config.Height <= 0 {
		return ThoughtMedia{}, fmt.Errorf("装饰图文件无效")
	}
	return ThoughtMedia{Path: abs, PreviewURL: a.mediaURL(abs, ext)}, nil
}

// PickThoughtAudio 提供给用户自选bgm
func (a *App) PickThoughtAudio() (ThoughtMedia, error) {
	path, err := runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{Filters: []runtime.FileFilter{
		{DisplayName: "音频 (*.mp3;*.wav;*.flac;*.m4a;*.ogg)", Pattern: "*.mp3;*.wav;*.flac;*.m4a;*.ogg"},
	}})
	if err != nil || path == "" {
		return ThoughtMedia{}, err
	}
	return a.ThoughtAudio(path)
}

// ThoughtAudio 是设置感想bgm的逻辑
func (a *App) ThoughtAudio(path string) (ThoughtMedia, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ThoughtMedia{}, err
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if !strings.Contains("|.mp3|.wav|.flac|.m4a|.ogg|", "|"+ext+"|") {
		return ThoughtMedia{}, fmt.Errorf("不支持的音频格式")
	}
	fi, err := os.Stat(abs)
	if err != nil || !fi.Mode().IsRegular() {
		return ThoughtMedia{}, fmt.Errorf("音频文件不存在")
	}
	overrides, err := loadToolPaths()
	if err != nil {
		return ThoughtMedia{}, err
	}
	ffprobe := tools.Detect(overrides).Tools.FFprobe
	if ffprobe == "" {
		return ThoughtMedia{}, fmt.Errorf("请先配置 ffprobe")
	}
	ctx, cancel := context.WithTimeout(a.context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=index", "-of", "default=noprint_wrappers=1:nokey=1", abs)
	tools.HideWindow(cmd)
	stream, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(stream)) == "" {
		return ThoughtMedia{}, fmt.Errorf("音频文件没有可用音轨")
	}
	cmd = exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "a:0", "-show_entries", "stream=duration", "-of", "default=noprint_wrappers=1:nokey=1", abs)
	tools.HideWindow(cmd)
	out, err := cmd.Output()
	seconds, parseErr := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || parseErr != nil || seconds <= 0 {
		cmd = exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "a:0", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", abs)
		tools.HideWindow(cmd)
		out, err = cmd.Output()
		seconds, parseErr = strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	}
	if err != nil {
		return ThoughtMedia{}, fmt.Errorf("无法读取音频时长：%w", err)
	}
	if parseErr != nil || seconds <= 0 {
		return ThoughtMedia{}, fmt.Errorf("无法读取音频时长")
	}
	return ThoughtMedia{Path: abs, PreviewURL: a.mediaURL(abs, ext), DurationMs: int(seconds * 1000)}, nil
}

// ThoughtAudioOption 是感想bgm的选择框
func (a *App) ThoughtAudioOptions() ([]ThoughtAudioOption, error) {
	out := make([]ThoughtAudioOption, 0, 7)
	for i := 1; i <= 7; i++ {
		kind := fmt.Sprintf("op%d", i)
		option := ThoughtAudioOption{Kind: kind}
		if path := opAudioPath(kind); path != "" {
			media, err := a.ThoughtAudio(path)
			if err == nil {
				option.ThoughtMedia, option.Available = media, true
				out = append(out, option)
				continue
			}
		}
		for _, ext := range []string{".mp3", ".wav", ".flac", ".m4a", ".ogg"} {
			path := filepath.Join(a.root, "templates", "audio", kind+ext)
			if _, err := os.Stat(path); err == nil {
				media, err := a.ThoughtAudio(path)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", strings.ToUpper(kind), err)
				}
				option.ThoughtMedia, option.Available = media, true
				break
			}
		}
		out = append(out, option)
	}
	return out, nil
}

type ThoughtPreview struct {
	PreviewURL string `json:"preview_url"`
	DurationMs int    `json:"duration_ms"`
}

// RenderThought 是渲染感想片段的逻辑
func (a *App) RenderThought(thought score.Thought) (ThoughtPreview, error) {
	overrides, err := loadToolPaths()
	if err != nil {
		return ThoughtPreview{}, err
	}
	tls, err := tools.Resolve(tools.Detect(overrides))
	if err != nil {
		return ThoughtPreview{}, err
	}
	var audioPath string
	if thought.Audio.Kind != "silent" {
		if thought.Audio.Kind == "custom" {
			audioPath = thought.Audio.Path
		} else if strings.HasPrefix(thought.Audio.Kind, "op") {
			options, err := a.ThoughtAudioOptions()
			if err != nil {
				return ThoughtPreview{}, err
			}
			for _, option := range options {
				if option.Kind == thought.Audio.Kind && option.Available {
					audioPath = option.Path
				}
			}
		}
		if audioPath == "" {
			return ThoughtPreview{}, fmt.Errorf("所选背景音乐尚未提供")
		}
		media, err := a.ThoughtAudio(audioPath)
		if err != nil {
			return ThoughtPreview{}, err
		}
		if thought.Audio.StartMs < 0 || thought.Audio.EndMs <= thought.Audio.StartMs || thought.Audio.EndMs > media.DurationMs+50 {
			return ThoughtPreview{}, fmt.Errorf("背景音乐截取范围无效")
		}
		if thought.Audio.Kind == "custom" {
			thought.DurationMs = thought.Audio.EndMs - thought.Audio.StartMs
		}
	}
	for i, page := range thought.EffectivePages() {
		for _, image := range page.Images {
			if _, err := a.ThoughtImage(image.Path); err != nil {
				return ThoughtPreview{}, fmt.Errorf("第 %d 段装饰图：%w", i+1, err)
			}
		}
	}
	output, err := render.Thought(a.context(), thought, audioPath, filepath.Join(a.root, "templates"), filepath.Join(a.root, ".cache", "thought"), tls.FFmpeg, tls.FFprobe, tls.Edge)
	if err != nil {
		return ThoughtPreview{}, err
	}
	return ThoughtPreview{PreviewURL: a.previewURL(output), DurationMs: thought.DurationMs}, nil
}
