package ocr

import (
	"fmt"
	"go/format"
	"image"
	"os"
	"sort"
	"strings"
	"testing"
)

// 从样例图里抽字形模板，生成 glyphs_gen.go。
// 标签来自真值夹具 —— 抽模板这件事本身也被校验：切出来的字数对不上就不生成。
// 只有定数和分数这两行的字切得干净（定数 4 字、分数 10 字），模板就从这两处自举；
// 单曲ptt 那行相邻数字常挨在一起，改用生成好的模板 + DP 去认。
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
	small, big := map[rune][]Glyph{}, map[rune][]Glyph{}
	for _, r := range rows {
		c := Grid()[r.Index-1]

		line := LineBox(c, LinePotential)
		collect(t, r.Index, '定', constantText(r.Constant), cutWord(img, line, 1), small)

		sl := LineBox(c, LineScore)
		x0, x1 := inkSpan(img, sl)
		collect(t, r.Index, '分', scoreText(r.Score), CutRow(img, sl, x0, x1, CharGap), big)

		// 通关类型的两个字母：真值就在图上（HC / FR / PM），不需要额外锚点
		collect(t, r.Index, '通', r.ClearType, cutWord(img, line, 4), small)
	}

	// ':' 只在判定行里。第一段固定是 "P:"，但两者常常只隔 1 列、切不开，
	// 所以先从合并的字形里试切：左半要像已经抽到的 'P'，右半要像冒号。
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
	b.WriteString("// 模板是从 dist/b50-sample.jpg 上量死的：图源固定成 Yurisaki 一种，\n")
	b.WriteString("// 字号和字形都不变，所以认字只要比字形，不用缩放也不用 AI 模型。\n\n")
	b.WriteString("package ocr\n\n")
	writeArt(&b, "smallGlyphArt", "定数/单曲ptt 行的小字", small)
	writeArt(&b, "bigGlyphArt", "分数行的大字", big)

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		t.Fatalf("生成的源码过不了 gofmt：%v", err)
	}
	if err := os.WriteFile("glyphs_gen.go", src, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("小字 %d 个模板，大字 %d 个模板 → glyphs_gen.go", len(small), len(big))
}

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

func writeArt(b *strings.Builder, name, doc string, m map[rune][]Glyph) {
	fmt.Fprintf(b, "// %s 是%s\nvar %s = map[rune]string{\n", name, doc, name)
	chars := make([]rune, 0, len(m))
	for ch := range m {
		chars = append(chars, ch)
	}
	sort.Slice(chars, func(i, j int) bool { return chars[i] < chars[j] })
	for _, ch := range chars {
		g := medoid(m[ch])
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
