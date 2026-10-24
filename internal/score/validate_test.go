package score

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
)

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.Load(filepath.Join("..", "..", "data", "arcaea-catalog.json"), 1)
	if err != nil {
		t.Skipf("真实目录不可用（还没跑数据管线？）：%v", err)
	}
	return c
}

func validateFixture(t *testing.T, name string, cata *catalog.Catalog) (*Resolved, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("读夹具失败：%v", err)
	}
	return ValidateBytes(data, name, cata)
}

func findProblem(ps []Problem, rule string) (Problem, bool) {
	for _, p := range ps {
		if p.Rule == rule {
			return p, true
		}
	}
	return Problem{}, false
}

func TestPartialJudgmentsDoNotChooseChart(t *testing.T) {
	constant, notesA, notesB := 10.0, 100, 200
	cata := &catalog.Catalog{Songs: []*catalog.Song{{SongID: "sample", Charts: []*catalog.Chart{
		{Difficulty: "FTR", Constant: &constant, Notes: &notesA},
		{Difficulty: "BYD", Constant: &constant, Notes: &notesB},
	}}}}
	song := Song{SongID: "sample", Difficulty: DiffRef{Constant: constant}, Score: ScoreRef{Achievement: 99, Pure: 100, UnreadJudgments: []string{"far"}}}
	row, problems := resolveRow(0, &song, cata)
	if row.Chart != nil {
		t.Fatal("部分判定和不能用于消歧谱面")
	}
	if _, ok := findProblem(problems, "difficulty-not-selected"); !ok {
		t.Errorf("应提示谱面仍有歧义：%v", problems)
	}
}

func TestUnselectedDifficultyBlocksRender(t *testing.T) {
	cata := &catalog.Catalog{Songs: []*catalog.Song{{SongID: "egoeimi", Charts: []*catalog.Chart{{Difficulty: "FTR"}}}}}
	song := Song{SongID: "egoeimi", Score: ScoreRef{Achievement: 99}}
	row, problems := resolveRow(0, &song, cata)
	if row.Chart != nil {
		t.Fatal("未选择难度时不应定位到谱面")
	}
	p, ok := findProblem(problems, "difficulty-not-selected")
	if !ok || p.Message != "未选择难度，请选择谱面" || p.Field != "songs[0].difficulty.name" || p.Severity != SeverityFatal {
		t.Fatalf("未选择难度的提示不正确：%+v", problems)
	}
	if (&Resolved{Problems: problems}).Renderable() {
		t.Fatal("未选择难度时不应允许生成")
	}
}

// 整份文件级失败：这些走 error，不进 Problems
func TestValidateFileLevelFailures(t *testing.T) {
	cata := loadCatalog(t)

	// want 是"这条消息必须说清的事实"，逐项断言 —— 文案可以改，事实不能丢
	cases := []struct {
		fixture string
		want    []string
	}{
		{"bad-syntax.json", []string{"4 行", "4 列"}}, // 行列号必须出现
		{"bad-type.json", []string{"songs.score", "string"}},
		{"bad-schema.json", []string{"99", "1"}}, // 文件版本 + 支持版本
		// b50 抄录形态（title/score 是整数）不是 Project —— 两套形态确实是分开的
		{"b50-from-image.json", []string{"songs.score"}},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			_, err := validateFixture(t, tc.fixture, cata)
			if err == nil {
				t.Fatal("期望整份文件级失败，却成功了")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("错误信息 %q 不含 %q", err.Error(), want)
				}
			}
		})
	}
}

func TestValidateRowProblems(t *testing.T) {
	cata := loadCatalog(t)

	t.Run("缺 achievement 指向第 2 首", func(t *testing.T) {
		res, err := validateFixture(t, "missing-required.json", cata)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := findProblem(res.Problems, "missing-field")
		if !ok {
			t.Fatalf("没报 missing-field：%+v", res.Problems)
		}
		if p.Severity != SeverityFatal {
			t.Errorf("严重度应为 fatal，实得 %s", p.Severity)
		}
		if p.SongIndex != 2 || p.Field != "songs[1].score.achievement" {
			t.Errorf("定位错了：SongIndex=%d Field=%q", p.SongIndex, p.Field)
		}
		if len(res.Rows) != len(res.Project.Songs) {
			t.Errorf("Rows 要和 Songs 一一对应：%d vs %d", len(res.Rows), len(res.Project.Songs))
		}
	})

	t.Run("曲目查不到要给候选，且一次列全", func(t *testing.T) {
		res, err := validateFixture(t, "song-not-found.json", cata)
		if err != nil {
			t.Fatal(err)
		}
		var notFound []Problem
		for _, p := range res.Problems {
			if p.Rule == "song-not-found" {
				notFound = append(notFound, p)
			}
		}
		if len(notFound) != 2 {
			t.Fatalf("3 首里有 2 首查不到，应报 2 条，实得 %d", len(notFound))
		}
		if res.Renderable() {
			t.Error("有 fatal 就不能渲染")
		}
		// "cyagha" 是"把 ë 整个删掉"的产物，候选里必须有 cyaegha
		var hit bool
		for _, s := range notFound[0].Suggestions {
			if s.SongID == "cyaegha" {
				hit = true
			}
		}
		if !hit {
			t.Errorf("候选里应该有 cyaegha，实得 %+v", notFound[0].Suggestions)
		}
		// 中间那首是好的，它照样要有 Row 和派生值
		if res.Rows[1].Catalog == nil || !res.Rows[1].PTTKnown {
			t.Error("查得到的那首不该受影响")
		}
	})

	t.Run("定数冲突只报 error，不阻断", func(t *testing.T) {
		res, err := validateFixture(t, "constant-conflict.json", cata)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := findProblem(res.Problems, "constant-conflict")
		if !ok {
			t.Fatalf("没报 constant-conflict：%+v", res.Problems)
		}
		if p.Severity != SeverityError {
			t.Errorf("定数冲突不该是 fatal（它不进成片）：%s", p.Severity)
		}
		if !res.Renderable() {
			t.Error("冲突不该阻断渲染")
		}
		if !res.Rows[0].PTTKnown {
			t.Error("冲突时也该用曲库定数算 ptt")
		}
		// 关键：用户填的 11.5 不能被静默覆盖
		if got := res.Rows[0].Input.Difficulty.Constant; got != 11.5 {
			t.Errorf("用户填的值被覆盖了：%v", got)
		}
	})

	t.Run("干净输入只该有首数 warn", func(t *testing.T) {
		res, err := validateFixture(t, "good-minimal.json", cata)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range res.Problems {
			if p.Rule != "song-count" {
				t.Errorf("不该有别的问题：%+v", p)
			}
		}
		if !res.Renderable() {
			t.Error("应该能渲染")
		}
		row := res.Rows[0]
		if !row.PTTKnown || FormatPtt(row.PTT) != 125420 {
			t.Errorf("单曲 ptt 应为 12.5420，实得 %v（known=%v）", row.PTT, row.PTTKnown)
		}
		if row.Rank != "EX+" {
			t.Errorf("评级应为 EX+，实得 %q", row.Rank)
		}
	})

	t.Run("缺 clear_type 要报 info 而不是静默跳过", func(t *testing.T) {
		data := []byte(`{"schema_version":1,"game":"arcaea","songs":[
			{"index":1,"song_id":"egoeimi","difficulty":{"name":"FTR","constant":10.5},
			 "score":{"achievement":99.68409,"pure":1,"far":0,"lost":0}}]}`)
		res, err := ValidateBytes(data, "<粘贴>", cata)
		if err != nil {
			t.Fatal(err)
		}
		p, ok := findProblem(res.Problems, "ptt-not-checkable")
		if !ok || p.Severity != SeverityInfo {
			t.Errorf("该报一条 info：%+v", res.Problems)
		}
		if res.Rows[0].PTTKnown {
			t.Error("缺 clear_type 时不该声称算出了 ptt")
		}
	})

	t.Run("难度代号缺失时按定数反查", func(t *testing.T) {
		// 图上没有难度代号，只有定数 —— 这是 b50 抄录路径的常态
		data := []byte(`{"schema_version":1,"game":"arcaea","songs":[
			{"index":1,"song_id":"egoeimi","difficulty":{"constant":10.5},
			 "score":{"achievement":99.68409,"clear_type":"HC"}}]}`)
		res, err := ValidateBytes(data, "<粘贴>", cata)
		if err != nil {
			t.Fatal(err)
		}
		row := res.Rows[0]
		if row.Chart == nil || row.Chart.Difficulty != "FTR" {
			t.Fatalf("按定数 10.5 该反查到 FTR：%+v", row.Chart)
		}
		if row.Input.Difficulty.Name != "FTR" {
			t.Errorf("难度代号该被补全，实得 %q", row.Input.Difficulty.Name)
		}
	})
}
