package score

import "context"

// 成绩的来路
const (
	KindJSON  = "json"
	KindImage = "image"
)

// Input 是一次导入的信息
type Input struct {
	Kind string // KindJSON / KindImage
	Path string // 文件路径
	Text string // 粘贴的内容
}

// Draft 是一次导入的结果
type Draft struct {
	Project *Project
	Notes   []string
}

// Provider 是成绩的来源
type Provider interface {
	Name() string
	Import(ctx context.Context, in Input) (*Draft, error)
}
