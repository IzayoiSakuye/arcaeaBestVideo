package job

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

// renderOne 渲染单首歌片段
func (j *Job) renderOne(ctx context.Context, i int, s score.Song, fades ...render.AudioFade) (string, RenderPhaseDuration, bool, error) {
	var fade render.AudioFade
	if len(fades) > 0 {
		fade = fades[0]
	}
	var d RenderPhaseDuration
	root := j.cfg.Root
	dataRoot := filepath.Dir(j.cfg.DataDir)

	dir := filepath.Join(j.cfg.WorkDir, fmt.Sprintf("%03d-%s", i, s.SongID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", d, false, fmt.Errorf("创建渲染目录失败：%w", err)
	}
	seg := filepath.Join(dir, fmt.Sprintf("seg_%03d.mp4", i))

	song, ok := j.deps.Catalog.Song(s.SongID)
	if !ok {
		return "", d, false, fmt.Errorf("曲库中找不到歌曲 %s", s.SongID)
	}
	chart, ok := j.deps.Catalog.Chart(s.SongID, s.Difficulty.Name)
	if !ok {
		return "", d, false, fmt.Errorf("歌曲 %s 缺少 %s 谱面", s.SongID, s.Difficulty.Name)
	}
	jacket, err := j.deps.Catalog.JacketPath(s.SongID, s.Difficulty.Name)
	if err != nil {
		return "", d, false, fmt.Errorf("获取曲绘路径失败：%w", err)
	}
	bg, err := j.deps.Bg.BgPath(s.SongID, "960")
	if err != nil {
		return "", d, false, fmt.Errorf("获取背景路径失败：%w", err)
	}
	clip, err := j.videoClip(ctx, s, chart)
	if err != nil {
		return "", d, false, err
	}
	info, err := render.Probe(ctx, j.deps.Tools.FFprobe, clip)
	if err != nil {
		return "", d, false, err
	}
	startMs, durationMs := s.Clip.StartMs, s.Clip.DurationMs
	if s.Video.LocalPath == "" && strings.HasPrefix(s.Video.URL, "https://www.bilibili.com/") {
		startMs = 0
	}
	if startMs < 0 || durationMs < 1000 || startMs > info.DurationMs || durationMs > info.DurationMs-startMs+100 {
		return "", d, false, fmt.Errorf("第 %d 首片段超出素材时长，请重新准备素材", i)
	}
	width, height := info.Width, info.Height
	tpl := render.Templates[resource.TemplateFor(&catalog.VideoSource{VideoWidth: &width, VideoHeight: &height})]
	val := valuesOf(song, chart, s)
	textHTML := filepath.Join(root, "templates/text.html")
	textSource, err := os.ReadFile(textHTML)
	if err != nil {
		return "", d, false, fmt.Errorf("读取文字模板失败: %w", err)
	}

	in := SegInput{
		Clip:          clip,
		Bg:            bg,
		Jacket:        jacket,
		Template:      filepath.Join(root, tpl.Source),
		TextHash:      fmt.Sprintf("%x", sha256.Sum256(textSource)),
		Win:           tpl.Win,
		Value:         val,
		RenderVersion: "seg-av-fade-v2",
		AudioFade:     fade,
	}
	if stat, err := os.Stat(clip); err == nil {
		in.ClipSize, in.ClipModified = stat.Size(), stat.ModTime().UnixNano()
	}
	if s.Clip != nil {
		in.StartMs, in.DurationMs = startMs, durationMs
	}
	if !j.cfg.Force && reusable(ctx, j.deps.Tools.FFprobe, dir, seg, in) {
		return seg, d, true, nil
	}

	bgPng := filepath.Join(dir, "bg.png")
	t := time.Now()
	if err := render.BuildBackground(ctx, j.deps.Tools.FFmpeg, filepath.Join(dataRoot, filepath.FromSlash(bg)), bgPng); err != nil {
		return "", d, false, err
	}
	d.Background = time.Since(t)

	textPng := filepath.Join(dir, "text.png")
	t = time.Now()
	if err := render.RenderText(ctx, val, j.deps.Tools.Edge,
		textHTML,
		filepath.Join(j.cacheDir(), "edge-profile"),
		textPng); err != nil {
		return "", d, false, err
	}
	d.Text = time.Since(t)

	tmp := filepath.Join(dir, fmt.Sprintf("seg_%03d.part.mp4", i))
	t = time.Now()
	if err := render.Composite(ctx, render.Layers{
		Clip: clip, BG: bgPng,
		Jacket:   filepath.Join(dataRoot, filepath.FromSlash(jacket)),
		Template: filepath.Join(root, tpl.Source),
		Text:     textPng, Win: tpl.Win, StartMs: in.StartMs, DurationMs: in.DurationMs, AudioFade: fade,
	}, tmp, j.deps.Tools.FFmpeg); err != nil {
		os.Remove(tmp)
		return "", d, false, err
	}
	d.Composite = time.Since(t)
	if info, err := render.Probe(ctx, j.deps.Tools.FFprobe, tmp); err != nil || info.PixFmt != "yuv420p" || info.DurationMs < durationMs-100 || info.DurationMs > durationMs+100 {
		os.Remove(tmp)
		return "", d, false, fmt.Errorf("片段产物参数或时长无效：%v", err)
	}

	if err := os.Rename(tmp, seg); err != nil {
		return "", d, false, err
	}
	if err := os.WriteFile(filepath.Join(dir, "input.json"), []byte(in.hash()), 0o644); err != nil {
		return "", d, false, err
	}
	return seg, d, false, nil
}
