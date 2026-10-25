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
