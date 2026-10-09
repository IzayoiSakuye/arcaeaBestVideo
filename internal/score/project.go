package score

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// 下面的接口是Project接口细分
type Player struct {
	Name        string  `json:"name"`
	Rating      float64 `json:"rating"`
	RatingLabel string  `json:"rating_label"`
}
type Source struct {
	Type       string  `json:"type"`
	Provider   string  `json:"provider"`
	Confidence float64 `json:"confidence"`
}
type Defaults struct {
	Clips struct {
		StartMs    int `json:"start_ms"`
		DurationMs int `json:"duration_ms"`
	} `json:"clips"`
}
type DiffRef struct {
	Name     string  `json:"name"`
	Level    string  `json:"level"`
	Constant float64 `json:"constant"`
}
type ScoreRef struct {
	Achievement float64 `json:"achievement"`
	Rank        string  `json:"rank"`
	Pure        int     `json:"pure"`
	Far         int     `json:"far"`
	Lost        int     `json:"lost"`
}
type UserRef struct {
	Comment string `json:"comment"`
}

type Song struct {
	Index      int      `json:"index"`
	SongID     string   `json:"song_id"`
	Difficulty DiffRef  `json:"difficulty"`
	Score      ScoreRef `json:"score"`
	User       UserRef  `json:"user"`
}
type Project struct {
	SchemaVersion int       `json:"schema_version"`
	Game          string    `json:"game"`
	Region        string    `json:"region"`
	Player        Player    `json:"player"`
	SnapshotAt    time.Time `json:"snapshot_at"`
	Source        Source    `json:"source"`
	Defaults      Defaults  `json:"defaults"`
	Songs         []Song    `json:"songs"`
}

// loadProject 用于解析project的JSON内容
func LoadProject(path string) (*Project, error) {
	if path == "" {
		return nil, fmt.Errorf("catalog: path required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("catalog: project load failed: %w", err)
	}
	var proj Project
	if err := json.Unmarshal(content, &proj); err != nil {
		return nil, fmt.Errorf("catalog: project load failed: %w", err)
	}
	return &proj, nil
}
