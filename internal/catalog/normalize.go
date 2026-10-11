package catalog

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormalizeKey 规格化key，保证和数据一致
func NormalizeKey(s string) string {
	t := norm.NFD.String(norm.NFKC.String(s))
	t = strings.Map(func(r rune) rune {
		if r >= 0x0300 && r <= 0x036f {
			return -1 // 组合记号：丢掉，与 TS 的 /[\u0300-\u036f]/g 对齐
		}
		return r
	}, t)
	t = quoteReplacer.Replace(t)
	t = dashReplacer.Replace(t)
	return strings.ToLower(strings.Join(strings.Fields(t), " "))
}

var (
	quoteReplacer = strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", "\u02bc", "'",
		"\u201c", `"`, "\u201d", `"`)
	dashReplacer = strings.NewReplacer(
		"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-",
		"\u2014", "-", "\u2015", "-", "\u2212", "-")
)
