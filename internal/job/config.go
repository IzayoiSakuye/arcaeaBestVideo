package job

// Config 是全部的外部输入参数
type Config struct {
	Project  string
	Output   string
	Root     string
	DataDir  string
	WorkDir  string
	ClipsDir string
	Force    bool
	Limit    int
}
