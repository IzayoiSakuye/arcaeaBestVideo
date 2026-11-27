package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type VideoSelection struct {
	Song  score.Song `json:"song"`
	State VideoState `json:"state"`
}

type videoTask struct {
	Source string
	Song   score.Song
	State  VideoState
	Cancel context.CancelFunc
}

type videoRequest struct {
	Key string
	ID  string
}

func (a *App) emitVideo(state VideoState) {
	if a.ctx != nil && state.RequestID != "" {
		runtime.EventsEmit(a.ctx, "video:event", state)
	}
}

func videoIdentity(song score.Song) string {
	copy := song
	copy.Clip = &score.ClipRange{DurationMs: 1000}
	return score.ClipKey(copy)
}

func (a *App) videoMaterialPath(song score.Song) string {
	if song.Video == nil {
		return ""
	}
	if song.Video.LocalPath != "" {
		return song.Video.LocalPath
	}
	dir := filepath.Join(a.cacheDir(), "clips")
	if strings.HasPrefix(song.Video.URL, "https://www.bilibili.com/") && song.Clip != nil {
		return resource.RangeCachePath(dir, song.SongID, song.Difficulty.Name, song.Video.URL, song.Clip.StartMs, song.Clip.DurationMs)
	}
	return resource.VideoCachePath(dir, song.SongID, song.Difficulty.Name, song.Video.URL)
}

// refreshVideoTaskLocked 不让已丢失的磁盘素材继续复用旧就绪状态。
func (a *App) refreshVideoTaskLocked(task *videoTask) {
	if a.refreshLocalVideoTaskLocked(task) {
		return
	}
	state := &task.State
	missing := false
	if state.PreviewReady {
		token := strings.TrimSuffix(strings.TrimPrefix(state.PreviewURL, "/preview/"), filepath.Ext(state.PreviewURL))
		if a.remotePreviews[token] == nil && !fileReady(a.previews[token]) {
			state.PreviewReady = false
			state.PreviewURL, state.PreviewAudioURL = "", ""
			missing = true
		}
	}
	if state.MaterialReady && !fileReady(a.videoMaterialPath(task.Song)) {
		state.MaterialReady = false
		missing = true
	}
	if missing {
		state.DownloadedBytes, state.TotalBytes, state.Progress = 0, 0, -1
		if state.Status == "ready" || state.Status == "preview" {
			state.Status, state.Error = "failed", "录像素材已丢失或不可用，请重新准备或重试"
		}
	}
}

func (a *App) beginCacheUse() func() {
	a.mu.Lock()
	a.cacheUsers++
	a.mu.Unlock()
	return func() {
		a.mu.Lock()
		a.cacheUsers--
		a.mu.Unlock()
	}
}

// PrepareVideo 只为实际采用的来源准备预览；B站不在此下载高质量素材。
func (a *App) PrepareVideo(song score.Song) (VideoState, error) {
	return a.prepareVideo(song, "")
}

func (a *App) prepareVideo(song score.Song, expectedID string) (VideoState, error) {
	release := a.beginCacheUse()
	defer release()
	if err := a.validateVideoSourceKey(song.SongID, song.Difficulty.Name); err != nil {
		return VideoState{}, err
	}
	cata, err := a.catalog()
	if err != nil {
		return VideoState{}, err
	}
	if song.Video == nil {
		sources, err := loadVideoSources()
		if err != nil {
			return VideoState{}, err
		}
		if source, ok := sources[videoSourceKey(song.SongID, song.Difficulty.Name)]; ok {
			song.Video = &source
		}
	}
	manual := song.Video != nil
	state := a.videoInfo(cata, song)
	fullReady := state.Status == "ready"
	if song.Video == nil || song.Video.LocalPath == "" {
		link, err := resource.VideoURL(state.URL)
		if err != nil {
			return state, fmt.Errorf("缺少有效录像来源")
		}
		song.Video = &score.VideoRef{URL: link, OwnerName: state.OwnerName}
	} else {
		video := *song.Video
		video.LocalPath, err = filepath.Abs(video.LocalPath)
		if err != nil {
			return state, err
		}
		song.Video = &video
	}
	if song.Video.LocalPath != "" && !fileReady(song.Video.LocalPath) {
		return a.unavailableLocalVideo(song, expectedID, state)
	}
	key, source := videoSourceKey(song.SongID, song.Difficulty.Name), videoIdentity(song)
	if source == "" {
		return state, fmt.Errorf("录像来源无效")
	}
	confirmed := score.ClipConfirmed(song)
	a.mu.Lock()
	if expectedID != "" && (a.videoTasks[key] == nil || a.videoTasks[key].State.RequestID != expectedID) {
		a.mu.Unlock()
		return VideoState{}, fmt.Errorf("素材任务已改变")
	}
	if previous := a.videoTasks[key]; previous != nil && previous.Source == source {
		a.refreshVideoTaskLocked(previous)
		sameRange := previous.Song.Clip == nil && !confirmed || confirmed && previous.Song.Clip != nil && previous.Song.Clip.ConfirmedKey == song.Clip.ConfirmedKey
		if sameRange && previous.State.Status != "failed" && previous.State.Status != "cancelled" {
			state = previous.State
			a.mu.Unlock()
			return state, nil
		}
	}
	if a.jobID != "" {
		a.mu.Unlock()
		return state, fmt.Errorf("生成中，无法更改素材")
	}
	if a.videoTasks == nil {
		a.videoTasks = make(map[string]*videoTask)
	}
	if old := a.videoTasks[key]; old != nil && old.Cancel != nil {
		old.Cancel()
	}
	// 同来源失效恢复保留有效确认，换源或修改范围仍须重新保存。
	if !confirmed {
		song.Clip = nil
	}
	state.Confirmed, state.MaterialReady = confirmed, false
	state.PreviewReady = state.PreviewURL != ""
	state.RequestID, state.SongID, state.Difficulty = fmt.Sprintf("%d", time.Now().UnixNano()), song.SongID, song.Difficulty.Name
	state.Index, state.Progress = song.Index, -1
	state.Status, state.Error = "preparing", ""
	prepareCtx, prepareCancel := context.WithTimeout(a.context(), 3*time.Minute)
	defer prepareCancel()
	task := &videoTask{Source: source, Song: song, State: state, Cancel: prepareCancel}
	a.videoTasks[key] = task
	a.mu.Unlock()
	a.emitVideo(state)
	overrides, err := loadToolPaths()
	if err != nil {
		return a.failVideo(key, state.RequestID, err), err
	}
	tls := tools.Detect(overrides).Tools
	if song.Video.LocalPath != "" || fullReady {
		path := song.Video.LocalPath
		if path == "" {
			path = resource.VideoCachePath(filepath.Join(a.cacheDir(), "clips"), song.SongID, song.Difficulty.Name, song.Video.URL)
		}
		info, probeErr := render.Probe(prepareCtx, tls.FFprobe, path)
		if probeErr != nil || info.Width <= 0 || info.DurationMs <= 0 || info.SampleRate == "" || confirmed && (song.Clip.StartMs > info.DurationMs || song.Clip.DurationMs > info.DurationMs-song.Clip.StartMs) {
			err = fmt.Errorf("录像无法读取，请重新获取或选择本地文件")
			return a.failVideo(key, state.RequestID, err), err
		}
		state.PreviewURL, state.DurationMs, state.PreviewReady, state.Status = a.previewURL(path), info.DurationMs, true, "preview"
		if confirmed {
			if strings.HasPrefix(song.Video.URL, "https://www.bilibili.com/") {
				state.Status = "queued"
			} else {
				state.MaterialReady, state.Status, state.Progress = true, "ready", 1
			}
		}
		if state.OwnerName == "" {
			state.OwnerName = resource.CachedVideoOwner(path)
		}
	} else if strings.HasPrefix(song.Video.URL, "https://www.bilibili.com/") {
		streams, resolveErr := resource.ResolvePreview(prepareCtx, tls.YtDlp, song.Video.URL)
		if resolveErr != nil && !manual {
			if chart, ok := cata.Chart(song.SongID, song.Difficulty.Name); ok && chart.Video != nil {
				for _, candidate := range chart.Video.Sources {
					if prepareCtx.Err() != nil {
						break
					}
					link, err := resource.VideoURL(candidate.URL)
					if err != nil || link == song.Video.URL || !strings.HasPrefix(link, "https://www.bilibili.com/") {
						continue
					}
					streams, resolveErr = resource.ResolvePreview(prepareCtx, tls.YtDlp, link)
					if resolveErr == nil {
						song.Video = &score.VideoRef{URL: link, OwnerName: candidate.OwnerName}
						state.URL = link
						break
					}
				}
			}
		}
		if resolveErr != nil {
			return a.failVideo(key, state.RequestID, resolveErr), resolveErr
		}
		state.PreviewURL, state.PreviewAudioURL = a.remotePreviewURLs(song.Video.URL, tls.YtDlp, streams)
		state.DurationMs, state.OwnerName, state.PreviewReady, state.Status = streams.DurationMs, streams.Owner, true, "preview"
		if confirmed && score.ClipConfirmed(song) {
			state.Status = "queued"
		} else {
			song.Clip = nil
			state.Confirmed = false
		}
	} else {
		state.Status = "queued"
	}
	if prepareCtx.Err() != nil {
		return a.failVideo(key, state.RequestID, prepareCtx.Err()), prepareCtx.Err()
	}
	a.mu.Lock()
	if song.Video.LocalPath != "" && (!fileReady(song.Video.LocalPath) || videoIdentity(song) != source) {
		a.mu.Unlock()
		err = fmt.Errorf("本地录像在准备期间已改变，请重新准备并确认选段")
		return a.failVideo(key, state.RequestID, err), err
	}
	if current := a.videoTasks[key]; current == task && a.jobID == "" && prepareCtx.Err() == nil {
		task.Cancel = nil
		task.Source, task.Song = videoIdentity(song), song
		task.State = state
		task.Song.Video.OwnerName = state.OwnerName
		if state.Status == "queued" {
			a.queueVideoLocked(key, task)
		}
	} else {
		a.mu.Unlock()
		return VideoState{}, fmt.Errorf("录像来源已改变，请重新获取状态")
	}
	a.mu.Unlock()
	a.emitVideo(state)
	return state, nil
}

func (a *App) failVideo(key, id string, err error) VideoState {
	a.mu.Lock()
	task := a.videoTasks[key]
	var state VideoState
	if task != nil && task.State.RequestID == id {
		a.refreshVideoTaskLocked(task)
		if task.State.Status != "cancelled" {
			task.State.Status = "failed"
		}
		task.State.Error = err.Error()
		task.Cancel = nil
		state = task.State
	}
	a.mu.Unlock()
	a.emitVideo(state)
	return state
}

// ConfirmVideoRange 返回须写回 Project 的来源和范围，保存成功与素材就绪分开。
func (a *App) ConfirmVideoRange(song score.Song, clip score.ClipRange) (VideoSelection, error) {
	state, err := a.PrepareVideo(song)
	if err != nil {
		return VideoSelection{}, err
	}
	if !state.PreviewReady {
		return VideoSelection{}, fmt.Errorf("录像尚不可预览，请等待准备完成")
	}
	if clip.StartMs < 0 || clip.DurationMs < 1000 || clip.StartMs > state.DurationMs || clip.DurationMs > state.DurationMs-clip.StartMs {
		return VideoSelection{}, fmt.Errorf("片段须至少 1 秒且不得超出原录像时长")
	}
	key := videoSourceKey(song.SongID, song.Difficulty.Name)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jobID != "" {
		return VideoSelection{}, fmt.Errorf("生成中，无法保存片段")
	}
	old := a.videoTasks[key]
	if old == nil || old.State.RequestID != state.RequestID {
		return VideoSelection{}, fmt.Errorf("录像来源已改变，请重新选段")
	}
	if old.Song.Video.LocalPath != "" {
		a.refreshVideoTaskLocked(old)
		if !old.State.PreviewReady {
			return VideoSelection{}, fmt.Errorf("本地录像已不可用或已改变，请重新准备后确认选段")
		}
	}
	video := *old.Song.Video
	song.Video = &video
	song.Clip = &score.ClipRange{StartMs: clip.StartMs, DurationMs: clip.DurationMs}
	song.Clip.ConfirmedKey = score.ClipKey(song)
	if song.Clip.ConfirmedKey == "" {
		return VideoSelection{}, fmt.Errorf("录像来源无效，请重新准备后确认选段")
	}
	if old.Song.Clip != nil && old.Song.Clip.ConfirmedKey == song.Clip.ConfirmedKey {
		return VideoSelection{Song: song, State: old.State}, nil
	}
	if old.Cancel != nil {
		old.Cancel()
	}
	state.RequestID = fmt.Sprintf("%d", time.Now().UnixNano())
	state.Confirmed, state.MaterialReady, state.Error, state.Progress = true, false, "", -1
	state.Status = "queued"
	state.DownloadedBytes, state.TotalBytes = 0, 0
	if song.Video.LocalPath != "" || strings.HasPrefix(song.Video.URL, "https://www.youtube.com/") {
		state.Status, state.MaterialReady, state.Progress = "ready", true, 1
	}
	task := &videoTask{Source: old.Source, Song: song, State: state}
	a.videoTasks[key] = task
	if !state.MaterialReady {
		a.queueVideoLocked(key, task)
	}
	return VideoSelection{Song: song, State: state}, nil
}

func (a *App) queueVideoLocked(key string, task *videoTask) {
	a.videoQueue = append(a.videoQueue, videoRequest{Key: key, ID: task.State.RequestID})
	if !a.videoWorker {
		a.videoWorker = true
		go a.runVideoQueue()
	}
}

func (a *App) runVideoQueue() {
	for {
		a.mu.Lock()
		if len(a.videoQueue) == 0 {
			a.videoWorker = false
			a.mu.Unlock()
			return
		}
		req := a.videoQueue[0]
		a.videoQueue = a.videoQueue[1:]
		task := a.videoTasks[req.Key]
		if task == nil || task.State.RequestID != req.ID || task.State.Status != "queued" {
			a.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
		task.Cancel, a.videoCancel = cancel, cancel
		task.State.Status = "downloading"
		state, song := task.State, task.Song
		a.mu.Unlock()
		a.emitVideo(state)
		overrides, err := loadToolPaths()
		var path, owner string
		if err == nil {
			tls := tools.Detect(overrides).Tools
			startMs, durationMs := 0, 0
			if strings.HasPrefix(song.Video.URL, "https://www.bilibili.com/") {
				if !score.ClipConfirmed(song) {
					err = fmt.Errorf("B站区间尚未确认")
				} else {
					startMs, durationMs = song.Clip.StartMs, song.Clip.DurationMs
				}
			}
			if err == nil {
				path, owner, err = resource.DownloadVideoProgress(ctx, tls.YtDlp, tls.FFmpeg, tls.FFprobe, filepath.Join(a.cacheDir(), "clips"), song.SongID, song.Difficulty.Name, song.Video.URL, startMs, durationMs, func(p resource.DownloadProgress) {
					a.mu.Lock()
					current := a.videoTasks[req.Key]
					if current != task {
						a.mu.Unlock()
						return
					}
					task.State.DownloadedBytes, task.State.TotalBytes = p.DownloadedBytes, p.TotalBytes
					task.State.Progress = -1
					if p.TotalBytes > 0 {
						task.State.Progress = float64(p.DownloadedBytes) / float64(p.TotalBytes)
					}
					update := task.State
					a.mu.Unlock()
					a.emitVideo(update)
				})
			}
			if err == nil && durationMs == 0 {
				info, probeErr := render.Probe(ctx, tls.FFprobe, path)
				if probeErr != nil || info.DurationMs <= 0 || info.SampleRate == "" {
					err = fmt.Errorf("录像时长不可用")
				} else {
					state.DurationMs, state.PreviewURL, state.PreviewReady = info.DurationMs, a.previewURL(path), true
				}
			}
		}
		a.mu.Lock()
		a.videoCancel = nil
		if a.videoTasks[req.Key] == task {
			task.Cancel = nil
			if err != nil || ctx.Err() != nil {
				task.State.Status = "failed"
				if err != nil {
					task.State.Error = err.Error()
				}
				if ctx.Err() != nil {
					task.State.Status, task.State.Error = "cancelled", "素材获取已取消或超时"
				}
			} else {
				task.State.DurationMs, task.State.PreviewURL, task.State.PreviewReady = state.DurationMs, state.PreviewURL, state.PreviewReady
				task.State.Status, task.State.Progress = "preview", 1
				task.State.MaterialReady = score.ClipConfirmed(task.Song)
				if task.State.MaterialReady {
					task.State.Status = "ready"
				}
				if owner != "" {
					task.State.OwnerName, task.Song.Video.OwnerName = owner, owner
				}
			}
			state = task.State
		} else {
			state = VideoState{}
		}
		a.mu.Unlock()
		cancel()
		if state.RequestID != "" {
			a.emitVideo(state)
		}
	}
}

func (a *App) CancelVideoPreparation(songID, difficulty, requestID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	task := a.videoTasks[videoSourceKey(songID, difficulty)]
	if task == nil || task.State.RequestID != requestID {
		return fmt.Errorf("素材任务已改变")
	}
	if task.Cancel != nil {
		task.Cancel()
	}
	if !task.State.MaterialReady {
		task.State.Status = "cancelled"
	}
	return nil
}

func (a *App) RetryVideoPreparation(songID, difficulty, requestID string) (VideoState, error) {
	a.mu.Lock()
	task := a.videoTasks[videoSourceKey(songID, difficulty)]
	if task == nil || task.State.RequestID != requestID {
		a.mu.Unlock()
		return VideoState{}, fmt.Errorf("素材任务已改变")
	}
	a.refreshVideoTaskLocked(task)
	state := task.State
	if a.jobID != "" {
		a.mu.Unlock()
		return state, fmt.Errorf("生成中，无法重试素材获取")
	}
	if task.State.Status != "failed" && task.State.Status != "cancelled" {
		a.mu.Unlock()
		return state, fmt.Errorf("素材任务尚未失败或取消")
	}
	if task.Song.Video.LocalPath != "" || strings.HasPrefix(task.Song.Video.URL, "https://www.bilibili.com/") && !score.ClipConfirmed(task.Song) {
		song := task.Song
		a.mu.Unlock()
		return a.prepareVideo(song, requestID)
	}
	copy := *task
	task = &copy
	task.Cancel = nil
	a.videoTasks[videoSourceKey(songID, difficulty)] = task
	task.State.RequestID, task.State.Status, task.State.Error = fmt.Sprintf("%d", time.Now().UnixNano()), "queued", ""
	task.State.DownloadedBytes, task.State.TotalBytes, task.State.Progress = 0, 0, -1
	a.queueVideoLocked(videoSourceKey(songID, difficulty), task)
	state = task.State
	a.mu.Unlock()
	return state, nil
}
