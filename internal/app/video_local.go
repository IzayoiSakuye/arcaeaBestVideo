package app

import (
	"fmt"
	"path/filepath"

	"rhythm-game-bvideo/internal/score"
)

// 路径仅用于找回失败任务，不能替代包含大小和修改时间的来源校验。
func sameLocalVideo(song, previous score.Song) bool {
	if song.Video == nil || previous.Video == nil || song.Video.LocalPath == "" || previous.Video.LocalPath == "" {
		return false
	}
	path, err := filepath.Abs(song.Video.LocalPath)
	oldPath, oldErr := filepath.Abs(previous.Video.LocalPath)
	return err == nil && oldErr == nil && path == oldPath
}

func (a *App) refreshLocalVideoTaskLocked(task *videoTask) bool {
	if task.Song.Video == nil || task.Song.Video.LocalPath == "" {
		return false
	}
	available := fileReady(task.Song.Video.LocalPath)
	if available && videoIdentity(task.Song) == task.Source {
		return false
	}
	state := &task.State
	state.PreviewReady, state.MaterialReady = false, false
	state.PreviewURL, state.PreviewAudioURL = "", ""
	state.DownloadedBytes, state.TotalBytes, state.Progress = 0, 0, -1
	if available {
		state.Confirmed = false
	}
	if state.Status != "cancelled" {
		state.Status = "failed"
	}
	state.Error = "本地录像已丢失、不可用或已改变，请恢复原文件后重试或重新确认选段"
	return true
}

func (a *App) unavailableLocalVideo(song score.Song, expectedID string, state VideoState) (VideoState, error) {
	a.mu.Lock()
	task := a.videoTasks[videoSourceKey(song.SongID, song.Difficulty.Name)]
	if expectedID != "" && (task == nil || task.State.RequestID != expectedID) {
		a.mu.Unlock()
		return VideoState{}, fmt.Errorf("素材任务已改变")
	}
	if task != nil && sameLocalVideo(song, task.Song) {
		a.refreshLocalVideoTaskLocked(task)
		state = task.State
	} else {
		state.Status, state.Error = "failed", "本地录像已丢失或不可用，请恢复文件后重新准备"
	}
	a.mu.Unlock()
	a.emitVideo(state)
	return state, fmt.Errorf("%s", state.Error)
}
