package ocr

import (
	"image"
	"image/color"
	"slices"
	"testing"
)

func TestShortShinyNumber(t *testing.T) {
	for _, value := range []string{"0", "42"} {
		width := 0
		for _, digit := range value {
			width += judgmentFont.arts[digit].W
		}
		img := image.NewGray(image.Rect(0, 0, width, 11))
		for i := range img.Pix {
			img.Pix[i] = 255
		}
		x := 0
		for _, digit := range value {
			glyph := judgmentFont.arts[digit]
			for y := 0; y < glyph.H; y++ {
				for dx := 0; dx < glyph.W; dx++ {
					if glyph.ink(dx, y) {
						img.SetGray(x+dx, y, color.Gray{Y: 0})
					}
				}
			}
			x += glyph.W
		}
		if got, ok := judgmentNumber(img, img.Bounds(), 2000, 1); !ok || got != map[string]int{"0": 0, "42": 42}[value] {
			t.Errorf("大 P 数 %s 读成 %d（成功=%v）", value, got, ok)
		}
	}
}

func TestReadJudgmentsFromSample(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skipf("没有样例图：%v", err)
	}
	rows := fixtureRows(t)
	read := 0
	for _, want := range rows {
		got := ReadFields(img, Grid()[want.Index-1])
		if got.ShinyPerfect == nil || *got.ShinyPerfect != want.Shiny {
			t.Errorf("#%d 大 P 数 %v，真值 %d", want.Index, got.ShinyPerfect, want.Shiny)
		}
		for _, item := range []struct {
			name            string
			value, expected int
		}{{"pure", got.Pure, want.Pure}, {"far", got.Far, want.Far}, {"lost", got.Lost, want.Lost}} {
			if slices.Contains(got.UnreadJudgments, item.name) {
				if item.value != 0 || len(got.Problems) == 0 {
					t.Errorf("#%d %s 未识别却没有留空或提示", want.Index, item.name)
				}
				continue
			}
			read++
			if item.value != item.expected {
				t.Errorf("#%d %s 读成 %d，真值 %d", want.Index, item.name, item.value, item.expected)
			}
		}
	}
	if read != 150 {
		t.Errorf("读取 %d/150 项，预期全部正确", read)
	}
}
