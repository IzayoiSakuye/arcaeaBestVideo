package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/job"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type RenderStart struct {
	JobID  string `json:"job_id"`
	Output string `json:"output"`
}

// PickOutput 让用户选择另存为位置
func (a *App) PickOutput() (string, error) {
	return runtime.SaveFileDialog(a.context(), runtime.SaveDialogOptions{
		DefaultFilename: "arcaea-b50.mp4",
		Filters:         []runtime.FileFilter{{DisplayName: "MP4 视频 (*.mp4)", Pattern: "*.mp4"}},
	})
}

// OpenOutput 成片后打开视频
func (a *App) OpenOutput(path string) error {
	abs, err := outputFile(path)
	if err != nil {
		return err
	}
	runtime.BrowserOpenURL(a.context(), (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(abs)}).String())
	return nil
}

// RevealOut 成片后定位文件
func (a *App) RevealOut(path string) error {
	abs, err := outputFile(path)
	if err != nil {
		return err
	}
	return exec.Command("explorer.exe", "/select,"+abs).Start()
}

func outputFile(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("缺少视频路径")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if strings.ToLower(filepath.Ext(abs)) != ".mp4" {
		return "", fmt.Errorf("不是 MP4 视频：%s", abs)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("不是视频文件：%s", abs)
	}
	return abs, nil
}

// StartRender 是渲染全流程
func (a *App) StartRender(project *score.Project, output string, replace bool) (RenderStart, error) {
	var start RenderStart
	review, err := a.Review(project)
	if err != nil {
		return start, err
	}
	order, err := review.Project.EffectivePlayOrder()
	if err != nil {
		return start, err
	}
	selected := make(map[int]bool)
	selectedThoughts := make(map[string]bool)
	for _, item := range order {
		if item.Kind == "song" {
			selected[item.SongIndex] = true
		} else if item.Kind == "thought" {
			selectedThoughts[item.ThoughtID] = true
		}
	}
	for _, problem := range review.Problems {
		if problem.Severity == score.SeverityFatal && (problem.SongIndex == 0 || selected[problem.SongIndex]) {
			return start, fmt.Errorf("待播放的成绩存在 fatal 问题，请先校对或移除")
		}
	}
	if output == "" {
		return start, fmt.Errorf("请选择输出路径")
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return start, err
	}
	if strings.ToLower(filepath.Ext(output)) != ".mp4" {
		return start, fmt.Errorf("输出文件必须为 .mp4")
	}
	if within(a.cacheDir(), output) {
		return start, fmt.Errorf("不能把成片写入可清理的缓存目录")
	}
	for _, song := range review.Project.Songs {
		if song.Video != nil && song.Video.LocalPath != "" {
			path, err := filepath.Abs(song.Video.LocalPath)
			if err == nil && strings.EqualFold(path, output) {
				return start, fmt.Errorf("成片不能覆盖本地录像来源")
			}
		}
	}
	for _, dataDir := range []string{filepath.Join(a.root, "data"), filepath.Join(a.dataRoot(), "data")} {
		if rel, err := filepath.Rel(dataDir, output); err == nil && filepath.IsLocal(rel) {
			return start, fmt.Errorf("不能把视频写入只读数据目录：%s", output)
		}
	}
	if _, err := os.Stat(output); err == nil && !replace {
		return start, fmt.Errorf("输出文件已存在，请确认覆盖：%s", output)
	} else if err != nil && !os.IsNotExist(err) {
		return start, err
	}
	overrides, err := loadToolPaths()
	if err != nil {
		return start, err
	}
	tls, err := tools.Resolve(tools.Detect(overrides))
	if err != nil {
		return start, err
	}
	release := a.beginCacheUse()
	defer release()
	for i, song := range review.Project.Songs {
		if !selected[i+1] {
			continue
		}
		if _, _, err := resource.VideoMaterial(a.context(), tls.FFprobe, filepath.Join(a.cacheDir(), "clips"), song); err != nil {
			return start, fmt.Errorf("第 %d 首：%w", i+1, err)
		}
	}
	cata, _ := a.catalog()
	for i := range review.Project.Thoughts {
		thought := &review.Project.Thoughts[i]
		if selectedThoughts[thought.ID] && strings.HasPrefix(thought.Audio.Kind, "op") {
			options, err := a.ThoughtAudioOptions()
			if err != nil {
				return start, err
			}
			found := false
			for _, option := range options {
				if option.Kind == thought.Audio.Kind && option.Available {
					thought.Audio.Path, found = option.Path, true
					break
				}
			}
			if !found {
				return start, fmt.Errorf("所选背景音乐 %s 尚未提供", thought.Audio.Kind)
			}
		}
	}
	bg, err := resource.LoadBgManifest(filepath.Join(a.dataRoot(), "data", "arcaea-backgrounds.json"))
	if err != nil {
		return start, err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return start, err
	}

	a.mu.Lock()
	if a.cancel != nil || a.videoWorker || a.videoCancel != nil || a.thoughtBusy {
		a.mu.Unlock()
		return start, fmt.Errorf("已有生成或素材获取任务正在运行")
	}
	for i, song := range review.Project.Songs {
		if !selected[i+1] {
			continue
		}
		if task := a.videoTasks[videoSourceKey(song.SongID, song.Difficulty.Name)]; task != nil {
			if task.Source != videoIdentity(song) || task.Song.Clip != nil && task.Song.Clip.ConfirmedKey != song.Clip.ConfirmedKey {
				a.mu.Unlock()
				return start, fmt.Errorf("第 %d 首来源或范围已改变，请刷新工程后再生成", i+1)
			}
		}
	}
	ctx, cancel := context.WithCancel(a.context())
	start = RenderStart{JobID: fmt.Sprintf("%d", time.Now().UnixNano()), Output: output}
	a.cancel, a.jobID = cancel, start.JobID
	a.mu.Unlock()

	j := job.NewJob(job.Deps{Catalog: cata, Bg: bg, Tools: tls}, job.Config{
		Project: review.Project, Output: output, Root: a.root, DataDir: filepath.Join(a.dataRoot(), "data"),
		CacheDir: a.cacheDir(), WorkDir: filepath.Join(a.cacheDir(), "pipeline"), ClipsDir: filepath.Join(a.cacheDir(), "clips"),
	}, func(ev job.Event) {
		if ev.Stage == job.StageDone {
			a.finishRender(start.JobID)
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "job:event", ev)
		}
	})
	go func() {
		_, runErr := j.Run(ctx)
		a.finishRender(start.JobID)
		if runErr != nil && a.ctx != nil {
			runtime.EventsEmit(a.ctx, "job:event", job.Event{Stage: job.StageDone,
				Status: job.StatusFailed, Err: runErr.Error()})
		}
		cancel()
	}()
	return start, nil
}

func (a *App) finishRender(jobID string) {
	a.mu.Lock()
	if a.jobID == jobID {
		a.cancel, a.jobID = nil, ""
	}
	a.mu.Unlock()
}

// CancelRender 按任务id发出取消渲染信号
func (a *App) CancelRender(jobID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if jobID == "" || a.jobID != jobID || a.cancel == nil {
		return fmt.Errorf("渲染任务不存在或已结束")
	}
	a.cancel()
	return nil
}
