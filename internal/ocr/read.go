package ocr

import (
	"image"
	"strconv"
	"strings"
)

// Fields 存放一首歌在b50里的信息
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

// ReadFieldsAtThreshold 用指定墨色阈值重读整格，供判定数复核使用。
func ReadFieldsAtThreshold(img image.Image, c Cell, limit uint8) Fields {
	return ReadFields(thresholdImage{Image: img, limit: limit}, c)
}

// ReadFields 读一格里印出来的字段
func ReadFields(img image.Image, c Cell) Fields {
	var f Fields
	line := LineBox(c, LinePotential)

	f.Constant = readNum(img, WordBox(img, line, 1), smallFont, "定数", &f.Problems)
	f.PTT = readNum(img, WordBox(img, line, 3), smallFont, "单曲ptt", &f.Problems)

	if d := smallFont.Decode(img, WordBox(img, line, 4)); isClearType(d.Text) && d.OK {
		f.ClearType = d.Text
	} else {
		f.Problems = append(f.Problems, "错误的通关类型 "+quote(d.Text))
	}

	d := bigFont.Decode(img, LineBox(c, LineScore))
	if v, ok := parseScore(d.Text); ok && d.OK {
		f.Score = v
	} else {
		f.Problems = append(f.Problems, "错误的分数格式 "+quote(d.Text))
	}
	j := LineBox(c, LineJudgment)
	for i, item := range []struct {
		name           string
		x0, x1         int
		max, minDigits int
	}{
		{"PURE", 117, 145, 9999, 3}, {"FAR", 224, 241, 999, 1}, {"LOST", 268, 286, 999, 1},
	} {
		box := image.Rect(c.Box.Min.X+item.x0, j.Min.Y+1, c.Box.Min.X+item.x1, j.Min.Y+12)
		if i == 0 {
			for x := c.Box.Min.X + 140; x < c.Box.Min.X+160; x++ {
				if isInk(img, x, j.Min.Y+12) || isInk(img, x, j.Min.Y+13) {
					box.Max.X = x - 2
					break
				}
			}
		}
		v, valid := judgmentNumber(img, box, item.max, item.minDigits)
		if !valid {
			f.Problems = append(f.Problems, item.name+"未能识别")
			f.UnreadJudgments = append(f.UnreadJudgments, strings.ToLower(item.name))
			continue
		}
		switch i {
		case 0:
			f.Pure = v
		case 1:
			f.Far = v
		case 2:
			f.Lost = v
		}
	}
	if v, ok := shinyNumber(img, c, j); ok && (f.Pure == 0 || v <= f.Pure) {
		f.ShinyPerfect = &v
	} else {
		f.Problems = append(f.Problems, "大 P 数未能识别")
	}
	return f
}

func shinyNumber(img image.Image, c Cell, line image.Rectangle) (int, bool) {
	// 括号比数字低两行，用它们定位随 PURE 位数移动的大 P 数。
	openEnd, closeStart := -1, -1
	previous := false
	for x := c.Box.Min.X + 140; x < c.Box.Min.X+205; x++ {
		ink := isInk(img, x, line.Min.Y+12) || isInk(img, x, line.Min.Y+13)
		if ink && !previous {
			closeStart = x
		}
		if !ink && previous && openEnd < 0 {
			openEnd = x
		}
		previous = ink
	}
	if openEnd < 0 || closeStart <= openEnd+11 {
		return 0, false
	}
	box := image.Rect(openEnd+10, line.Min.Y+1, closeStart-1, line.Min.Y+12)
	return judgmentNumber(img, box, 2000, 1)
}

func judgmentNumber(img image.Image, box image.Rectangle, maxValue, minDigits int) (int, bool) {
	d := judgmentFont.Decode(img, box)
	for i, ch := range d.Text {
		if maxValue > 999 && ch == '?' && d.Spans[i][1]-d.Spans[i][0] > 2 {
			return 0, false
		}
	}
	value := strings.ReplaceAll(d.Text, "?", "")
	if maxValue > 999 && (len(value) < minDigits || len(value) > 4) {
		return 0, false
	}
	if maxValue <= 999 && (len(value) < 1 || len(value) > 2 || (strings.Contains(d.Text, "?") && !strings.HasSuffix(d.Text, "?"))) {
		// 小数字的抗锯齿会让窄体 1 消失；按单字字形再比一次。
		parts := CutRow(img, box, box.Min.X, box.Max.X, CharGap)
		if len(parts) == 0 || len(parts) > 2 {
			return 0, false
		}
		value = ""
		for _, part := range parts {
			if part.W <= 3 {
				value += "1"
				continue
			}
			best, delta := rune(0), 1<<30
			for _, ch := range judgmentFont.Chars() {
				if n := Diff(part, judgmentFont.arts[ch]); n < delta {
					best, delta = ch, n
				}
			}
			if delta > 15 {
				return 0, false
			}
			value += string(best)
		}
	}
	v, err := strconv.Atoi(value)
	if err != nil || v > maxValue {
		return 0, false
	}
	if maxValue <= 999 {
		if d.Text == value && len(d.Spans) == len(value) {
			digits := []byte(value)
			r := InkBox(img, box)
			for i, digit := range digits {
				if digit != '5' && digit != '6' {
					continue
				}
				span := d.Spans[i]
				partBox := image.Rect(r.Min.X+span[0], box.Min.Y, r.Min.X+span[1], box.Max.Y)
				parts := CutRow(img, partBox, partBox.Min.X, partBox.Max.X, CharGap)
				if len(parts) != 1 {
					return 0, false
				}
				if five, uncertain := fiveSixCheck(parts[0]); uncertain {
					if !five {
						return 0, false
					}
					digits[i] = '5'
				}
			}
			value = string(digits)
			v, _ = strconv.Atoi(value)
		}
		parts := CutRow(img, box, box.Min.X, box.Max.X, CharGap)
		if len(parts) == 1 && parts[0].W <= 9 {
			first, second := 1<<30, 1<<30
			for _, ch := range judgmentFont.Chars() {
				n := Diff(parts[0], judgmentFont.arts[ch])
				if n < first {
					second, first = first, n
				} else if n < second {
					second = n
				}
			}
			if first >= 10 && second-first <= 2 && (v != 5 || !looksLikeFive(parts[0])) {
				return 0, false
			}
		}
	}
	return v, true
}

func fiveSixCheck(g Glyph) (five, uncertain bool) {
	fiveDiff := Diff(g, judgmentFont.arts['5'])
	sixDiff := Diff(g, judgmentFont.arts['6'])
	return looksLikeFive(g), max(fiveDiff, sixDiff)-min(fiveDiff, sixDiff) <= 2
}

func looksLikeFive(g Glyph) bool {
	if g.W != 7 || g.H != 11 || !(g.ink(1, 1) || g.ink(1, 2)) {
		return false
	}
	// 样图里的 5 顶部会断笔，但第三行下方留白；6 在这里仍有斜笔。
	for x := 0; x < g.W; x++ {
		if g.ink(x, 3) {
			return false
		}
	}
	return true
}

func readNum(img image.Image, box image.Rectangle, f *Font, name string, problems *[]string) float64 {
	d := f.Decode(img, box)
	v, err := strconv.ParseFloat(d.Text, 64)
	if err != nil || !d.OK {
		*problems = append(*problems, name+"读成 "+quote(d.Text))
		return 0
	}
	return v
}

// parseScore 分数是 8 位补零、每 3 位插一个撇号
func parseScore(s string) (int, bool) {
	t := strings.ReplaceAll(s, "'", "")
	if len(t) != 8 {
		return 0, false
	}
	v, err := strconv.Atoi(t)
	return v, err == nil
}

// isClearType 通关类型是两个大写字母
func isClearType(s string) bool {
	if len([]rune(s)) != 2 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func quote(s string) string { return "「" + s + "」" }
