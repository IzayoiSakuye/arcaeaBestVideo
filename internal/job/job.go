package job

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"
)

type Deps struct {
	Catalog *catalog.Catalog
	Bg      *resource.BgManifest
	Tools   tools.Tools
}

type Event struct {
	Stage     string  `json:"stage"`
	SongIndex int     `json:"song_index"`
	SongID    string  `json:"song_id"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"`
	Message   string  `json:"message"`
	Err       string  `json:"error,omitempty"`
}

// Event 的 Stage 与 Status 取值
const (
	StageRender = "render"
	StageConcat = "concat"
	StageDone   = "done"

	StatusRunning = "running"
	StatusDone    = "done"
	StatusSkipped = "skipped"
	StatusFailed  = "failed"
)

type Result struct {
	Output   string
	Segments []string
	Phases   map[string]time.Duration
	Rendered int
	Concat   time.Duration
}

type Job struct {
	deps Deps
	cfg  Config
	emit func(Event)
}

func NewJob(deps Deps, cfg Config, emit func(Event)) *Job {
	if emit == nil {
		emit = func(Event) {}
	}
	return &Job{deps: deps, cfg: cfg, emit: emit}
}

// normalize 把所有路径绝对化
func (j *Job) normalize() error {
	for _, p := range []*string{
		&j.cfg.Root, &j.cfg.DataDir, &j.cfg.WorkDir, &j.cfg.ClipsDir, &j.cfg.ProjectPath, &j.cfg.Output,
	} {
		if *p == "" {
			continue
		}
		abs, err := filepath.Abs(*p)
		if err != nil {
			return fmt.Errorf("failed to parse filepath.Abs: %w", err)
		}
		*p = abs
	}
	return nil
}

func (j *Job) validate() error {
	for _, rel := range []string{"templates/text.html", "templates/bg16-9.png", "templates/bg4-3.png"} {
		p := filepath.Join(j.cfg.Root, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("root %s missing %s", j.cfg.Root, rel)
		}
	}
	return nil
}

// Run 逐曲渲染后无损拼接，取消在逐曲边界生效
func (j *Job) Run(ctx context.Context) (Result, error) {
	res := Result{Phases: map[string]time.Duration{}}
	if err := j.normalize(); err != nil {
		return res, err
	}
	res.Output = j.cfg.Output
	if err := j.validate(); err != nil {
		return res, err
	}
	proj, err := j.project()
	if err != nil {
		return res, fmt.Errorf("project load failed: %w", err)
	}

	order, err := proj.EffectivePlayOrder()
	if err != nil {
		return res, err
	}
	total := len(order)
	for i, item := range order {
		if j.cfg.Limit > 0 && i >= j.cfg.Limit {
			break
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if item.Kind == "thought" {
			var thought score.Thought
			for _, candidate := range proj.Thoughts {
				if candidate.ID == item.ThoughtID {
					thought = candidate
					break
				}
			}
			j.emit(Event{Stage: StageRender, SongIndex: i + 1, Status: StatusRunning, Progress: float64(i) / float64(total), Message: thought.Name})
			audioPath := thought.Audio.Path
			if strings.HasPrefix(thought.Audio.Kind, "op") {
				audioPath = filepath.Join(j.cfg.Root, "templates", "audio", thought.Audio.Kind+".wav")
			}
			if thought.Audio.Kind == "silent" {
				audioPath = ""
			}
			seg, err := render.Thought(ctx, thought, audioPath, filepath.Join(j.cfg.Root, "templates"), filepath.Join(j.cfg.Root, ".cache", "thought"), j.deps.Tools.FFmpeg, j.deps.Tools.FFprobe, j.deps.Tools.Edge)
			if err != nil {
				return res, fmt.Errorf("第 %d 个感想生成失败：%w", i+1, err)
			}
			res.Segments = append(res.Segments, seg)
			j.emit(Event{Stage: StageRender, SongIndex: i + 1, Status: StatusDone, Message: thought.Name})
			continue
		}
		s := proj.Songs[item.SongIndex-1]
		j.emit(Event{Stage: StageRender, SongIndex: item.SongIndex, SongID: s.SongID,
			Status: StatusRunning, Progress: float64(i) / float64(total)})

		seg, d, skipped, err := j.renderOne(ctx, item.SongIndex, s)
		if err != nil {
			j.emit(Event{Stage: StageRender, SongIndex: item.SongIndex, SongID: s.SongID,
				Status: StatusFailed, Err: err.Error()})
			return res, fmt.Errorf("render song %d failed: %w", item.SongIndex, err)
		}
		status := StatusDone
		if skipped {
			status = StatusSkipped
		} else {
			res.Rendered++
			d.Merge(res.Phases)
		}
		j.emit(Event{Stage: StageRender, SongIndex: item.SongIndex, SongID: s.SongID, Status: status})
		res.Segments = append(res.Segments, seg)
	}
	if len(res.Segments) > 1 {
		for i, seg := range res.Segments {
			faded, err := render.FadeAudio(ctx, j.deps.Tools.FFmpeg, j.deps.Tools.FFprobe, seg, filepath.Join(j.cfg.WorkDir, fmt.Sprintf("fade_%03d.mp4", i)), i > 0, i < len(res.Segments)-1)
			if err != nil {
				return res, fmt.Errorf("第 %d 段音频转场失败：%w", i+1, err)
			}
			res.Segments[i] = faded
		}
	}

	j.emit(Event{Stage: StageConcat, Status: StatusRunning})
	t := time.Now()
	listFile := filepath.Join(j.cfg.WorkDir, "concat.txt")
	if err := render.Concat(ctx, j.deps.Tools.FFmpeg, j.deps.Tools.FFprobe, listFile, j.cfg.Output, res.Segments); err != nil {
		return res, err
	}
	res.Concat = time.Since(t)
	j.emit(Event{Stage: StageDone, Status: StatusDone})
	return res, nil
}

// project 负责读取project分流
func (j *Job) project() (*score.Project, error) {
	proj := j.cfg.Project
	if proj == nil {
		var err error
		if proj, err = score.LoadProject(j.cfg.ProjectPath); err != nil {
			return nil, err
		}
	}
	order, err := proj.EffectivePlayOrder()
	if err != nil {
		return nil, err
	}
	for _, item := range order {
		if item.Kind != "song" {
			continue
		}
		i := item.SongIndex - 1
		s := proj.Songs[i]
		if s.SongID == "" || s.Difficulty.Name == "" {
			return nil, fmt.Errorf("第 %d 首缺 song_id 或难度名称", i+1)
		}
	}
	return proj, nil
}
