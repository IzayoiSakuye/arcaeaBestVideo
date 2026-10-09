package job

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

// renderOne 渲染单首歌片段
func (j *Job) renderOne(ctx context.Context, i int, s score.Song) (string, RenderPhaseDuration, bool, error) {
	var d RenderPhaseDuration
	root := j.cfg.Root

	dir := filepath.Join(j.cfg.WorkDir, fmt.Sprintf("%03d-%s", i, s.SongID))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", d, false, fmt.Errorf("catalog: project create directory failed: %w", err)
	}
	seg := filepath.Join(dir, fmt.Sprintf("seg_%03d.mp4", i))

	song, ok := j.deps.Catalog.Song(s.SongID)
	if !ok {
		return "", d, false, fmt.Errorf("song %s not found", s.SongID)
	}
	chart, ok := j.deps.Catalog.Chart(s.SongID, s.Difficulty.Name)
	if !ok || chart.Video == nil || len(chart.Video.Sources) == 0 {
		return "", d, false, fmt.Errorf("no video sources for song %s", s.SongID)
	}
	src := chart.Video.Sources[0]
	jacket, err := j.deps.Catalog.JacketPath(s.SongID, s.Difficulty.Name)
	if err != nil {
		return "", d, false, fmt.Errorf("parse jacket path failed: %w", err)
	}
	bg, err := j.deps.Bg.BgPath(s.SongID, "960")
	if err != nil {
		return "", d, false, fmt.Errorf("parse bg path failed: %w", err)
	}
	clip, err := resource.ClipPath(j.cfg.ClipsDir, s.SongID, s.Difficulty.Name)
	if err != nil {
		return "", d, false, fmt.Errorf("parse clip failed: %w", err)
	}
	tpl := render.Templates[resource.TemplateFor(&src)]
	val := valuesOf(song, chart, s)

	in := SegInput{
		Clip:     clip,
		Bg:       bg,
		Jacket:   jacket,
		Template: filepath.Join(root, tpl.Source),
		Win:      tpl.Win,
		Value:    val,
	}
	if !j.cfg.Force && reusable(dir, seg, in) {
		return seg, d, true, nil
	}

	bgPng := filepath.Join(dir, "bg.png")
	t := time.Now()
	if err := render.BuildBackground(ctx, j.deps.Tools.FFmpeg, filepath.Join(root, filepath.FromSlash(bg)), bgPng); err != nil {
		return "", d, false, err
	}
	d.Background = time.Since(t)

	textPng := filepath.Join(dir, "text.png")
	t = time.Now()
	if err := render.RenderText(ctx, val, j.deps.Tools.Edge,
		filepath.Join(root, "templates/text.html"),
		filepath.Join(root, ".cache/edge-profile"),
		textPng); err != nil {
		return "", d, false, err
	}
	d.Text = time.Since(t)

	tmp := filepath.Join(dir, fmt.Sprintf("seg_%03d.part.mp4", i))
	t = time.Now()
	if err := render.Composite(ctx, render.Layers{
		Clip: clip, BG: bgPng,
		Jacket:   filepath.Join(root, filepath.FromSlash(jacket)),
		Template: filepath.Join(root, tpl.Source),
		Text:     textPng, Win: tpl.Win,
	}, tmp, j.deps.Tools.FFmpeg); err != nil {
		os.Remove(tmp)
		return "", d, false, err
	}
	d.Composite = time.Since(t)

	if err := os.Rename(tmp, seg); err != nil {
		return "", d, false, err
	}
	if err := os.WriteFile(filepath.Join(dir, "input.json"), []byte(in.hash()), 0o644); err != nil {
		return "", d, false, err
	}
	return seg, d, false, nil
}
