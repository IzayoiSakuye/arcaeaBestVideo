package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/ocr"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"
)

const (
	sampleSheet = "../../dist/b50-sample.jpg"
	truthPath   = "../score/testdata/b50-from-image.json"
	// 图上表头自己印的校验和：BEST TOP50 AVG 12.05013 × 50
	headerSum50 = 602.5065
)

type truthRow struct {
	Index     int     `json:"index"`
	Title     string  `json:"title"`
	Score     int     `json:"score"`
	Constant  float64 `json:"constant"`
	PTT       float64 `json:"ptt"`
	ClearType string  `json:"clear_type"`
	Pure      int     `json:"pure"`
	Shiny     int     `json:"shiny_perfect"`
	Far       int     `json:"far"`
	Lost      int     `json:"lost"`
}

func loadTruth(t *testing.T) []truthRow {
	t.Helper()
	b, err := os.ReadFile(truthPath)
	if err != nil {
		t.Skipf("没有真值夹具：%v", err)
	}
	var f struct {
		Songs []truthRow `json:"songs"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Songs) != 50 {
		t.Fatalf("夹具里 %d 行，期望 50", len(f.Songs))
	}
	return f.Songs
}

// 端到端：样例图 → Project → 校验 → 六条自检。
// 曲目靠曲绘指纹（实测 49/50，少的那行是形似撞车），其余字段靠字形模板（50/50）。
func TestImageProviderReadsSampleSheet(t *testing.T) {
	if _, err := os.Stat(sampleSheet); err != nil {
		t.Skipf("没有样例图：%v", err)
	}
	cata, err := catalog.Load(filepath.Join("..", "..", "data", "arcaea-catalog.json"), 1)
	if err != nil {
		t.Skipf("没有曲库：%v", err)
	}
	ff := tools.Detect(tools.Tools{}).Tools.FFmpeg
	if ff == "" {
		t.Skip("没找到 ffmpeg")
	}

	p := &ImageProvider{Cata: cata, FFmpeg: ff}
	draft, err := p.Import(context.Background(), score.Input{Kind: score.KindImage, Path: sampleSheet})
	if err != nil {
		t.Fatal(err)
	}
	songs := draft.Project.Songs
	if len(songs) != 50 {
		t.Fatalf("读出 %d 行，期望 50", len(songs))
	}

	// 逐行对真值。曲目真值用夹具标题查曲库代替（图上曲名不认，这条只是拿来做对照的）
	truth := loadTruth(t)
	songHit, bad := 0, 0
	sum := 0.0
	for i, r := range truth {
		got := songs[i]
		want, _, found := cata.Find(r.Title)
		if !found {
			if cands := cata.Candidates(r.Title, 1); len(cands) > 0 {
				want = cands[0].Song
				found = true
			}
		}
		if found && got.SongID == want.SongID {
			songHit++
		} else {
			t.Logf("#%d 曲目认成 %q", r.Index, got.SongID)
		}
		switch {
		case got.Score.Score() != r.Score:
			bad++
			t.Errorf("#%d 分数 %d，真值 %d", r.Index, got.Score.Score(), r.Score)
		case got.Difficulty.Constant != r.Constant:
			bad++
			t.Errorf("#%d 定数 %v，真值 %v", r.Index, got.Difficulty.Constant, r.Constant)
		case got.Score.ClearType != r.ClearType:
			bad++
			t.Errorf("#%d 通关类型 %q，真值 %q", r.Index, got.Score.ClearType, r.ClearType)
		case got.Score.PTT == nil || *got.Score.PTT != r.PTT:
			bad++
			t.Errorf("#%d 单曲ptt %v，真值 %v", r.Index, got.Score.PTT, r.PTT)
		}
		if got.Score.PTT != nil {
			sum += *got.Score.PTT
		}
		for _, item := range []struct {
			name            string
			value, expected int
		}{{"pure", got.Score.Pure, r.Pure}, {"far", got.Score.Far, r.Far}, {"lost", got.Score.Lost, r.Lost}} {
			if slices.Contains(got.Score.UnreadJudgments, item.name) {
				continue
			}
			if item.value != item.expected {
				t.Errorf("#%d %s 判定数 %d，真值 %d", r.Index, item.name, item.value, item.expected)
			}
		}
		if got.Score.ShinyPerfect == nil || *got.Score.ShinyPerfect != r.Shiny {
			t.Errorf("#%d 大 P 数 %v，真值 %d", r.Index, got.Score.ShinyPerfect, r.Shiny)
		}
		if len(got.Score.UnreadJudgments) > 0 {
			t.Errorf("#%d 样例图判定数应全部读取：%v", r.Index, got.Score.UnreadJudgments)
		}
	}
	t.Logf("曲绘定曲目 %d/50，逐字段错 %d 处", songHit, bad)
	if bad > 0 {
		t.Fatalf("%d 处字段读错", bad)
	}
	if songHit < 45 {
		t.Errorf("曲绘只认对 %d/50，明显低于已知的 49", songHit)
	}

	// 图自带的校验和：50 行 ptt 之和。抄错/认错一位，这个数就对不上
	if math.Abs(sum-headerSum50) > 0.002 {
		t.Errorf("读出 ptt 之和 %.4f，图上表头是 %.4f", sum, headerSum50)
	}
	t.Logf("50 行 ptt 之和 %.4f（图上表头 %.4f）", sum, headerSum50)

	// 校验 + 自检。难度代号图上没有，Validate 按 (曲目, 定数) 反查补上。
	// 问题分两处：定位/补全阶段的问题在 res.Problems，六条自检在 Check(res)
	raw, err := json.Marshal(draft.Project)
	if err != nil {
		t.Fatal(err)
	}
	res, err := score.ValidateBytes(raw, "image", cata)
	if err != nil {
		t.Fatal(err)
	}
	problems := append(append([]score.Problem{}, res.Problems...), score.Check(res)...)

	fatals := 0
	noted := map[int]bool{}
	for _, pr := range problems {
		if pr.Severity == score.SeverityInfo {
			continue
		}
		noted[pr.SongIndex] = true
		if pr.Severity == score.SeverityFatal {
			fatals++
		}
		t.Logf("自检 %s %s：%s", pr.Severity, pr.Rule, pr.Message)
	}
	// #27 的曲绘指纹撞车，其余行的判定和应与曲库一致。
	for idx := range noted {
		if idx != 27 {
			t.Errorf("第 %d 行有自检问题，但这一行本该是好的", idx)
		}
	}
	if fatals != 1 {
		t.Errorf("%d 行不可渲染，只该有 #27 那一行", fatals)
	}

	// 认不准的行必须进 Notes，不能静默
	if !strings.Contains(strings.Join(draft.Notes, "\n"), "#27") {
		t.Errorf("曲绘认不准的行应该进 Notes，现在只有：%v", draft.Notes)
	}
	t.Logf("Notes %d 条：%v", len(draft.Notes), draft.Notes)

	// 好的那些行，难度该被反查出来。
	// 注意查的是 res.Project：Validate 是在自己解码出来的那份上补难度的
	filled := 0
	for i, s := range songs {
		if res.Project.Songs[i].Difficulty.Name != "" {
			filled++
		}
		if s.SongID == "" || s.Score.Achievement <= 0 || s.Score.ClearType == "" {
			t.Errorf("#%d 有字段是空的：%+v", s.Index, s)
		}
	}
	if filled < 45 {
		t.Errorf("只有 %d 行反查出难度", filled)
	}
	t.Logf("反查出难度 %d/50（曲目认错的那行查不到，属预期）", filled)

	closed := map[int]bool{}
	for _, note := range draft.Notes {
		if strings.Contains(note, "曲绘无法准确识别") {
			closed[noteIndex(t, note)] = true
		}
		if strings.Contains(note, "校验和") {
			t.Errorf("表头校验和对不上：%s", note)
		}
	}
	if !closed[27] {
		t.Errorf("#27 的撞车应该进 Notes，现在 Notes 里的认不准行是 %v", closed)
	}
	for idx := range closed {
		if idx != 27 {
			t.Errorf("#%d 被标成认不准，但这一行本该认得准", idx)
		}
	}

	// 表头：玩家 PTT 取图上 MAX POTENTIAL 的 3 位
	if got := draft.Project.Player.Rating; got != 12.104 {
		t.Errorf("玩家 PTT %v，图上 MAX POTENTIAL 12.10442 取 3 位该是 12.104", got)
	}
	t.Logf("玩家 PTT %v（表头印的 MAX POTENTIAL 是 12.10442）", draft.Project.Player.Rating)

	// 顺手把导入结果落一份，用眼睛看看整条流水线吐出来的东西
	dump := filepath.Join("..", "..", ".cache", "imported-project.json")
	if raw, err := json.MarshalIndent(draft.Project, "", "  "); err == nil {
		if err := os.WriteFile(dump, raw, 0o644); err != nil {
			t.Logf("落盘失败（不影响测试）：%v", err)
		} else {
			t.Logf("导入结果：%s（%d 字节）", dump, len(raw))
		}
	}
}

// noteIndex 从 "「#27 ...」" 里取出序号
func noteIndex(t *testing.T, note string) int {
	t.Helper()
	var idx int
	if _, err := fmt.Sscanf(note, "#%d", &idx); err != nil {
		t.Fatalf("看不懂的 Note：%q", note)
	}
	return idx
}

// 校验和得真能抓住错：故意改一行就得报出来
func TestChecksumCatchesWrongPTT(t *testing.T) {
	h := ocr.Header{Top10Avg: 12.37583, Top50Avg: 12.05013}
	ptr := func(v float64) *float64 { return &v }

	songs := make([]score.Song, 50)
	for i := range songs {
		v := 12.37583 // 前 10 行
		if i >= 10 {
			v = (12.05013*50 - 12.37583*10) / 40
		}
		songs[i].Score.PTT = ptr(v)
	}
	if notes := checksums(songs, h); len(notes) != 0 {
		t.Fatalf("造的数据本来就对得上，却报了：%v", notes)
	}

	songs[7].Score.PTT = ptr(12.37583 + 0.05)
	notes := checksums(songs, h)
	if len(notes) == 0 {
		t.Fatal("改了一行的 ptt，校验和应该报出来")
	}
	t.Logf("报出：%s", notes[0])
}

// 尺寸不对必须直接报错：坐标是按 1920×1750 量死的，硬套只会切出一堆看不懂的东西
func TestImageProviderRejectsForeignSheet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	p := &ImageProvider{Cata: &catalog.Catalog{}, FFmpeg: "ffmpeg"}
	_, err = p.Import(context.Background(), score.Input{Kind: score.KindImage, Path: path})
	if err == nil {
		t.Fatal("尺寸不对的图应该报错")
	}
	if !strings.Contains(err.Error(), "16×16") {
		t.Errorf("报错要写清图的尺寸，现在是：%v", err)
	}
}

// 少给依赖时要报得清楚，不要到一半才炸
func TestImageProviderChecksInputs(t *testing.T) {
	p := &ImageProvider{}
	_, err := p.Import(context.Background(), score.Input{Kind: score.KindImage, Path: "x.jpg"})
	if err == nil || !strings.Contains(err.Error(), "曲库") {
		t.Errorf("没曲库时的报错：%v", err)
	}
	p.Cata = &catalog.Catalog{}
	_, err = p.Import(context.Background(), score.Input{Kind: score.KindImage, Path: "x.jpg"})
	if err == nil || !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("没 ffmpeg 时的报错：%v", err)
	}
	_, err = p.Import(context.Background(), score.Input{Kind: score.KindImage})
	if err == nil || !strings.Contains(err.Error(), "路径") {
		t.Errorf("没给路径时的报错：%v", err)
	}
}
