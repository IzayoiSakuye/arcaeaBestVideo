package job

import "rhythm-game-bvideo/internal/score"

// Config 是全部的外部输入参数
type Config struct {
	Project     *score.Project
	ProjectPath string
	Output      string
	Root        string
	DataDir     string
	WorkDir     string
	ClipsDir    string
	Force       bool
	Limit       int
}
