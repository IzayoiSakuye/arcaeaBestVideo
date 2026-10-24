package ocr

import (
	"slices"
	"testing"
)

func TestParseJudgmentText(t *testing.T) {
	f := parseJudgmentText("P:1335(+1193)F:5L:7")
	if f.Pure != 1335 || f.ShinyPerfect == nil || *f.ShinyPerfect != 1193 || f.Far != 5 || f.Lost != 7 || len(f.Problems) != 0 {
		t.Fatalf("判定行解析错误：%+v", f)
	}
	f = parseJudgmentText("P:1335(+1193)F:L:7")
	if f.Pure != 1335 || f.ShinyPerfect == nil || *f.ShinyPerfect != 1193 || f.Lost != 7 || !slices.Equal(f.UnreadJudgments, []string{"far"}) {
		t.Fatalf("FAR 漏读时应只留空 FAR：%+v", f)
	}
	f = parseJudgmentText("P:1397(+1335) F.3 L:5")
	if f.Far != 3 || len(f.UnreadJudgments) != 0 {
		t.Fatalf("冒号认成句点时应保留数字：%+v", f)
	}
	f = parseJudgmentText("P:12345(+1111) F:2 L:3")
	if !slices.Contains(f.UnreadJudgments, "pure") {
		t.Fatalf("超长数字不能截断后当成 PURE：%+v", f)
	}
}

func TestParseOtherText(t *testing.T) {
	if f := parseScoreText("10'000'972"); f.Score != 10000972 || len(f.Problems) != 0 {
		t.Fatalf("满分分数识别错误：%+v", f)
	}
	if f := parseScoreText("09'96S'409"); f.Score != 0 || len(f.Problems) == 0 {
		t.Fatalf("分数中有错字应提示校对：%+v", f)
	}
	if f := parsePotentialText("Potential 10.5 > 12.5420 HC"); f.Constant != 10.5 || f.PTT != 12.5420 || f.ClearType != "HC" || len(f.Problems) != 0 {
		t.Fatalf("单曲成绩识别错误：%+v", f)
	}
	if f := parsePotentialText("Potential 10,5 > 12,5420 NC"); f.Constant != 10.5 || f.PTT != 12.5420 || f.ClearType != "NC" {
		t.Fatalf("小数点误读后应按格式解析：%+v", f)
	}
	if h := parseHeaderText("12.37583", "12,10442", "12.05013"); h.Top10Avg != 12.37583 || h.MaxPotential != 12.10442 || h.Top50Avg != 12.05013 || len(h.Problems) != 0 {
		t.Fatalf("表头识别错误：%+v", h)
	}
}
