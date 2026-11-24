package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ClearCache 清除可重建的渲染缓存；录像由用户单独决定是否删除
func (a *App) ClearCache(includeVideos bool) error {
	if a.root == "" {
		return fmt.Errorf("应用目录未初始化")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jobID != "" || a.videoCancel != nil || a.videoWorker || a.cacheUsers > 0 || a.resourceUpdating || a.opUpdating {
		return fmt.Errorf("生成或下载进行中，无法清理缓存")
	}
	cache := a.cacheDir()
	names := []string{"pipeline", "thought"}
	if includeVideos {
		names = append(names, "clips")
	}
	for _, path := range a.previewLeases {
		for _, name := range names {
			if within(filepath.Join(cache, name), path) {
				return fmt.Errorf("缓存正在预览，请先关闭预览")
			}
		}
	}
	sources, err := loadVideoSources()
	if err != nil {
		return err
	}
	for _, source := range sources {
		for _, name := range names {
			if source.LocalPath != "" && within(filepath.Join(cache, name), source.LocalPath) {
				return fmt.Errorf("缓存中存在已选为本地来源的文件，无法清理")
			}
		}
	}
	for _, task := range a.videoTasks {
		for _, name := range names {
			if task.Song.Video.LocalPath != "" && within(filepath.Join(cache, name), task.Song.Video.LocalPath) {
				return fmt.Errorf("缓存中存在工程的本地来源文件，无法清理")
			}
		}
	}
	for _, name := range names {
		if err := os.RemoveAll(filepath.Join(cache, name)); err != nil {
			return fmt.Errorf("清理 %s 缓存失败：%w", name, err)
		}
	}
	if includeVideos {
		for _, task := range a.videoTasks {
			if task.Song.Video.LocalPath == "" {
				task.State.MaterialReady = false
				task.State.Status, task.State.Error = "cancelled", "素材缓存已清理，请重新准备"
			}
		}
	}
	return nil
}

func within(dir, path string) bool {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	return err == nil && filepath.IsLocal(rel)
}

type CacheState struct {
	VideoBytes   int64 `json:"video_bytes"`
	RenderBytes  int64 `json:"render_bytes"`
	TotalBytes   int64 `json:"total_bytes"`
	Rendering    bool  `json:"rendering"`
	Downloading  bool  `json:"downloading"`
	InUse        bool  `json:"in_use"`
	PreviewCount int   `json:"preview_count"`
}

func (a *App) CacheInfo() (CacheState, error) {
	a.mu.Lock()
	state := CacheState{Rendering: a.jobID != "", Downloading: a.videoWorker || a.videoCancel != nil, InUse: a.cacheUsers > 0 || a.resourceUpdating || a.opUpdating, PreviewCount: len(a.previewLeases)}
	a.cacheUsers++
	a.mu.Unlock()
	defer func() { a.mu.Lock(); a.cacheUsers--; a.mu.Unlock() }()
	for _, name := range []string{"clips", "pipeline", "thought"} {
		var size int64
		err := filepath.WalkDir(filepath.Join(a.cacheDir(), name), func(path string, entry fs.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			info, err := entry.Info()
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			size += info.Size()
			return nil
		})
		if err != nil {
			return state, err
		}
		if name == "clips" {
			state.VideoBytes += size
		} else {
			state.RenderBytes += size
		}
		state.TotalBytes += size
	}
	return state, nil
}
