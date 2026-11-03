package job

import (
	"testing"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/score"
)

func TestValuesOfUsesEachSongPTT(t *testing.T) {
	c := 10.5
	chart := &catalog.Chart{Difficulty: "FTR", Constant: &c}
	song := &catalog.Song{TitleEn: "Example"}
	v := valuesOf(song, chart, score.Song{Score: score.ScoreRef{Achievement: 99.5, ClearType: "HC"}})
	if v["ptt"] != "+12.450" || v["pure"] != "—" || v["far"] != "—" || v["lost"] != "—" {
		t.Fatalf("成绩展示失真：%v", v)
	}
	v = valuesOf(song, chart, score.Song{Score: score.ScoreRef{Achievement: 99.0, ClearType: "TL"}})
	if v["ptt"] != "+12.000" {
		t.Fatalf("单曲 PTT 没有随成绩变化：%v", v)
	}
	v = valuesOf(song, chart, score.Song{Score: score.ScoreRef{Achievement: 99.5}})
	if v["ptt"] != "—" {
		t.Fatalf("缺通关类型时不应显示玩家 PTT：%v", v)
	}
}

func TestValuesOfUsesEditedScoreDetails(t *testing.T) {
	catalogConstant, editedConstant, editedPTT := 10.5, 10.6, 12.3456
	chart := &catalog.Chart{Difficulty: "FTR", Constant: &catalogConstant}
	s := score.Song{Difficulty: score.DiffRef{Constant: editedConstant}, Score: score.ScoreRef{PTT: &editedPTT}}
	v := valuesOf(&catalog.Song{TitleEn: "Example"}, chart, s)
	if v["ptt"] != "+12.346" || v["diff"] != "Future 10.6" {
		t.Fatalf("手动校对值未进入画面：%v", v)
	}
}

func TestValuesOfShowsTotalAndShinyPure(t *testing.T) {
	song := &catalog.Song{TitleEn: "Example"}
	chart := &catalog.Chart{Difficulty: "FTR"}
	shiny := 567
	s := score.Song{Score: score.ScoreRef{Pure: 1234, ShinyPerfect: &shiny}}
	if got := valuesOf(song, chart, s)["pure"]; got != "1234 (+567)" {
		t.Fatalf("大 PURE 展示错误：%q", got)
	}
	shiny = 0
	if got := valuesOf(song, chart, s)["pure"]; got != "1234 (+0)" {
		t.Fatalf("真实 0 不应隐藏：%q", got)
	}
	s.Score.ShinyPerfect = nil
	if got := valuesOf(song, chart, s)["pure"]; got != "1234" {
		t.Fatalf("未知大 PURE 不应伪造：%q", got)
	}
	s.Score.UnreadJudgments = []string{"pure"}
	if got := valuesOf(song, chart, s)["pure"]; got != "—" {
		t.Fatalf("未读取总 PURE 不应显示数值：%q", got)
	}
}
