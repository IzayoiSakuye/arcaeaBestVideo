package ocr

import "image"

// 表头三个数 + 左下角时间戳
var (
	headerTop10 = image.Rect(94, 298, 207, 317)
	headerMax   = image.Rect(294, 298, 407, 317)
	headerTop50 = image.Rect(95, 378, 207, 397)
)

// Header 是表头的ptt数据，用于校验总ptt
type Header struct {
	Top10Avg     float64
	MaxPotential float64
	Top50Avg     float64
	Problems     []string
}
