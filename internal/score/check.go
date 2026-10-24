package score

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Check 负责自检
func Check(res *Resolved) []Problem {
	var ps []Problem
	ps = append(ps, checkSinglePTT(res)...)
	ps = append(ps, checkMonotonic(res)...)
	ps = append(ps, checkNoteSum(res)...)
	ps = append(ps, checkPrecision(res)...)
	ps = append(ps, checkRank(res)...)
	ps = append(ps, checkPerfectMemory(res)...)
	ps = append(ps, checkTotalPTT(res)...)
	sortProblems(ps)
	return ps
}

func fieldPath(i int, name string) string { return fmt.Sprintf("songs[%d].%s", i, name) }

// skipInfo 用于跳过若干行info
func skipInfo(rule string, total, skipped int, why string) Problem {
	return Problem{Rule: rule, Severity: SeverityInfo,
		Message: fmt.Sprintf("跳过 %d/%d 行：%s", skipped, total, why)}
}

func pttMismatch(row Row) bool {
	if row.Input.Score.PTT == nil || !row.PTTKnown {
		return false
	}
	d := FormatPtt(row.PTT) - FormatPtt(*row.Input.Score.PTT)
	return d > 1 || d < -1
}

// checkSinglePTT 用曲库定数算单曲 ptt，与图上写的比
func checkSinglePTT(res *Resolved) []Problem {
	const rule = "ptt-mismatch"
	var ps []Problem
	skipped := 0
	for i, row := range res.Rows {
		if row.Input.Score.PTT == nil || !row.PTTKnown {
			skipped++
			continue
		}

		if pttMismatch(row) {
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityError,
				Field:   fieldPath(i, "score.ptt"),
				Message: fmt.Sprintf("计算结果 %.4f，实际输入 %.4f", row.PTT, *row.Input.Score.PTT)})
		}
	}
	if skipped > 0 {
		ps = append(ps, skipInfo(rule, len(res.Rows), skipped, "缺少单曲ptt或结算等级"))
	}
	return ps
}

// checkMonotonic 检查b50分表自上而下递减的ptt
func checkMonotonic(res *Resolved) []Problem {
	const rule = "monotonic"
	var ps []Problem
	prev := math.NaN()
	for i, row := range res.Rows {
		if !row.PTTKnown {
			continue
		}
		if pttMismatch(row) {
			prev = math.NaN()
			continue
		}
		if !math.IsNaN(prev) && row.PTT > prev+1e-9 {
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityWarn,
				Message: fmt.Sprintf("ptt %.4f 高于 %.4f", row.PTT, prev)})
		}
		prev = row.PTT
	}
	return ps
}

// checkNoteSum 检查判定数之和是否是note数
func checkNoteSum(res *Resolved) []Problem {
	const rule = "note-sum"
	var ps []Problem
	skipped := 0
	for i, row := range res.Rows {
		if row.Chart == nil || row.Chart.Notes == nil {
			skipped++
			continue
		}
		sum := row.Input.Score.Pure + row.Input.Score.Far + row.Input.Score.Lost
		if sum == 0 || len(row.Input.Score.UnreadJudgments) > 0 {
			skipped++
			continue
		}
		if sum != *row.Chart.Notes {
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityWarn,
				Field:   fieldPath(i, "score"),
				Message: fmt.Sprintf("判定和 %d != 曲库物量 %d（差 %+d）", sum, *row.Chart.Notes, sum-*row.Chart.Notes)})
		}
	}
	if skipped > 0 {
		ps = append(ps, skipInfo(rule, len(res.Rows), skipped, "曲库缺物量或输入没填判定数"))
	}
	return ps
}

// checkPrecision 检查ptt精度
func checkPrecision(res *Resolved) []Problem {
	const rule = "ptt-precision"
	var ps []Problem
	if r := res.Project.Player.Rating; r > 0 && !isScaledInteger(r, 1000) {
		ps = append(ps, Problem{Rule: rule, Severity: SeverityError, Field: "player.rating",
			Message: fmt.Sprintf("玩家 PTT %v 超过 3 位小数", r)})
	}
	for i, row := range res.Rows {
		if p := row.Input.Score.PTT; p != nil && !isScaledInteger(*p, 10000) {
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityError,
				Field:   fieldPath(i, "score.ptt"),
				Message: fmt.Sprintf("单曲 ptt %v 超过 4 位小数", *p)})
		}
	}
	return ps
}

// isScaledInteger 判断整数，防止出现精度误差
func isScaledInteger(v, scale float64) bool {
	return math.Abs(v*scale-math.Round(v*scale)) < 1e-6
}

// checkRank 检查结果的rank是否正确
func checkRank(res *Resolved) []Problem {
	const rule = "rank-mismatch"
	var ps []Problem
	for i, row := range res.Rows {
		got := row.Input.Score.Rank
		if got == "" || row.Rank == "" {
			continue
		}
		if !strings.EqualFold(got, row.Rank) {
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityError,
				Field:   fieldPath(i, "score.rank"),
				Message: fmt.Sprintf("评级 %q 与分数不符（应为 %s）", got, row.Rank)})
		}
	}
	return ps
}

// checkPerfectMemory 检查PM可能性
func checkPerfectMemory(res *Resolved) []Problem {
	const rule = "pm-score"
	var ps []Problem
	skipped := 0
	for i, row := range res.Rows {
		ct := row.Input.Score.ClearType
		if ct == "" {
			skipped++
			continue
		}
		score := row.Input.Score.Score()
		isPM := strings.EqualFold(ct, "PM")
		switch {
		case isPM && score < 10_000_000:
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityError,
				Field:   fieldPath(i, "score.clear_type"),
				Message: fmt.Sprintf("PM 的分数不可能低于 1000 万（得 %d）", score)})
		case !isPM && score >= 10_000_000:
			ps = append(ps, Problem{SongIndex: i + 1, Rule: rule, Severity: SeverityError,
				Field:   fieldPath(i, "score.clear_type"),
				Message: fmt.Sprintf("分数 %d ≥ 1000 万，通关类型必须是 PM（得 %q）", score, ct)})
		}
	}
	if skipped > 0 {
		ps = append(ps, skipInfo(rule, len(res.Rows), skipped, "输入没填通关类型"))
	}
	return ps
}

// checkTotalPTT 用所有50首的计算ptt
func checkTotalPTT(res *Resolved) []Problem {
	const rule = "total-ptt-mismatch"
	n := len(res.Rows)
	if n != 50 {
		return []Problem{skipInfo(rule, n, n, fmt.Sprintf("总数少于 50 首（%d 首）", n))}
	}
	rating := res.Project.Player.Rating
	if rating <= 0 {
		return []Problem{skipInfo(rule, n, n, "没有玩家总 PTT")}
	}
	ptts := make([]float64, 0, n)
	for _, row := range res.Rows {
		if row.PTTKnown {
			ptts = append(ptts, row.PTT)
		}
	}
	if missing := n - len(ptts); missing > 0 {
		return []Problem{{Rule: rule, Severity: SeverityInfo,
			Message: fmt.Sprintf("总 PTT 校验未执行：%d/%d 首算不出单曲 PTT", missing, n)}}
	}
	slices.Sort(ptts)
	var sum50, sum10 float64
	for _, p := range ptts {
		sum50 += p
	}
	for _, p := range ptts[n-10:] {
		sum10 += p
	}
	want := (sum50 + sum10) / 60
	if math.Abs(want-rating) > 1e-3 {
		return []Problem{{Rule: rule, Severity: SeverityError, Field: "player.rating",
			Message: fmt.Sprintf("50 行重算 %.4f，图上 %.3f（差 %+.4f）", want, rating, want-rating)}}
	}
	return nil
}
