package job

import (
	"testing"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/score"
)

func TestValuesOfUsesPlayerRatingAndUnknownJudgements(t *testing.T) {
	c := 10.5
	v := valuesOf(&catalog.Song{TitleEn: "Example"}, &catalog.Chart{Difficulty: "FTR", Constant: &c},
		score.Song{Score: score.ScoreRef{Achievement: 99.5}}, 12.104)
	if v["ptt"] != "+12.104" || v["pure"] != "—" || v["far"] != "—" || v["lost"] != "—" {
		t.Fatalf("成绩展示失真：%v", v)
	}
}
