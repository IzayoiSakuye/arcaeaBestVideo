package resource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/tools"
)

// LoadBgManifest 用于将bg.json相关数据解析为BgManifest结构体
func LoadBgManifest(path string) (*BgManifest, error) {
	if path == "" {
		return nil, fmt.Errorf("no path provided")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s bg manifest: %w", path, err)
	}
	var r BgManifest
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("failed to unmarshal %s bg manifest: %w", path, err)
	}
	if len(r.Songs) == 0 {
		return nil, fmt.Errorf("no songs found in %s", path)
	}
	return &r, nil
}

// BgPath 用于拼接背景的文件路径
func (bm *BgManifest) BgPath(songID, size string) (string, error) {
	if songID == "" || size == "" {
		return "", fmt.Errorf("songID or size is empty")
	}

	bgID := bm.Songs[songID].Bg
	p, ok := bm.Bgs[bgID].Files[size]
	if !ok {
		return "", fmt.Errorf("song %s: Bg(%s) %s not exist", songID, size, bgID)
	}
	return p.Path, nil
}

// TemplateFor 根据传入的视频源选择尺寸模板
func TemplateFor(src *catalog.VideoSource) bool {
	if src == nil {
		return false
	}
	if src.VideoWidth == nil || src.VideoHeight == nil {
		return false
	}
	width := int64(*src.VideoWidth)
	height := int64(*src.VideoHeight)

	g := tools.Gcd(width, height)

	if width/g == 16 && height/g == 9 {
		return true
	}
	return false

}

// ClipPath 找已经下载过的录像缓存
func ClipPath(workDir, songID, diff string) (string, error) {
	if workDir == "" || songID == "" || diff == "" {
		return "", fmt.Errorf("workDir or songID or diff is empty")
	}

	p := filepath.Join(workDir, songID+".mp4")
	if !isFile(p) {
		return "", fmt.Errorf("%s.mp4(%s) missing record clips", songID, diff)
	}
	return p, nil
}

// isFile 判断一个路径是否是文件
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
