package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"rhythm-game-bvideo/internal/score"
)

// ManualJSONProvider 读一份录入或别处导出的 Project JSON
type ManualJSONProvider struct{}

func (p *ManualJSONProvider) Name() string { return "手抄 JSON" }

// Import 读取 JSON
func (p *ManualJSONProvider) Import(ctx context.Context, in score.Input) (*score.Draft, error) {
	data, source, err := readJSON(in)
	if err != nil {
		return nil, err
	}
	proj, err := score.DecodeProject(data, source)
	if err != nil {
		return nil, err
	}
	return &score.Draft{Project: proj}, nil
}

// readJSON 读取得到的JSON内容
func readJSON(in score.Input) ([]byte, string, error) {
	if in.Path != "" {
		abs, err := filepath.Abs(in.Path)
		if err != nil {
			abs = in.Path
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return nil, "", fmt.Errorf("读取失败 %s：%w ，请检查文件", abs, err)
		}
		return data, abs, nil
	}
	if in.Text != "" {
		return []byte(in.Text), "<粘贴的内容>", nil
	}
	return nil, "", fmt.Errorf("importer: 无文件路径或JSON文本")
}
