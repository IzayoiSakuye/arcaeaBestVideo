package score

import (
	"fmt"
	"math"
	"slices"
)

type ClearType string

const (
	TL ClearType = "TL"
	NC ClearType = "NC"
	HC ClearType = "HC"
	EC ClearType = "EC"
	FR ClearType = "FR"
	PM ClearType = "PM"
)

// ClearBonus 用于计算达成类型给予的定数加分
func ClearBonus(c ClearType) (float64, error) {
	if !slices.Contains([]ClearType{TL, NC, HC, EC, FR, PM}, c) {
		return 0.0, fmt.Errorf("无效的通关类型 %q", c)
	}
	if c == "TL" {
		return 0.0, nil
	}
	return 0.2, nil
}

// SinglePTT 计算单曲ptt
func SinglePTT(constant float64, score int, c ClearType) (float64, error) {
	bounce, err := ClearBonus(c)
	if err != nil {
		return 0.0, fmt.Errorf("单曲 PTT 计算失败：%w", err)
	}
	if score >= 10000000 {
		return constant + 2.0 + bounce, nil
	} else if score >= 9800000 {
		return constant + 1.0 + (float64(score-9800000) / 200000) + bounce, nil
	}

	return max(0, constant+(float64(score-9500000)/300000)+bounce), nil
}

// SinglePTTRange 计算ClearType缺失时的单曲ptt范围
func SinglePTTRange(constant float64, score int) (lo, hi float64) {
	tlPtt, _ := SinglePTT(constant, score, TL)
	pmPtt, _ := SinglePTT(constant, score, PM)
	return tlPtt, pmPtt
}

// FormatPtt 将小数ptt变为整数，便于比较
func FormatPtt(ptt float64) int64 {
	return int64(math.Round(ptt * 10000))
}

// Rank 根据分数定等级
func Rank(score int) string {
	if score >= 9900000 {
		return "EX+"
	} else if score >= 9800000 {
		return "EX"
	} else if score >= 9500000 {
		return "AA"
	} else if score >= 9200000 {
		return "A"
	} else if score >= 8900000 {
		return "B"
	} else if score >= 8600000 {
		return "C"
	}
	return "D"
}
