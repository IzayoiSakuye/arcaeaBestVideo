package score

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// x10000 把 ptt 换成整数再比较，避开浮点等值判断。
// 刻意不复用 FormatPtt：拿被测的函数当比较工具，它的 bug 会跟着一起被"验证通过"。
func x10000(v float64) int64 { return int64(math.Round(v * 10000)) }

// ── A 组：对拍独立 oracle ────────────────────────────────────────────────
// 期望值来自 scripts/check-ptt-formula.ts（wiki 潜力值页的 7 个分数节点）。
// 12 项里 TL 取 0 奖励、FR 取 +0.2 —— 实测 HC 也 +0.2（docs/04 §4.3），所以只区分"有没有奖励"。
func TestSinglePTTMatchesOracleNodes(t *testing.T) {
	const c = 11.0 // 只看相对定数的偏移，和 oracle 取同一个定数

	cases := []struct {
		score int
		clear ClearType
		delta float64
	}{
		{10_000_000, TL, 2.0}, {10_000_000, FR, 2.2},
		{9_900_000, TL, 1.5}, {9_900_000, FR, 1.7},
		{9_800_000, TL, 1.0}, {9_800_000, FR, 1.2},
		{9_500_000, TL, 0.0}, {9_500_000, FR, 0.2},
		{9_200_000, TL, -1.0}, {9_200_000, FR, -0.8},
		{8_900_000, TL, -2.0}, {8_900_000, FR, -1.8},
		{8_600_000, TL, -3.0}, {8_600_000, FR, -2.8},
	}

	for _, tc := range cases {
		got, err := SinglePTT(c, tc.score, tc.clear)
		if err != nil {
			t.Errorf("score=%d clear=%s 不该报错：%v", tc.score, tc.clear, err)
			continue
		}
		if want := x10000(c + tc.delta); x10000(got) != want {
			t.Errorf("score=%d clear=%s：得 %v（%d），期望 %v（%d）",
				tc.score, tc.clear, got, x10000(got), c+tc.delta, want)
		}
	}
}

// 分数补正本身可能是负的，夹 0 必须在**加完通关奖励之后**。
// 定数 1.0 + 8,600,000：1.0 - 3.0 = -2.0 → 0（不是 -2.0，也不是 0.2）
func TestSinglePTTClampsAtZero(t *testing.T) {
	for _, c := range []ClearType{TL, FR} {
		got, err := SinglePTT(1.0, 8_600_000, c)
		if err != nil {
			t.Fatalf("clear=%s 不该报错：%v", c, err)
		}
		if got != 0 {
			t.Errorf("clear=%s：得 %v，期望 0（负数必须夹到 0）", c, got)
		}
	}
}

// ── 通关奖励 ────────────────────────────────────────────────────────────
func TestClearBonus(t *testing.T) {
	// TL 是唯一的 0，其余一律 +0.2 —— 实测 HC 也拿 +0.2（docs/04 §4.3）。
	// 写成"列举 PM/FR 才 +0.2"的白名单会让 HC 漏成 0，于是每一行都被判成错的。
	if b, err := ClearBonus(TL); err != nil || b != 0 {
		t.Errorf("TL：得 (%v, %v)，期望 (0, nil)", b, err)
	}
	for _, c := range []ClearType{NC, HC, EC, FR, PM} {
		b, err := ClearBonus(c)
		if err != nil {
			t.Errorf("%s 不该报错：%v", c, err)
			continue
		}
		if x10000(b) != 2000 {
			t.Errorf("%s：得 %v，期望 0.2", c, b)
		}
	}
	// 不认识的类型必须报错，不能默默按 0 或 +0.2 算 ——
	// ①⑥ 要据此报 info「无法校验」，而不是给出一个看起来正常的数。
	for _, c := range []ClearType{"", "??", "TCX"} {
		if _, err := ClearBonus(c); err == nil {
			t.Errorf("%q 是未知通关类型，必须报错", c)
		}
	}
}

// ── 精度：必须四舍五入，不能截断 ────────────────────────────────────────
func TestFormatPttRounds(t *testing.T) {
	cases := []struct {
		in   float64
		want int64
		why  string
	}{
		{12.413075, 124131, "截断会给 124130"},
		{12.4371, 124371, "截断会给 124370（真实夹具第 3 行就是它）"},
		{12.26349, 122635, "截断会给 122634"},
		{12.5420, 125420, "整值不受影响"},
	}
	for _, tc := range cases {
		if got := FormatPtt(tc.in); got != tc.want {
			t.Errorf("FormatPtt(%v)：得 %d，期望 %d（%s）", tc.in, got, tc.want, tc.why)
		}
	}
}

// ── B 组：真实 b50 夹具 ─────────────────────────────────────────────────
type b50Fixture struct {
	Header struct {
		BestTop10Avg float64 `json:"best_top10_avg"`
		BestTop50Avg float64 `json:"best_top50_avg"`
	} `json:"header"`
	Songs []struct {
		Index     int     `json:"index"`
		Title     string  `json:"title"`
		Score     int     `json:"score"`
		Constant  float64 `json:"constant"`
		PTT       float64 `json:"ptt"`
		ClearType string  `json:"clear_type"`
	} `json:"songs"`
}

func loadB50Fixture(t *testing.T) b50Fixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "b50-from-image-10.json"))
	if err != nil {
		t.Fatalf("读夹具失败：%v", err)
	}
	var f b50Fixture
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("解析夹具失败：%v", err)
	}
	if len(f.Songs) == 0 {
		t.Fatal("夹具里没有 songs")
	}
	return f
}

// 用公式重算图上那 10 行，必须逐行命中。这组比 A 组更有价值：
// A 组只证明"和 oracle 抄了同一份文档"，B 组证明公式能解释真实玩家数据。
func TestSinglePTTOnRealB50(t *testing.T) {
	f := loadB50Fixture(t)
	for _, s := range f.Songs {
		got, err := SinglePTT(s.Constant, s.Score, ClearType(s.ClearType))
		if err != nil {
			t.Errorf("#%d %s：%v", s.Index, s.Title, err)
			continue
		}
		if gotX, wantX := x10000(got), x10000(s.PTT); gotX != wantX {
			t.Errorf("#%d %s：得 %.6f（%d），图上 %.4f（%d）",
				s.Index, s.Title, got, gotX, s.PTT, wantX)
		}
	}
}

// clear_type 缺失时的区间必须罩住图上值（docs/04 §2 的 ⑥ 裁决）。
func TestSinglePTTRangeCoversDisplayedPTT(t *testing.T) {
	f := loadB50Fixture(t)
	for _, s := range f.Songs {
		lo, hi := SinglePTTRange(s.Constant, s.Score)
		// 图上值是 round4 过的，给半个最后一位的余量
		if s.PTT < lo-5e-5 || s.PTT > hi+5e-5 {
			t.Errorf("#%d %s：图上 %.4f 落在 [%.4f, %.4f] 之外", s.Index, s.Title, s.PTT, lo, hi)
		}
		if x10000(hi-lo) != 2000 {
			t.Errorf("#%d %s：区间宽度应恰好 0.2，得 %.6f", s.Index, s.Title, hi-lo)
		}
	}
}

// ── C 组：夹具自证 ──────────────────────────────────────────────────────
// 这条不测公式，测数据。图上的表头写着 BEST TOP10 AVG = 12.37583，
// 所以 Σ(前 10 名单曲 ptt) 必须 = 123.7583 —— 将来补抄剩下 40 行时，抄错一位这条就红。
// 能抓住：整行抄错 / 定数错 0.1 / 通关类型错（都是 ≥0.1 量级）
// 抓不住：分数最后一位差 1（只影响 5e-6）
func TestFixtureChecksumMatchesHeader(t *testing.T) {
	f := loadB50Fixture(t)

	var sumDisplayed, sumComputed float64
	for _, s := range f.Songs {
		sumDisplayed += s.PTT
		if ptt, err := SinglePTT(s.Constant, s.Score, ClearType(s.ClearType)); err == nil {
			sumComputed += ptt
		}
	}

	// 表头平均值是 5 位小数，每行 round4 最多贡献 5e-5 → 取 1e-4/行 的容差
	want := f.Header.BestTop10Avg * float64(len(f.Songs))
	tol := 1e-4 * float64(len(f.Songs))

	for name, sum := range map[string]float64{"夹具字段": sumDisplayed, "公式重算": sumComputed} {
		if math.Abs(sum-want) > tol {
			t.Errorf("%s 的 Σ=%.4f，由表头推得 %.4f（容差 %.4f）", name, sum, want, tol)
		}
	}
}

// ── 评级：表征测试，**不是**验证测试 ────────────────────────────────────
// 评级与通关类型是**两套独立的词表**：
//
//	评级     = EX+ / EX / AA / A / B / C / D（只由分数决定）
//	通关类型 = TL / NC / EC / HC / FR / PM（另一根轴，影响 ptt 奖励）
//
// PM 属于通关类型那一族 —— 打出 PM 时，评级栏显示的是 EX+。
// b50 图上不印评级（docs/04 §4.2），所以这张阈值表至今没有独立来源验证过；
// 本测试只锁定当前实现、防止无意改动。
func TestRankBoundaries(t *testing.T) {
	cases := []struct {
		score int
		want  string
	}{
		{10_000_000, "EX+"}, // PM 时也是 EX+
		{9_900_000, "EX+"},
		{9_899_999, "EX"},
		{9_800_000, "EX"},
		{9_500_000, "AA"},
		{9_200_000, "A"},
		{8_900_000, "B"},
		{8_600_000, "C"},
		{8_599_999, "D"},
		{0, "D"},
	}

	// 用例表自己也要防呆：评级里出现通关类型，就是两套词表被混了
	clearTypes := map[string]bool{"TL": true, "NC": true, "EC": true, "HC": true, "FR": true, "PM": true}
	for _, tc := range cases {
		if clearTypes[tc.want] {
			t.Fatalf("%q 是通关类型，不是评级 —— 两套词表别混", tc.want)
		}
	}

	for _, tc := range cases {
		if got := Rank(tc.score); got != tc.want {
			t.Errorf("Rank(%d)：得 %q，期望 %q", tc.score, got, tc.want)
		}
	}
}
