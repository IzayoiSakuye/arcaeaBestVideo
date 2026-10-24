package score

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"rhythm-game-bvideo/internal/catalog"
)

type Severity string

const (
	SeverityFatal Severity = "fatal" // 这一行根本用不了 → 不能渲染
	SeverityError Severity = "error" // 可疑且大概真错 → 能渲染，必须人看
	SeverityWarn  Severity = "warn"
	SeverityInfo  Severity = "info" // 这条没查（缺字段），不是"查了没问题"
)

const supportedSchema int = 1

// Suggestion 用于存放候选。绝不自动套用。
type Suggestion struct {
	SongID   string `json:"song_id"`
	Title    string `json:"title"`
	MatchKey string `json:"match_key"`
	Distance int    `json:"distance"`
}

// Problem 存放发现的问题
type Problem struct {
	SongIndex   int          `json:"song_index"` // 0 = 全局
	Rule        string       `json:"rule"`       // UI 按它查中文文案；Message 给日志/开发者
	Severity    Severity     `json:"severity"`
	Message     string       `json:"message"`
	Field       string       `json:"field,omitempty"`
	Suggestions []Suggestion `json:"suggestions,omitempty"`
}

// Row 存放一首已经查到的歌
type Row struct {
	Input    *Song
	Catalog  *catalog.Song
	Chart    *catalog.Chart
	PTT      float64
	PTTKnown bool
	Rank     string
}

// Resolved 存放解析后的结果
type Resolved struct {
	Project  *Project
	Rows     []Row
	Problems []Problem
}

// Renderable 返回能否渲染，存在 Fatal 问题即无法渲染
func (r *Resolved) Renderable() bool {
	for _, p := range r.Problems {
		if p.Severity == SeverityFatal {
			return false
		}
	}
	return true
}

func ValidateFile(path string, cata *catalog.Catalog) (*Resolved, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取 %s: %w", path, err)
	}
	return ValidateBytes(content, path, cata)
}

// ValidateBytes 负责：解码 + 粗校验 + 逐曲定位/补全/核对/计算。
func ValidateBytes(data []byte, source string, cata *catalog.Catalog) (*Resolved, error) {
	proj, err := DecodeProject(data, source)
	if err != nil {
		return nil, err
	}
	if len(proj.Songs) == 0 {
		return nil, fmt.Errorf("%s: songs 为空，没有可渲染的内容", source)
	}

	res := &Resolved{Project: proj}
	for i := range proj.Songs {
		row, probs := resolveRow(i, &proj.Songs[i], cata)
		res.Rows = append(res.Rows, row)
		res.Problems = append(res.Problems, probs...)
	}
	if n := len(proj.Songs); n != 50 {
		res.Problems = append(res.Problems, Problem{
			SongIndex: 0,
			Rule:      "song-count",
			Severity:  SeverityWarn,
			Message:   fmt.Sprintf("只找到 %d 首歌", n),
		})
	}
	sortProblems(res.Problems)
	return res, nil
}

// resolveRow 用于校验，补全单行歌曲数据
func resolveRow(i int, s *Song, cata *catalog.Catalog) (Row, []Problem) {
	row := Row{Input: s}
	field := func(name string) string { return fmt.Sprintf("songs[%d].%s", i, name) }

	if s.SongID == "" {
		return row, []Problem{{
			SongIndex: i + 1,
			Rule:      "missing-field",
			Severity:  SeverityFatal,
			Field:     field("song_id"),
			Message:   "缺少 song_id",
		}}
	}
	if s.Score.Achievement <= 0 {
		return row, []Problem{{
			SongIndex: i + 1,
			Rule:      "missing-field",
			Severity:  SeverityFatal,
			Field:     field("score.achievement"),
			Message:   "缺少评级",
		}}
	}

	song, _, ok := cata.Find(s.SongID)
	if !ok {
		return row, []Problem{{
			SongIndex:   i + 1,
			Rule:        "song-not-found",
			Severity:    SeverityFatal,
			Field:       field("song_id"),
			Message:     fmt.Sprintf("无法定位歌曲 %q ", s.SongID),
			Suggestions: suggest(cata, s.SongID),
		}}
	}
	row.Catalog = song
	row.Rank = Rank(s.Score.Score())

	inputConstant := s.Difficulty.Constant
	noteSum := s.Score.Pure + s.Score.Far + s.Score.Lost
	if len(s.Score.UnreadJudgments) > 0 {
		noteSum = 0
	}
	chart, ok := findChart(song, s.Difficulty.Name, inputConstant, noteSum)
	if !ok {
		if s.Difficulty.Name == "" {
			return row, []Problem{{
				SongIndex: i + 1,
				Rule:      "difficulty-not-selected",
				Severity:  SeverityFatal,
				Field:     field("difficulty.name"),
				Message:   "未选择难度，请选择谱面",
			}}
		}
		return row, []Problem{{
			SongIndex: i + 1,
			Rule:      "chart-not-found",
			Severity:  SeverityFatal,
			Field:     field("difficulty"),
			Message: fmt.Sprintf("%s: 没有匹配该难度的谱面 difficulty=%q constant=%v",
				song.SongID, s.Difficulty.Name, inputConstant),
		}}
	}
	row.Chart = chart

	// 补全
	if s.Difficulty.Name == "" {
		s.Difficulty.Name = chart.Difficulty
	}
	if inputConstant == 0 && chart.Constant != nil {
		s.Difficulty.Constant = *chart.Constant
	}
	if s.Difficulty.Level == "" && chart.Level != nil {
		s.Difficulty.Level = *chart.Level
	}

	var probs []Problem
	if inputConstant != 0 && chart.Constant != nil && inputConstant != *chart.Constant {
		probs = append(probs, Problem{
			SongIndex: i + 1,
			Rule:      "constant-conflict",
			Severity:  SeverityError,
			Field:     field("difficulty.constant"),
			Message:   fmt.Sprintf("输入定数 %v ，但实际定数 %v", inputConstant, *chart.Constant),
		})
	}

	// 单曲ptt
	if s.Score.ClearType == "" {
		probs = append(probs, Problem{
			SongIndex: i + 1,
			Rule:      "ptt-not-checkable",
			Severity:  SeverityInfo,
			Field:     field("score.clear_type"),
			Message:   "缺少评级，跳过单曲ptt检查",
		})
		return row, probs
	}
	if chart.Constant == nil {
		probs = append(probs, Problem{
			SongIndex: i + 1,
			Rule:      "ptt-not-checkable",
			Severity:  SeverityInfo,
			Field:     field("difficulty.constant"),
			Message:   "缺少定数，跳过单曲ptt检查",
		})
		return row, probs
	}
	ptt, err := SinglePTT(*chart.Constant, s.Score.Score(), ClearType(s.Score.ClearType))
	if err != nil {
		probs = append(probs, Problem{
			SongIndex: i + 1,
			Rule:      "unknown-clear-type",
			Severity:  SeverityError,
			Field:     field("score.clear_type"),
			Message:   err.Error(),
		})
		return row, probs
	}
	row.PTT, row.PTTKnown = ptt, true
	return row, probs
}

// findChart 定位谱面，按照传入的难度或定数定位
func findChart(s *catalog.Song, name string, constant float64, judgments int) (*catalog.Chart, bool) {
	if name != "" {
		for _, c := range s.Charts {
			if strings.EqualFold(c.Difficulty, name) {
				return c, true
			}
		}
		return nil, false
	}
	var hits []*catalog.Chart
	for _, c := range s.Charts {
		if c.Constant != nil && *c.Constant == constant {
			hits = append(hits, c)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], true
	case 0:
		return nil, false
	default:
		if judgments > 0 {
			for _, c := range hits {
				if c.Notes != nil && *c.Notes == judgments {
					return c, true
				}
			}
		}
		return nil, false
	}
}

// suggest 给出可能匹配到的谱面
func suggest(cata *catalog.Catalog, key string) []Suggestion {
	cands := cata.Candidates(key, 3)
	if len(cands) == 0 {
		return nil
	}
	out := make([]Suggestion, 0, len(cands))
	for _, c := range cands {
		out = append(out, Suggestion{
			SongID: c.Song.SongID, Title: c.Song.TitleEn,
			MatchKey: c.Key, Distance: c.Distance,
		})
	}
	return out
}

var severityRank = map[Severity]int{
	SeverityFatal: 0, SeverityError: 1, SeverityWarn: 2, SeverityInfo: 3,
}

// sortProblems 按严重度再按行号排
func sortProblems(ps []Problem) {
	sort.SliceStable(ps, func(i, j int) bool {
		if severityRank[ps[i].Severity] != severityRank[ps[j].Severity] {
			return severityRank[ps[i].Severity] < severityRank[ps[j].Severity]
		}
		return ps[i].SongIndex < ps[j].SongIndex
	})
}

// decodeError 用于表示解码错误
func decodeError(data []byte, source string, err error) error {
	var typ *json.UnmarshalTypeError
	if errors.As(err, &typ) {
		return fmt.Errorf("%s: %s 类型错误 (得到 %s, 预期 %s)%s",
			source, typ.Field, typ.Value, typ.Type, where(data, typ.Offset))
	}
	var syn *json.SyntaxError
	if errors.As(err, &syn) {
		return fmt.Errorf("%s: JSON 语法错误%s: %v", source, where(data, syn.Offset), err)
	}
	return fmt.Errorf("%s: 解码失败: %w", source, err)
}

// where 报出字节偏移值，按行列
func where(data []byte, offset int64) string {
	if offset <= 0 {
		return ""
	}
	line, col := lineCol(data, offset)
	return fmt.Sprintf(" （%d 行 %d 列）%s", line, col, lineText(data, line))
}

// lineText 取某一行的原文
func lineText(data []byte, line int) string {
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}

// lineCol 把字节偏移换成行列
func lineCol(data []byte, offset int64) (int, int) {
	line, col := 1, 1
	for _, r := range string(data[:min(offset, int64(len(data)))]) {
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return line, col
}
