package catalog

import (
	"sort"
	"strings"
	"unicode"
)

// Find 是三级查找的前两级：① song_id 精确 → ② 归一化后撞 match_keys。
// 第二级的 key 用 match_keys 里的原值返回，让人能判断"凭什么命中"。
func (c *Catalog) Find(key string) (*Song, string, bool) {
	if key == "" {
		return nil, "", false
	}
	if s, ok := c.Song(key); ok {
		return s, key, true
	}
	n := NormalizeKey(key)
	for _, s := range c.Songs {
		for _, k := range s.MatchKeys {
			if k == n {
				return s, k, true
			}
		}
	}
	return nil, "", false
}

// Candidate 是"你要找的是不是它"。绝不自动套用（给一首曲配上别人的曲绘，比缺图糟糕得多）。
type Candidate struct {
	Song     *Song
	Key      string
	Distance int
}

// maxSuggestDistance 挡住"完全不着边也硬给 3 个候选" —— 那比没有候选更误导人。
const maxSuggestDistance = 3

// Candidates 按编辑距离给候选。距离在 loose 形态（去标点）上算，
// 所以 "cyagha" ↔ "cyaegha" 只差 1。
func (c *Catalog) Candidates(key string, n int) []Candidate {
	if key == "" || n <= 0 {
		return nil
	}
	target := looseKey(key)
	type best struct {
		s *Song
		k string
		d int
	}
	var hits []best
	for _, s := range c.Songs {
		bd, bk := -1, ""
		for _, k := range s.MatchKeys {
			d := editDistance(target, looseKey(k))
			if bd < 0 || d < bd {
				bd, bk = d, k
			}
		}
		if bd >= 0 && bd <= maxSuggestDistance {
			hits = append(hits, best{s, bk, bd})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].d < hits[j].d })
	if len(hits) > n {
		hits = hits[:n]
	}
	out := make([]Candidate, 0, len(hits))
	for _, h := range hits {
		out = append(out, Candidate{Song: h.s, Key: h.k, Distance: h.d})
	}
	return out
}

// looseKey 与 scripts 的 looseKey 对齐：去掉所有非字母数字。
func looseKey(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, NormalizeKey(s))
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, min(cur[j-1]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
