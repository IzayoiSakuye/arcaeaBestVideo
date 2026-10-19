package app

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

// Jacket 返回曲库中当前歌曲的本地缩略图。
func (a *App) Jacket(songID, difficulty string) (string, error) {
	cata, err := a.catalog()
	if err != nil {
		return "", err
	}
	path, err := cata.JacketPath(songID, difficulty)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(a.root, filepath.FromSlash(path))
	base := filepath.Join(a.root, "data", "jackets")
	rel, err := filepath.Rel(base, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("曲绘路径不在本地曲绘目录：%s", path)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("读取曲绘失败：%w", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data), nil
}
