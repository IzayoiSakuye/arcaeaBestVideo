package score

import (
	"encoding/json"
	"fmt"
	"math"
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
	Achievement     float64  `json:"achievement"`
	Rank            string   `json:"rank"`
	ClearType       string   `json:"clear_type"`
	PTT             *float64 `json:"ptt,omitempty"`
	Pure            int      `json:"pure"`
	ShinyPerfect    *int     `json:"shiny_perfect,omitempty"`
	Far             int      `json:"far"`
	Lost            int      `json:"lost"`
	UnreadJudgments []string `json:"unread_judgments,omitempty"`
}
type UserRef struct {
	Comment string `json:"comment"`
}

type VideoRef struct {
	URL       string `json:"url,omitempty"`
	LocalPath string `json:"local_path,omitempty"`
	OwnerName string `json:"owner_name,omitempty"`
}

type ClipRange struct {
	StartMs    int `json:"start_ms"`
	DurationMs int `json:"duration_ms"`
}

type ThoughtImage struct {
	Path  string `json:"path"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Width int    `json:"width"`
}

type ThoughtPage struct {
	Text   string         `json:"text"`
	Images []ThoughtImage `json:"images,omitempty"`
}

type ThoughtAudio struct {
	Kind    string `json:"kind"`
	Path    string `json:"path,omitempty"`
	StartMs int    `json:"start_ms,omitempty"`
	EndMs   int    `json:"end_ms,omitempty"`
}

type Thought struct {
	ID         string        `json:"id,omitempty"`
	Name       string        `json:"name"`
	Pages      []ThoughtPage `json:"pages,omitempty"`
	Texts      []string      `json:"texts,omitempty"`
	DurationMs int           `json:"duration_ms"`
	Audio      ThoughtAudio  `json:"audio"`
	Image      *ThoughtImage `json:"image,omitempty"`
}

// EffectivePages 保留旧草稿中的共享装饰图，避免读入后丢失画面
func (t Thought) EffectivePages() []ThoughtPage {
	if len(t.Pages) > 0 {
		return t.Pages
	}
	pages := make([]ThoughtPage, 0, len(t.Texts))
	for _, line := range t.Texts {
		page := ThoughtPage{Text: line}
		if t.Image != nil {
			page.Images = []ThoughtImage{*t.Image}
		}
		pages = append(pages, page)
	}
	return pages
}

type Song struct {
	Index      int        `json:"index"`
	SongID     string     `json:"song_id"`
	Difficulty DiffRef    `json:"difficulty"`
	Score      ScoreRef   `json:"score"`
	User       UserRef    `json:"user"`
	Video      *VideoRef  `json:"video,omitempty"`
	Clip       *ClipRange `json:"clip,omitempty"`
}
type PlayItem struct {
	Kind      string `json:"kind"`
	SongIndex int    `json:"song_index,omitempty"`
	ThoughtID string `json:"thought_id,omitempty"`
}
type Project struct {
	SchemaVersion int        `json:"schema_version"`
	Game          string     `json:"game"`
	Region        string     `json:"region"`
	Player        Player     `json:"player"`
	SnapshotAt    time.Time  `json:"snapshot_at"`
	Source        Source     `json:"source"`
	Defaults      Defaults   `json:"defaults"`
	Songs         []Song     `json:"songs"`
	Thoughts      []Thought  `json:"thoughts,omitempty"`
	Thought       *Thought   `json:"thought,omitempty"`
	PlayOrder     []PlayItem `json:"play_order,omitempty"`
}

// SchemaVersion 是当前支持的契约版本
const SchemaVersion = supportedSchema

// Score 把达成率换回整数分
func (s ScoreRef) Score() int {
	return int(math.Round(s.Achievement * 100000))
}

// DecodeProject 用于解码，将传入的data转换为proj结构体
func DecodeProject(data []byte, source string) (*Project, error) {
	if source == "" {
		source = "<json>"
	}
	var head struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, decodeError(data, source, err)
	}
	if head.SchemaVersion != supportedSchema {
		return nil, fmt.Errorf("%s: 不支持的数据版本 %d (预期 %d)",
			source, head.SchemaVersion, supportedSchema)
	}
	var proj Project
	if err := json.Unmarshal(data, &proj); err != nil {
		return nil, decodeError(data, source, err)
	}
	return &proj, nil
}

// LoadProject 读取文件+解码
func LoadProject(path string) (*Project, error) {
	if path == "" {
		return nil, fmt.Errorf("score: 需要合法文件路径")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("score: 无法读取 %s: %w", path, err)
	}
	return DecodeProject(content, path)
}
