package job

import "time"

// 阶段名
const (
	PhaseBackground = "background"
	PhaseText       = "text"
	PhaseComposite  = "composite"
	PhaseConcat     = "concat"
)

// PhaseOrder 是渲染阶段的顺序，也是打印顺序的唯一来源
var PhaseOrder = []string{PhaseBackground, PhaseText, PhaseComposite}

// RenderPhaseDuration 是渲染阶段单独的耗时计算
type RenderPhaseDuration struct {
	Background, Text, Composite time.Duration
}

// Merge 将时长统计入map
func (d RenderPhaseDuration) Merge(into map[string]time.Duration) {
	into[PhaseBackground] += d.Background
	into[PhaseText] += d.Text
	into[PhaseComposite] += d.Composite
}

// PerSegment 是单段三个渲染阶段的合计，不含 concat
func PerSegment(phases map[string]time.Duration, rendered int) time.Duration {
	if rendered <= 0 {
		return 0
	}
	var sum time.Duration
	for _, name := range PhaseOrder {
		sum += phases[name]
	}
	return sum / time.Duration(rendered)
}

// EstimateMinutes 用于计算整个阶段的时间
func EstimateMinutes(phases map[string]time.Duration, rendered int, concat time.Duration) float64 {
	if rendered <= 0 {
		return 0
	}
	return (PerSegment(phases, rendered).Seconds()*50 + concat.Seconds()) / 60
}
