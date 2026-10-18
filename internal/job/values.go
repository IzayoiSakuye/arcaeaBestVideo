package job

import (
	"math"
	"strconv"
	"strings"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/score"
)

// valuesOf 用于需要的字段值
func valuesOf(song *catalog.Song, chart *catalog.Chart, s score.Song, songRating float64) map[string]string {
	pure, far, lost := "—", "—", "—"
	if s.Score.Pure+s.Score.Far+s.Score.Lost != 0 {
		pure, far, lost = strconv.Itoa(s.Score.Pure), strconv.Itoa(s.Score.Far), strconv.Itoa(s.Score.Lost)
	}
	title := song.TitleEn
	if title == "" {
		title = song.TitleJP
	}
	return map[string]string{
		"ptt":       "+" + strconv.FormatFloat(songRating, 'f', 3, 64),
		"score":     formatScore(s.Score.Achievement),
		"pure":      pure,
		"far":       far,
		"lost":      lost,
		"title":     title,
		"author":    song.Artist,
		"diff":      score.DiffName(chart.Difficulty) + " " + formatConstant(chart.Constant),
		"diffClass": strings.ToLower(chart.Difficulty),
		"comment":   s.User.Comment,
	}
}

// formatScore 用于格式化分数
func formatScore(ach float64) string {
	s := strconv.FormatInt(int64(math.Round(ach*100000)), 10)
	var b strings.Builder
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	return b.String()
}

// formatConstant 用于格式化定数
func formatConstant(c *float64) string {
	if c == nil {
		return "?"
	}
	return strconv.FormatFloat(*c, 'f', -1, 64) // 12 → "12"   11.7 → "11.7"
}
