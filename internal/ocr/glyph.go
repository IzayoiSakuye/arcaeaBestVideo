package ocr

import (
	"image"
	"sort"
	"strings"
)

// 分段阈值：词间距 ≥5px，字间距 ≥2px
const (
	WordGap = 5
	CharGap = 2
)

// isInk 亮度低于 150 算墨
func isInk(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return (r*299+g*587+b*114)/1000/257 < 150
}

// InkColumns 查看一块区域里哪些列有墨
func InkColumns(img image.Image, r image.Rectangle) []bool {
	out := make([]bool, r.Dx())
	for x := r.Min.X; x < r.Max.X; x++ {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			if isInk(img, x, y) {
				out[x-r.Min.X] = true
				break
			}
		}
	}
	return out
}

// InkRows 找一块区域里有墨的行范围，返回 [y0, y1)
func InkRows(img image.Image, r image.Rectangle) (int, int) {
	top, bottom := -1, -1
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if isInk(img, x, y) {
				if top < 0 {
					top = y
				}
				bottom = y
				break
			}
		}
	}
	if top < 0 {
		return r.Min.Y, r.Min.Y
	}
	return top, bottom + 1
}

// InkBox 把区域左右收窄到有墨的范围
func InkBox(img image.Image, r image.Rectangle) image.Rectangle {
	cols := InkColumns(img, r)
	lo, hi := -1, -1
	for i, c := range cols {
		if c {
			if lo < 0 {
				lo = i
			}
			hi = i
		}
	}
	if lo < 0 {
		return image.Rectangle{}
	}
	return image.Rect(r.Min.X+lo, r.Min.Y, r.Min.X+hi+1, r.Max.Y)
}

// Groups 把有墨的列按空隙切成若干段，返回每段的 x 范围（相对图）
// 空隙小于 minGap 的算同一段
func Groups(cols []bool, x0, minGap int) [][2]int {
	var out [][2]int
	start := -1
	gap := 0
	for i, ink := range cols {
		if ink {
			if start < 0 {
				start = i
			}
			gap = 0
			continue
		}
		if start < 0 {
			continue
		}
		gap++
		if gap >= minGap {
			out = append(out, [2]int{x0 + start, x0 + i - gap + 1})
			start = -1
			gap = 0
		}
	}
	if start >= 0 {
		out = append(out, [2]int{x0 + start, x0 + len(cols)})
	}
	return out
}

// WordBox 取一行里第 i 个词的范围，取不到返回空矩形
func WordBox(img image.Image, line image.Rectangle, i int) image.Rectangle {
	ws := Groups(InkColumns(img, line), line.Min.X, WordGap)
	if i < 0 || i >= len(ws) {
		return image.Rectangle{}
	}
	return image.Rect(ws[i][0], line.Min.Y, ws[i][1], line.Max.Y)
}

// Glyph 是一个字的墨位图，行优先
type Glyph struct {
	W, H int
	Bits []bool
}

func (g Glyph) ink(x, y int) bool {
	if x < 0 || y < 0 || x >= g.W || y >= g.H {
		return false
	}
	return g.Bits[y*g.W+x]
}

// CutRow 把 [x0,x1) 这段字按列空隙硬切成字形
func CutRow(img image.Image, line image.Rectangle, x0, x1, minGap int) []Glyph {
	r := image.Rect(x0, line.Min.Y, x1, line.Max.Y)
	y0, y1 := InkRows(img, r)
	if y0 == y1 {
		return nil
	}
	parts := Groups(InkColumns(img, r), x0, minGap)
	out := make([]Glyph, 0, len(parts))
	for _, p := range parts {
		g := Glyph{W: p[1] - p[0], H: y1 - y0, Bits: make([]bool, (p[1]-p[0])*(y1-y0))}
		for y := y0; y < y1; y++ {
			for x := p[0]; x < p[1]; x++ {
				g.Bits[(y-y0)*g.W+(x-p[0])] = isInk(img, x, y)
			}
		}
		out = append(out, g)
	}
	return out
}

// Diff 数两张字形差多少像素，允许 ±1 误差
func Diff(a, b Glyph) int {
	w, h := max(a.W, b.W)+1, max(a.H, b.H)
	best := -1
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			n := 0
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if a.ink(x, y) != b.ink(x+dx, y+dy) {
						n++
					}
				}
			}
			if best < 0 || n < best {
				best = n
			}
		}
	}
	return best
}

// Font  字体模板
type Font struct {
	chars []rune
	arts  map[rune]Glyph
}

// NewFont 从 ASCII 图拼模板
func NewFont(art map[rune]string) *Font {
	f := &Font{arts: make(map[rune]Glyph, len(art))}
	for ch, s := range art {
		rows := strings.Split(s, "\n")
		h := len(rows)
		w := 0
		for _, r := range rows {
			if n := len([]rune(r)); n > w {
				w = n
			}
		}
		g := Glyph{W: w, H: h, Bits: make([]bool, w*h)}
		for y, r := range rows {
			for x, c := range []rune(r) {
				g.Bits[y*w+x] = c == '#'
			}
		}
		f.arts[ch] = g
		f.chars = append(f.chars, ch)
	}
	sort.Slice(f.chars, func(i, j int) bool { return f.chars[i] < f.chars[j] })
	return f
}

// Chars 返回这套模板认识的字
func (f *Font) Chars() []rune { return f.chars }

// Decoded 是一次认字的结果
type Decoded struct {
	Text    string
	Diffs   []int    // 每个字的差异像素数，用来判可信度
	Spans   [][2]int // 每个字占的列范围，和 Text 一一对应
	OK      bool     // 一个未识别都没有才算 true
	Unknown int      // 走未识别通道盖掉的列数
}

// Decode 解析一块区域里的字
func (f *Font) Decode(img image.Image, r image.Rectangle) Decoded {
	r = InkBox(img, r)
	if r.Empty() {
		return Decoded{}
	}
	y0, y1 := InkRows(img, r)
	h := y1 - y0
	if h == 0 {
		return Decoded{}
	}

	// 整段先读成位图，DP 里只比内存
	w := r.Dx()
	bits := make([]bool, w*h)
	colInk := make([]int, w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if isInk(img, r.Min.X+x, y0+y) {
				bits[y*w+x] = true
				colInk[x]++
			}
		}
	}
	ink := InkColumns(img, r)

	const inf = 1 << 30
	best := make([]int, w+1) // 认到第 i 列为止的最小总差异
	prev := make([]int, w+1) // 上一步在哪一列
	tpl := make([]int, w+1)  // 上一步用了哪套模板（-1 跳过空白列，-2 未识别）
	add := make([]int, w+1)  // 上一步加了多少差异
	for i := 1; i <= w; i++ {
		best[i] = inf
	}
	for i := 0; i < w; i++ {
		if best[i] == inf {
			continue
		}
		if !ink[i] {
			if best[i] < best[i+1] {
				best[i+1], prev[i+1], tpl[i+1], add[i+1] = best[i], i, -1, 0
			}
			continue
		}
		if c := best[i] + colInk[i]; c < best[i+1] {
			best[i+1], prev[i+1], tpl[i+1], add[i+1] = c, i, -2, colInk[i]
		}
		for k, ch := range f.chars {
			g := f.arts[ch]
			if g.H != h {
				continue
			}
			// 允许模板伸出右边界（越界部分按空白比）
			next := min(i+g.W, w)
			c := best[i] + windowDiff(g, bits, w, h, i)
			if c < best[next] {
				best[next], prev[next], tpl[next], add[next] = c, i, k, c-best[i]
			}
		}
	}

	// 倒着回放，再翻正；相邻的未识别列合成一个 '?'
	var revText []rune
	var revDiff []int
	var revSpan [][2]int
	for pos := w; pos > 0; pos = prev[pos] {
		switch t := tpl[pos]; {
		case t == -1:
			continue
		case t == -2:
			revText = append(revText, '?')
			revDiff = append(revDiff, add[pos])
			revSpan = append(revSpan, [2]int{pos - 1, pos})
		default:
			revText = append(revText, f.chars[t])
			revDiff = append(revDiff, add[pos])
			revSpan = append(revSpan, [2]int{max(pos-f.arts[f.chars[t]].W, 0), pos})
		}
	}

	var out Decoded
	for i := len(revText) - 1; i >= 0; i-- {
		if revText[i] == '?' && strings.HasSuffix(out.Text, "?") &&
			out.Spans[len(out.Spans)-1][1] == revSpan[i][0] {
			out.Spans[len(out.Spans)-1][1] = revSpan[i][1]
			out.Unknown++
			continue
		}
		out.Text += string(revText[i])
		out.Diffs = append(out.Diffs, revDiff[i])
		out.Spans = append(out.Spans, revSpan[i])
		if revText[i] == '?' {
			out.Unknown++
		}
	}
	out.OK = out.Unknown == 0
	return out
}

// windowDiff 数模板落在第 i 列时差多少像素
func windowDiff(g Glyph, bits []bool, w, h, i int) int {
	best := -1
	for dx := -1; dx <= 1; dx++ {
		n := 0
		for y := 0; y < h; y++ {
			for x := 0; x < g.W; x++ {
				px := i + dx + x
				has := px >= 0 && px < w && bits[y*w+px]
				if g.Bits[y*g.W+x] != has {
					n++
				}
			}
		}
		if best < 0 || n < best {
			best = n
		}
	}
	return best
}
