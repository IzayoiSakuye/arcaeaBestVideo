package ocr

// Fields 存放一首歌在 b50 图中的识别结果。
type Fields struct {
	Constant        float64
	PTT             float64
	Score           int
	ClearType       string
	Pure            int
	ShinyPerfect    *int
	Far             int
	Lost            int
	UnreadJudgments []string
	Problems        []string
}
