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
		return nil, fmt.Errorf("未提供背景清单路径")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取背景清单 %s 失败：%w", path, err)
	}
	var r BgManifest
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("解析背景清单 %s 失败：%w", path, err)
	}
	if len(r.Songs) == 0 {
		return nil, fmt.Errorf("背景清单 %s 中没有歌曲", path)
	}
	return &r, nil
}

// BgPath 用于拼接背景的文件路径
func (bm *BgManifest) BgPath(songID, size string) (string, error) {
	if songID == "" || size == "" {
		return "", fmt.Errorf("歌曲 ID 或背景尺寸为空")
	}

	bgID := bm.Songs[songID].Bg
	p, ok := bm.Bgs[bgID].Files[size]
	if !ok {
		return "", fmt.Errorf("歌曲 %s 缺少 %s 尺寸的背景 %s", songID, size, bgID)
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
		return "", fmt.Errorf("录像缓存目录、歌曲 ID 或难度为空")
	}

	p := filepath.Join(workDir, songID+".mp4")
	if !isFile(p) {
		return "", fmt.Errorf("歌曲 %s（%s）的录像缓存缺失（预期文件：%s.mp4）", songID, diff, songID)
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
