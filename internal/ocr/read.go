package ocr

import (
	"image"
	"strconv"
	"strings"
)

// Fields 存放一首歌在b50里的信息
type Fields struct {
	Constant  float64
	PTT       float64
	Score     int
	ClearType string
	Problems  []string
}

// ReadFields 读一格里印出来的字段
func ReadFields(img image.Image, c Cell) Fields {
	var f Fields
	line := LineBox(c, LinePotential)

	f.Constant = readFloat(img, WordBox(img, line, 1), "定数", &f)
	f.PTT = readFloat(img, WordBox(img, line, 3), "单曲ptt", &f)

	if d := smallFont.Decode(img, WordBox(img, line, 4)); isClearType(d.Text) && d.OK {
		f.ClearType = d.Text
	} else {
		f.Problems = append(f.Problems, "通关类型读成 "+quote(d.Text)+"（可能是还没建模板的字母）")
	}

	d := bigFont.Decode(img, LineBox(c, LineScore))
	if v, ok := parseScore(d.Text); ok && d.OK {
		f.Score = v
	} else {
		f.Problems = append(f.Problems, "分数读成 "+quote(d.Text))
	}
	return f
}

func readFloat(img image.Image, box image.Rectangle, name string, f *Fields) float64 {
	d := smallFont.Decode(img, box)
	v, err := strconv.ParseFloat(d.Text, 64)
	if err != nil || !d.OK {
		f.Problems = append(f.Problems, name+"读成 "+quote(d.Text))
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
