package ocr

import (
	"image"
	"testing"
)

// 定数行是固定的五段：Potential / <定数> / > / <单曲ptt> / <通关类型>
func TestPotentialLineGroups(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skip(err)
	}
	bad := 0
	for _, c := range Grid() {
		line := LineBox(c, LinePotential)
		words := Groups(InkColumns(img, line), line.Min.X, 5)
		if len(words) != 5 {
			bad++
			t.Errorf("#%d 切成 %d 段，期望 5：%v", c.Index, len(words), words)
			continue
		}
		if c.Index <= 3 || c.Index%17 == 0 {
			widths := make([]int, 0, 5)
			for _, w := range words {
				widths = append(widths, w[1]-w[0])
			}
			t.Logf("#%-2d 五段宽度 %v", c.Index, widths)
		}
	}
	if bad == 0 {
		t.Log("50 格全部切成 5 段")
	}
	_ = image.Rect
}
