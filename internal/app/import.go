package app

import (
	"fmt"

	"rhythm-game-bvideo/internal/importer"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PickFile 弹出手动选择文件的窗口
func (a *App) PickFile() (string, error) {
	return runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{})
}

// Import 导入数据源，分为图片或JSON
func (a *App) Import(kind, path, body string) (*score.Draft, error) {
	in := score.Input{Kind: kind, Path: path, Text: body}
	var provider score.Provider
	switch kind {
	case score.KindJSON:
		provider = &importer.ManualJSONProvider{}
	case score.KindImage:
		cata, err := a.catalog()
		if err != nil {
			return nil, err
		}
		overrides, err := loadToolPaths()
		if err != nil {
			return nil, err
		}
		provider = &importer.ImageProvider{Cata: cata, FFmpeg: tools.Detect(overrides).Tools.FFmpeg, OCRRoot: a.root}
	default:
		return nil, fmt.Errorf("不支持的导入类型：%s", kind)
	}
	return provider.Import(a.context(), in)
}
