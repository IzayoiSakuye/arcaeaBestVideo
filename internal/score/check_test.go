package score

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
)

// synthB50 造一份 50 行的自洽数据。造数据时用 SinglePTT 是故意的：
// 被测的是"规则会不会误报"，公式本身另有 ptt_test.go 对拍。
func synthB50(t *testing.T, cata *catalog.Catalog) *Project {
	t.Helper()

	type pick struct {
		song  *catalog.Song
		chart *catalog.Chart
	}
	var picks []pick
	for _, s := range cata.Songs {
		for _, ch := range s.Charts {
			if ch.Constant != nil && ch.Notes != nil && ch.Difficulty == "FTR" {
				picks = append(picks, pick{s, ch})
			}
		}
		if len(picks) >= 50 {
			break
		}
	}
	if len(picks) < 50 {
		t.Skipf("目录里凑不出 50 个带定数和物量的 FTR 谱面")
	}

	type row struct {
		song            *catalog.Song
		chart           *catalog.Chart
		ptt             float64
		score           int
		pure, far, lost int
	}
	rows := make([]row, 0, 50)
	for i, p := range picks[:50] {
		score := int(math.Round((98.0 + float64(i)*0.03) * 100000))
		lost, far := i%4, i%7
		ptt, err := SinglePTT(*p.chart.Constant, score, HC)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row{p.song, p.chart, ptt, score, *p.chart.Notes - far - lost, far, lost})
	}
	slices.SortFunc(rows, func(a, b row) int {
		switch {
		case a.ptt > b.ptt:
			return -1
		case a.ptt < b.ptt:
			return 1
		}
		return 0
	})

	proj := &Project{SchemaVersion: 1, Game: "arcaea"}
	for i, r := range rows {
		ptt := math.Round(r.ptt*10000) / 10000
		proj.Songs = append(proj.Songs, Song{
			Index:      i + 1,
			SongID:     r.song.SongID,
			Difficulty: DiffRef{Name: r.chart.Difficulty, Constant: *r.chart.Constant},
			Score: ScoreRef{
				Achievement: float64(r.score) / 100000,
				Rank:        Rank(r.score),
				ClearType:   string(HC),
				PTT:         &ptt,
				Pure:        r.pure, Far: r.far, Lost: r.lost,
			},
		})
	}
	var sum50, sum10 float64
	for _, r := range rows {
		sum50 += r.ptt
	}
	for _, r := range rows[:10] {
		sum10 += r.ptt
	}
	proj.Player.Rating = math.Round((sum50+sum10)/60*1000) / 1000
	return proj
}

func runCheck(t *testing.T, proj *Project, cata *catalog.Catalog) []Problem {
	t.Helper()
	data, err := json.Marshal(proj)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ValidateBytes(data, "<synth>", cata)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Renderable() {
		t.Fatalf("合成数据不该有 fatal：%+v", res.Problems)
	}
	return Check(res)
}

func TestCheckPassesOnConsistentData(t *testing.T) {
	cata := loadCatalog(t)
	if ps := runCheck(t, synthB50(t, cata), cata); len(ps) != 0 {
		t.Errorf("自洽数据不该报任何问题：%+v", ps)
	}
}

// 故意改错一个数字 —— 这就是 G2 门禁要的材料：抓不住的自检规则等于没写。
func TestCheckCatchesEachMutation(t *testing.T) {
	cata := loadCatalog(t)

	mutations := []struct {
		name   string
		rule   string
		mutate func(*Project)
	}{
		{"分数抄错", "ptt-mismatch", func(p *Project) {
			p.Songs[3].Score.Achievement -= 0.01
		}},
		{"评级写错", "rank-mismatch", func(p *Project) {
			p.Songs[5].Score.Rank = "EX"
		}},
		{"判定数少 1", "note-sum", func(p *Project) {
			p.Songs[7].Score.Pure--
		}},
		{"相邻两行对调", "monotonic", func(p *Project) {
			p.Songs[20], p.Songs[21] = p.Songs[21], p.Songs[20]
		}},
		{"单曲 ptt 多一位小数", "ptt-precision", func(p *Project) {
			v := *p.Songs[9].Score.PTT + 0.00001
			p.Songs[9].Score.PTT = &v
		}},
		{"总分被改", "total-ptt-mismatch", func(p *Project) {
			p.Player.Rating += 0.01
		}},
	}

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			proj := synthB50(t, cata)
			m.mutate(proj)
			if _, ok := findProblem(runCheck(t, proj, cata), m.rule); !ok {
				t.Errorf("改错了「%s」，却没报 %s", m.name, m.rule)
			}
		})
	}
}

func TestCheckSkipsAreRecorded(t *testing.T) {
	cata := loadCatalog(t)
	proj := synthB50(t, cata)
	for i := range proj.Songs {
		proj.Songs[i].Score.PTT = nil
	}
	ps := runCheck(t, proj, cata)
	p, ok := findProblem(ps, "ptt-mismatch")
	if !ok || p.Severity != SeverityInfo {
		t.Fatalf("没有单曲 ptt 时应报一条 info，实得 %+v", ps)
	}
}

func TestCheckTotalPTTSkipsWrongSongCount(t *testing.T) {
	cata := loadCatalog(t)
	proj := synthB50(t, cata)
	proj.Songs = proj.Songs[:30]
	ps := runCheck(t, proj, cata)
	p, ok := findProblem(ps, "total-ptt-mismatch")
	if !ok || p.Severity != SeverityInfo {
		t.Fatalf("30 首时 ⑥ 该跳过并留下 info，实得 %+v", ps)
	}
}

// 实测 Sulfur：10'000'972 = 10'000'000 + 972 个 shiny —— 满分能超过 1000 万，
// 所以判据是 >= 1000 万，不是 == 1000 万
func TestCheckPerfectMemory(t *testing.T) {
	cata := loadCatalog(t)

	cases := []struct {
		name        string
		achievement float64
		clear       string
		wantProblem bool
	}{
		{"满分带 shiny + PM", 100.00972, "PM", false},
		{"正好 1000 万 + PM", 100.0, "PM", false},
		{"PM 但分数不足", 99.9, "PM", true},
		{"超过 1000 万但不是 PM", 100.00972, "FR", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := []byte(fmt.Sprintf(`{"schema_version":1,"game":"arcaea","songs":[
				{"index":1,"song_id":"egoeimi","difficulty":{"name":"FTR","constant":10.5},
				 "score":{"achievement":%v,"clear_type":%q}}]}`, tc.achievement, tc.clear))
			res, err := ValidateBytes(data, "<粘贴>", cata)
			if err != nil {
				t.Fatal(err)
			}
			if _, got := findProblem(Check(res), "pm-score"); got != tc.wantProblem {
				t.Errorf("报 pm-score=%v，期望 %v", got, tc.wantProblem)
			}
		})
	}
}
