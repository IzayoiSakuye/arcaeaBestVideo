package ocr

import (
	"math"
	"testing"
)

// 表头三个数必须读对 —— 它们是这张图的校验和，读错一位整套自查就废了
func TestReadHeaderMatchesTruth(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skipf("没有样例图：%v", err)
	}
	want := fixtureHeaderOf(t)
	got := ReadHeader(img)

	if len(got.Problems) > 0 {
		t.Fatalf("有字段没读出来：%v", got.Problems)
	}
	if got.Top10Avg != want.Top10Avg {
		t.Errorf("TOP10 平均读成 %v，真值 %v", got.Top10Avg, want.Top10Avg)
	}
	if got.MaxPotential != want.MaxPotential {
		t.Errorf("MAX POTENTIAL 读成 %v，真值 %v", got.MaxPotential, want.MaxPotential)
	}
	if got.Top50Avg != want.Top50Avg {
		t.Errorf("TOP50 平均读成 %v，真值 %v", got.Top50Avg, want.Top50Avg)
	}
	// 玩家 PTT 这里印 5 位，游戏里是 3 位 —— 取 3 位应该跟头像角标上的一模一样
	badge := math.Round(got.MaxPotential*1000) / 1000
	if badge != 12.104 {
		t.Errorf("MAX POTENTIAL 取 3 位 = %v，头像角标上印的是 12.104", badge)
	}
	t.Logf("表头：TOP10 %v · MAX %v（取 3 位 %v）· TOP50 %v",
		got.Top10Avg, got.MaxPotential, badge, got.Top50Avg)
}
