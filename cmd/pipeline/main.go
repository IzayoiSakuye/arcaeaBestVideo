package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/tools"
	"strconv"
	"strings"
	"time"
)

// flags 参数初始化
var (
	project = flag.String("project", "samples/sample-project.json", "输入样例")
	out     = flag.String("out", "dist/sample/sample.mp4", "成片输出")
	root    = flag.String("root", ".", "项目根目录")
	dataDir = flag.String("data", "data", "数据包根目录")
	workDir = flag.String("work", ".cache/pipeline", "渲染工作区")
	clips   = flag.String("clips", ".cache/clips", "录像片段缓存目录")
	force   = flag.Bool("force", false, "已存在的分段也重渲")
	limit   = flag.Int("limit", 0, "只处理前 N 首（0 = 全部）")
)

// Config 是flags用到的参数
type Config struct {
	Project  string
	Output   string
	Root     string
	DataDir  string
	WorkDir  string
	ClipsDir string
	Force    bool
	Limit    int
}

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

// run 是pipeline的主体运行逻辑
func run(c Config) error {
	// 绝对值化路径
	for _, p := range []*string{&c.Root, &c.DataDir, &c.WorkDir, &c.ClipsDir} {
		abs, err := filepath.Abs(*p)
		if err != nil {
			return fmt.Errorf("failed to parse filepath.Abs: %w", err)
		}
		*p = abs
	}
	root := c.Root
	for _, rel := range []string{"templates/text.html", "templates/bg16-9.png", "templates/bg4-3.png"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("root %s missing %s", root, rel)
		}
	}
	cata, err := catalog.Load(filepath.Join(c.DataDir, "arcaea-catalog.json"), 1)
	if err != nil {
		return fmt.Errorf("catalog load failed: %w", err)
	}
	bg, err := resource.LoadBgManifest(filepath.Join(c.DataDir, "arcaea-backgrounds.json"))
	if err != nil {
		return fmt.Errorf("bg load failed: %w", err)
	}
	proj, err := loadProject(c.Project)
	if err != nil {
		return fmt.Errorf("project load failed: %w", err)
	}

	tl, err := tools.Resolve(tools.Detect(tools.Tools{}))
	if err != nil {
		return fmt.Errorf("tools resolve failed: %w", err)
	}

	var segs []string
	for i, s := range proj.Songs {
		if c.Limit > 0 && i >= c.Limit {
			break
		}
		seg, err := renderOne(c, cata, bg, &tl, i+1, s, root)
		if err != nil {
			return fmt.Errorf("render song %d failed: %w", i+1, err)
		}
		segs = append(segs, seg)
	}
	listFile := filepath.Join(c.WorkDir, "concat.txt")
	return render.Concat(context.Background(), tl.FFmpeg, listFile, c.Output, segs)
}
func main() {
	args, err := parseArgs()
	if err != nil {
		log.Fatal(err)
	}
	if err := run(args); err != nil {
		log.Fatal(err)
	}
}

// parseArgs 用于解析参数
func parseArgs() (Config, error) {
	flag.Parse()
	if flag.NArg() >= 1 {
		return Config{}, fmt.Errorf("unknown argument: %s", flag.Args())
	}
	return Config{
		Project:  *project,
		Output:   *out,
		DataDir:  *dataDir,
		WorkDir:  *workDir,
		ClipsDir: *clips,
		Force:    *force,
		Limit:    *limit,
	}, nil
}

// loadProject 用于解析project的JSON内容
func loadProject(path string) (*Project, error) {
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

// renderOne 用于渲染单首歌片段
func renderOne(c Config, cata *catalog.Catalog, bgm *resource.BgManifest, tl *tools.Tools, i int, s Song, root string) (string, error) {
	//  工作目录
	dir := filepath.Join(c.WorkDir, fmt.Sprintf("%03d-%s", i, s.SongID))
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return "", fmt.Errorf("catalog: project create directory failed: %w", err)
	}
	seg := filepath.Join(dir, fmt.Sprintf("seg_%03d.mp4", i))

	// 幂等
	if !c.Force && isFile(seg) {
		log.Printf("skipping song %s", s.SongID)
		return seg, nil
	}

	// 计算路径
	song, ok := cata.Song(s.SongID)
	if !ok {
		return "", fmt.Errorf("song %s not found", s.SongID)
	}
	chart, ok := cata.Chart(s.SongID, s.Difficulty.Name)
	if chart == nil || chart.Video == nil || len(chart.Video.Sources) == 0 {
		return "", fmt.Errorf("no video sources for song %s", s.SongID)
	}
	src := chart.Video.Sources[0]
	jacket, err := cata.JacketPath(s.SongID, s.Difficulty.Name)
	if err != nil {
		return "", fmt.Errorf("parse jacket path failed: %w", err)
	}
	bg, err := bgm.BgPath(s.SongID, "960")
	if err != nil {
		return "", fmt.Errorf("parse bg path failed: %w", err)
	}
	clip, err := resource.ClipPath(c.ClipsDir, s.SongID, s.Difficulty.Name)
	if err != nil {
		return "", fmt.Errorf("parse clip failed: %w", err)
	}
	tpl := render.Templates[resource.TemplateFor(&src)]

	// 渲染
	bgPng := filepath.Join(dir, "bg.png")
	err = render.BuildBackground(context.Background(), tl.FFmpeg, filepath.Join(root, filepath.FromSlash(bg)), bgPng)
	if err != nil {
		return "", err
	}

	textPng := filepath.Join(dir, "text.png")
	err = render.RenderText(context.Background(), valuesOf(song, chart, s), tl.Edge,
		filepath.Join(root, "templates/text.html"),
		filepath.Join(root, ".cache/edge-profile"),
		textPng)
	if err != nil {
		return "", err
	}

	err = render.Composite(context.Background(), render.Layers{
		Clip: clip, BG: bgPng,
		Jacket:   filepath.Join(c.Root, filepath.FromSlash(jacket)),
		Template: filepath.Join(c.Root, tpl.Source),
		Text:     textPng, Win: tpl.Win,
	}, seg, tl.FFmpeg)
	if err != nil {
		return "", err
	}

	return seg, nil
}

// valuesOf 用于需要的字段值
func valuesOf(song *catalog.Song, chart *catalog.Chart, s Song) map[string]string {
	return map[string]string{
		"ptt":       "+12.34",                         // test
		"score":     formatScore(s.Score.Achievement), // test
		"pure":      strconv.Itoa(s.Score.Pure),
		"far":       strconv.Itoa(s.Score.Far),
		"lost":      strconv.Itoa(s.Score.Lost),
		"title":     song.TitleEn,
		"author":    song.Artist,
		"diff":      diffName(chart.Difficulty) + " " + formatConstant(chart.Constant),
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

// formatConstant
func formatConstant(c *float64) string {
	if c == nil {
		return "?"
	}
	return strconv.FormatFloat(*c, 'f', -1, 64) // 12 → "12"   11.7 → "11.7"
}

// diffName 是难度和文字的映射
func diffName(d string) string {
	switch d {
	case "PST":
		return "Past"
	case "PRS":
		return "Present"
	case "FTR":
		return "Future"
	case "BYD":
		return "Beyond"
	case "ETR":
		return "Eternal"
	}
	return d
}

// isFile 判断传入的是不是文件
func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
