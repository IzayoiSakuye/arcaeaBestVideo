package ocr

import (
	"fmt"
	"go/format"
	"image"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// 从样例图里抽字形模板，生成 glyphs_gen.go。
//
//	OCR_GEN=1 go test ./internal/ocr -run TestHarvestFonts
func TestHarvestFonts(t *testing.T) {
	if os.Getenv("OCR_GEN") != "1" {
		t.Skip("要重新生成模板才跑：OCR_GEN=1 go test ./internal/ocr -run TestHarvestFonts")
	}
	img, err := Load(sampleImage)
	if err != nil {
		t.Skip(err)
	}

	rows := fixtureRows(t)
	small, big, judgment := map[rune][]Glyph{}, map[rune][]Glyph{}, map[rune][]Glyph{}
	h := fixtureHeaderOf(t)
	for _, r := range rows {
		c := Grid()[r.Index-1]

		line := LineBox(c, LinePotential)
		collect(t, r.Index, '定', constantText(r.Constant), cutWord(img, line, 1), small)

		sl := LineBox(c, LineScore)
		x0, x1 := inkSpan(img, sl)
		collect(t, r.Index, '分', scoreText(r.Score), CutRow(img, sl, x0, x1, CharGap), big)

		// 通关类型的两个字母：真值就在图上（HC / FR / PM），不需要额外锚点
		collect(t, r.Index, '通', r.ClearType, cutWord(img, line, 4), small)
		jl := LineBox(c, LineJudgment)
		for _, item := range []struct{ value, x0, x1 int }{{r.Far, 224, 241}, {r.Lost, 268, 286}} {
			if item.value >= 10 {
				continue
			}
			gs := CutRow(img, jl, c.Box.Min.X+item.x0, c.Box.Min.X+item.x1, CharGap)
			if len(gs) == 1 && gs[0].H == 11 {
				judgment[rune('0'+item.value)] = append(judgment[rune('0'+item.value)], gs[0])
			}
		}
	}

	img2, err := Load("../../dist/b50-sample2.jpg")
	if err != nil {
		t.Fatal(err)
	}
	collect(t, 4, '通', "NC", cutWord(img2, LineBox(Grid()[3], LinePotential), 4), small)

	// ':' 只在判定行里。第一段固定是 "P:"，但两者常常只隔 1 列、切不开，
	if len(small['P']) > 0 {
		p := medoid(small['P'])
		for _, r := range rows {
			word := cutWord(img, LineBox(Grid()[r.Index-1], LineJudgment), 0)
			if len(word) == 2 && Diff(word[0], p) <= 3 {
				small[':'] = append(small[':'], word[1])
				continue
			}
			if len(word) != 1 {
				continue
			}
			left, colon, ok := splitColon(word[0], p)
			if !ok {
				continue
			}
			small[':'] = append(small[':'], colon)
			small['P'] = append(small['P'], left) // 顺带把 'P' 的样本从 1 份补到 50 份
		}
	}

	// 表头那套 19px 的字：三个平均数 + 左下角时间戳。
	head := map[rune][]Glyph{}
	inv := lightOnDark(img)
	for _, item := range []struct {
		box  image.Rectangle
		text string
	}{
		{headerTop10, fmtNum(h.Top10Avg)},
		{headerMax, fmtNum(h.MaxPotential)},
		{headerTop50, fmtNum(h.Top50Avg)},
	} {
		collect(t, 0, '头', item.text, CutRow(inv, item.box, item.box.Min.X, item.box.Max.X, CharGap), head)
	}
	// 时间戳是两个词（日期 + 时分）
	if words := Groups(InkColumns(inv, footerStamp), footerStamp.Min.X, 8); len(words) == len(strings.Fields(h.Stamp)) {
		for i, w := range words {
			collect(t, 0, '时', strings.Fields(h.Stamp)[i], CutRow(inv, footerStamp, w[0], w[1], CharGap), head)
		}
	} else {
		t.Errorf("时间戳切成 %d 词，真值 %q", len(words), h.Stamp)
	}

	if t.Failed() {
		t.Fatal("有字切错了，模板不可信")
	}

	for _, ch := range []rune("014.P:LMHCFR") {
		if len(small[ch]) > 0 {
			t.Logf("小字 %q：%d 个样本，尺寸 %v", ch, len(small[ch]), sizes(small[ch]))
		}
	}
	if len(big['0']) > 0 {
		t.Logf("大字 '0'：%d 个样本，尺寸 %v", len(big['0']), sizes(big['0']))
	}

	var b strings.Builder
	b.WriteString("// 代码生成，勿手改：OCR_GEN=1 go test ./internal/ocr -run TestHarvestFonts\n")
	b.WriteString("package ocr\n\n")
	writeArt(&b, "smallGlyphArt", "定数/单曲ptt 行的小字", small)
	writeArt(&b, "bigGlyphArt", "分数行的大字", big)
	writeArt(&b, "headerGlyphArt", "表头和时间戳的字", head)
	writeArt(&b, "judgmentGlyphArt", "判定行的数字", judgment)

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		t.Fatalf("生成的源码过不了 gofmt：%v", err)
	}
	if err := os.WriteFile("glyphs_gen.go", src, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("小字 %d / 大字 %d / 表头 %d 个模板 → glyphs_gen.go", len(small), len(big), len(head))
}

// fmtNum 数字按图上那样写：小数末尾不留零
func fmtNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// splitColon 把粘在一起的 "P:" 拆开：逐个试切点，要求左半像 'P'、右半像冒号
func splitColon(g, p Glyph) (left, colon Glyph, ok bool) {
	best := 1 << 30
	for k := 5; k+1 < g.W; k++ {
		l, r := sliceCols(g, 0, k), sliceCols(g, k, g.W)
		if !likeColon(r) {
			continue
		}
		if d := Diff(l, p); d <= 3 && d < best {
			best, left, colon, ok = d, l, r, true
		}
	}
	return left, colon, ok
}

// likeColon 冒号的样子：很窄，竖直方向正好两团墨
func likeColon(g Glyph) bool {
	if g.W > 3 {
		return false
	}
	bands, prev := 0, false
	for y := 0; y < g.H; y++ {
		var has bool
		for x := 0; x < g.W; x++ {
			has = has || g.ink(x, y)
		}
		if has && !prev {
			bands++
		}
		prev = has
	}
	return bands == 2
}

// sliceCols 取字形的 [x0,x1) 列
func sliceCols(g Glyph, x0, x1 int) Glyph {
	out := Glyph{W: x1 - x0, H: g.H, Bits: make([]bool, (x1-x0)*g.H)}
	for y := 0; y < g.H; y++ {
		for x := x0; x < x1; x++ {
			out.Bits[y*out.W+(x-x0)] = g.ink(x, y)
		}
	}
	return out
}

// cutWord 按列空隙硬切一行里的第 i 个词
func cutWord(img image.Image, line image.Rectangle, i int) []Glyph {
	b := WordBox(img, line, i)
	if b.Empty() {
		return nil
	}
	return CutRow(img, line, b.Min.X, b.Max.X, CharGap)
}

// inkSpan 取区域里首尾有墨的列范围
func inkSpan(img image.Image, r image.Rectangle) (int, int) {
	b := InkBox(img, r)
	if b.Empty() {
		return r.Min.X, r.Min.X
	}
	return b.Min.X, b.Max.X
}

func sizes(gs []Glyph) []string {
	seen := map[[2]int]int{}
	for _, g := range gs {
		seen[[2]int{g.W, g.H}]++
	}
	out := make([]string, 0, len(seen))
	for k, n := range seen {
		out = append(out, fmt.Sprintf("%dx%d×%d", k[0], k[1], n))
	}
	sort.Strings(out)
	return out
}

// collect 按真值把字形贴标签：个数对不上就记账，不静默丢
func collect(t *testing.T, idx int, field rune, text string, gs []Glyph, into map[rune][]Glyph) {
	t.Helper()
	cs := []rune(text)
	if len(cs) != len(gs) {
		t.Errorf("#%d %c %q：切成 %d 个字，期望 %d", idx, field, text, len(gs), len(cs))
		return
	}
	for i, ch := range cs {
		into[ch] = append(into[ch], gs[i])
	}
}

// medoid 取同族字形里「跟其他样本总差异最小」的那一份当模板，避开抗锯齿的边角样本
func medoid(samples []Glyph) Glyph {
	const cap = 100
	if len(samples) > cap {
		samples = samples[:cap]
	}
	best, bestSum := 0, -1
	for i := range samples {
		sum := 0
		for j := range samples {
			sum += Diff(samples[i], samples[j])
		}
		if bestSum < 0 || sum < bestSum {
			best, bestSum = i, sum
		}
	}
	return samples[best]
}

// template 先挑最宽的那批样本再取中位数：
func template(samples []Glyph) Glyph {
	w := 0
	for _, g := range samples {
		if g.W > w {
			w = g.W
		}
	}
	wide := make([]Glyph, 0, len(samples))
	for _, g := range samples {
		if g.W == w {
			wide = append(wide, g)
		}
	}
	return medoid(wide)
}

func writeArt(b *strings.Builder, name, doc string, m map[rune][]Glyph) {
	fmt.Fprintf(b, "// %s 是%s\nvar %s = map[rune]string{\n", name, doc, name)
	chars := make([]rune, 0, len(m))
	for ch := range m {
		chars = append(chars, ch)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	for _, ch := range chars {
		g := template(m[ch])
		fmt.Fprintf(b, "\t%q: `%s`,\n", ch, artText(g))
	}
	b.WriteString("}\n\n")
}

func artText(g Glyph) string {
	var b strings.Builder
	for y := 0; y < g.H; y++ {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := 0; x < g.W; x++ {
			if g.ink(x, y) {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
	}
	return b.String()
}
