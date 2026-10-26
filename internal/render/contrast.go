package render

const (
	// 有关视频质量的参数
	OutW   = 1920
	OutH   = 1080
	FPS    = 30
	GOP    = 60       // Group oc Picture，关键帧之间的距离
	CRF    = 18       // x264 的恒定质量参数，越小越清晰，文件越大，一般18是视觉无损
	Preset = "medium" // 压缩率权衡挡位，默认medium即可
	// 有关视频内容的参数
	SecPerSong = 10 // 每首歌的展示时间
	BlurSigma  = 25 // 背景的高斯模糊值
)

// RectWin 是模板中矩形窗口的size和定位点，用于放置曲绘等
type RectWin struct {
	X, Y, W, H int
}

// Template 是模板的相关数据
type Template struct {
	Source string
	Win    RectWin
}

// StreamInfo 是 FFprobe 返回的视频流信息
type StreamInfo struct {
	CodecName     string // 视频压缩编码
	Width, Height int    // 画面像素尺寸
	PixFmt        string // 像素格式
	AvgFrameRate  string // 平均帧率
	SAR           string // 单个像素长宽比
	SampleRate    string // 音频采样率
	Channels      int    // 声道数
	DurationMs    int
}

// Layers 是视频最终形态各层的定义
type Layers struct {
	Clip       string
	BG         string
	Jacket     string
	Template   string
	Text       string
	Win        RectWin
	StartMs    int
	DurationMs int
}
