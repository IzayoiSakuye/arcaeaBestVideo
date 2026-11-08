package app

import (
	"fmt"
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
	if a.jobID != "" || a.videoCancel != nil || a.resourceUpdating || a.opUpdating {
		return fmt.Errorf("生成或下载进行中，无法清理缓存")
	}
	cache := filepath.Join(a.root, ".cache")
	names := []string{"pipeline", "thought"}
	if includeVideos {
		names = append(names, "clips")
	}
	for _, name := range names {
		if err := os.RemoveAll(filepath.Join(cache, name)); err != nil {
			return fmt.Errorf("清理 %s 缓存失败：%w", name, err)
		}
	}
	return nil
}
