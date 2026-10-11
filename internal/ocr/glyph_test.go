package ocr

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"testing"
)

// 定数行是固定的五段：Potential / <定数> / > / <单曲ptt> / <通关类型>
func TestPotentialLineGroups(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skip(err)
	}
	for _, c := range Grid() {
		line := LineBox(c, LinePotential)
		words := Groups(InkColumns(img, line), line.Min.X, WordGap)
		if len(words) != 5 {
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
}

// 模板必须把样例图上的定数、单曲ptt、分数全读对。
// 这条同时压住坐标、分段、模板三样：任何一处漂了都会在这里掉准确率。
func TestReadNumbersMatchesTruth(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skipf("没有样例图：%v", err)
	}

	n, bad := 0, 0
	var all []int
	for _, r := range fixtureRows(t) {
		c := Grid()[r.Index-1]
		line := LineBox(c, LinePotential)
		check := func(field string, f *Font, box image.Rectangle, want string) {
			d := f.Decode(img, box)
			n++
			if !d.OK {
				bad++
				t.Errorf("#%d %s 没认下来（区域 %v）", r.Index, field, box)
				return
			}
			all = append(all, d.Diffs...)
			if d.Text != want {
				bad++
				t.Errorf("#%d %s 认成 %q，真值 %q（逐字差异 %v）", r.Index, field, d.Text, want, d.Diffs)
			}
		}
		check("定数", smallFont, WordBox(img, line, 1), constantText(r.Constant))
		check("单曲ptt", smallFont, WordBox(img, line, 3), pttText(r.PTT))
		check("分数", bigFont, LineBox(c, LineScore), scoreText(r.Score))
	}
	if bad > 0 {
		t.Fatalf("%d/%d 处读错", bad, n)
	}
	worst, over5 := 0, 0
	for _, d := range all {
		if d > worst {
			worst = d
		}
		if d > 5 {
			over5++
		}
	}
	// 认对的时候差异基本是 0-3；行末的字被 JPEG 压掉一条淡边会有 11 左右
	// （实测 2/100 个行末数字）。认错时按经验跳到 50 以上，所以门槛放 20 够松也够紧。
	t.Logf("%d 个数字串 %d 个字全读对，最差 %d 像素，%d 个字超过 5 像素", n, len(all), worst, over5)
	if worst > 20 {
		t.Errorf("最差单字差异 %d 像素，模板可能跟图源漂了", worst)
	}
}

// 整格读一遍，四个字段都得跟真值一模一样，而且不许有「没读出来」的记录
func TestReadFieldsMatchesTruth(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skipf("没有样例图：%v", err)
	}
	bad := 0
	for _, r := range fixtureRows(t) {
		got := ReadFields(img, Grid()[r.Index-1])
		if len(got.Problems) > 0 {
			bad++
			t.Errorf("#%d 有字段没读出来：%v", r.Index, got.Problems)
			continue
		}
		if got.Constant != r.Constant || got.PTT != r.PTT || got.Score != r.Score || got.ClearType != r.ClearType {
			bad++
			t.Errorf("#%d 读成 定数%v ptt%v 分%d %s，真值 定数%v ptt%v 分%d %s",
				r.Index, got.Constant, got.PTT, got.Score, got.ClearType,
				r.Constant, r.PTT, r.Score, r.ClearType)
		}
	}
	if bad == 0 {
		t.Logf("%d 格：定数 / 单曲ptt / 分数 / 通关类型全对", Rows*Cols)
	}
}

// 图上数字的排法：定数 1 位小数、单曲ptt 4 位小数、分数补足 8 位后每 3 位插一个撇号
func constantText(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) }
func pttText(v float64) string      { return strconv.FormatFloat(v, 'f', 4, 64) }

func scoreText(score int) string {
	s := fmt.Sprintf("%08d", score)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('\'')
		}
		b.WriteRune(c)
	}
	return b.String()
}
