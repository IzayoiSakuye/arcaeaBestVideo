package app

import (
	"encoding/json"
	"fmt"
	"strings"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/score"
)

type ReviewRow struct {
	Index        int      `json:"index"`
	SongID       string   `json:"song_id"`
	Title        string   `json:"title"`
	Artist       string   `json:"artist"`
	Difficulty   string   `json:"difficulty"`
	Difficulties []string `json:"difficulties"`
	Constant     *float64 `json:"constant,omitempty"`
	Score        int      `json:"score"`
	Rank         string   `json:"rank"`
	PTT          *float64 `json:"ptt,omitempty"`
	ClearType    string   `json:"clear_type"`
	Pure         int      `json:"pure"`
	Far          int      `json:"far"`
	Lost         int      `json:"lost"`
	Comment      string   `json:"comment"`
}

type ReviewResult struct {
	Project    *score.Project  `json:"project"`
	Rows       []ReviewRow     `json:"rows"`
	Problems   []score.Problem `json:"problems"`
	Renderable bool            `json:"renderable"`
}

type SongChoice struct {
	SongID       string   `json:"song_id"`
	Title        string   `json:"title"`
	Artist       string   `json:"artist"`
	Difficulties []string `json:"difficulties"`
}

// Review 每次从当前草稿重算，返回补全后的 Project
func (a *App) Review(project *score.Project) (*ReviewResult, error) {
	if project == nil {
		return nil, fmt.Errorf("缺少成绩数据")
	}
	cata, err := a.catalog()
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(project)
	if err != nil {
		return nil, err
	}
	resolved, err := score.ValidateBytes(b, "<当前成绩>", cata)
	if err != nil {
		return nil, err
	}
	out := &ReviewResult{Project: resolved.Project, Rows: make([]ReviewRow, 0, len(resolved.Rows)),
		Problems: append(resolved.Problems, score.Check(resolved)...), Renderable: resolved.Renderable()}
	if out.Problems == nil {
		out.Problems = []score.Problem{}
	}
	for i, row := range resolved.Rows {
		s := resolved.Project.Songs[i]
		r := ReviewRow{Index: i + 1, SongID: s.SongID, Difficulty: s.Difficulty.Name,
			Score: s.Score.Score(), Rank: row.Rank, ClearType: s.Score.ClearType,
			Pure: s.Score.Pure, Far: s.Score.Far, Lost: s.Score.Lost, Comment: s.User.Comment}
		if row.Catalog != nil {
			r.Title, r.Artist = row.Catalog.TitleEn, row.Catalog.Artist
			if r.Title == "" {
				r.Title = row.Catalog.TitleJP
			}
			for _, chart := range row.Catalog.Charts {
				r.Difficulties = append(r.Difficulties, chart.Difficulty)
			}
		}
		if row.Chart != nil {
			r.Constant = row.Chart.Constant
		}
		if row.PTTKnown {
			ptt := row.PTT
			r.PTT = &ptt
		}
		out.Rows = append(out.Rows, r)
	}
	return out, nil
}

// SearchSongs 从catalog处找对应的歌曲
func (a *App) SearchSongs(key string, limit int) ([]SongChoice, error) {
	cata, err := a.catalog()
	if err != nil {
		return nil, err
	}
	key = catalog.NormalizeKey(strings.TrimSpace(key))
	if key == "" {
		return []SongChoice{}, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	out := make([]SongChoice, 0, limit)
	for _, song := range cata.Songs {
		match := strings.Contains(catalog.NormalizeKey(song.SongID), key) ||
			strings.Contains(catalog.NormalizeKey(song.TitleEn), key) ||
			strings.Contains(catalog.NormalizeKey(song.TitleJP), key)
		for _, alias := range song.MatchKeys {
			if strings.Contains(alias, key) {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		choice := SongChoice{SongID: song.SongID, Title: song.TitleEn, Artist: song.Artist}
		if choice.Title == "" {
			choice.Title = song.TitleJP
		}
		for _, chart := range song.Charts {
			choice.Difficulties = append(choice.Difficulties, chart.Difficulty)
		}
		out = append(out, choice)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
