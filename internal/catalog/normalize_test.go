package catalog

import "testing"

func loadReal(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(realCatalogPath(), supportedSchema)
	if err != nil {
		t.Skipf("真实目录不可用（还没跑数据管线？）：%v", err)
	}
	return c
}

// 曲库里每首歌都同时存着 title_en（原始）和 title_norm（归一化）—— 那 564 对
// 就是现成的对拍样例：自带、自动同步、零额外产物。它同时是跨语言一致性测试
// （Go 的归一化必须复刻 scripts/build-catalog.ts）和回归测试（曲库刷新后自动跟着变）。
func TestNormalizeKeyMatchesCatalog(t *testing.T) {
	c := loadReal(t)
	bad := 0
	for _, s := range c.Songs {
		if got := NormalizeKey(s.TitleEn); got != s.TitleNorm {
			bad++
			t.Errorf("%s: NormalizeKey(%q) = %q, 曲库给的是 %q", s.SongID, s.TitleEn, got, s.TitleNorm)
		}
	}
	t.Logf("对拍 %d 首，失配 %d 首", len(c.Songs), bad)
}

func TestNormalizeKeyTricky(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Cyaëgha", "cyaegha"},           // 预组合变音符号（b50 图上就是这么写的）
		{"Ouvertüre", "ouverture"},       //
		{" ͟͝͞Ⅱ́̕ ", "ii"},               // 罗马数字 Ⅱ → II，组合记号丢掉
		{"INCARNATOR₀₀", "incarnator00"}, // 下标数字
		{"GLORY：ROAD", "glory:road"},     // 全角冒号 → 半角
		{"  Testify  ", "testify"},       // 折叠空白
		{"Don’t", "don't"},               // 弯引号 → 直引号
		{"A－B", "a-b"},                   // 全角减号
	}
	for _, tc := range cases {
		if got := NormalizeKey(tc.in); got != tc.want {
			t.Errorf("NormalizeKey(%q) = %q，期望 %q", tc.in, got, tc.want)
		}
	}
}

// 挡住 json tag 写错：数据的键是单数 note，写成 notes 会永远解出 nil（且不报错）。
func TestChartNoteIsParsed(t *testing.T) {
	c := loadReal(t)
	total, withNote := 0, 0
	for _, s := range c.Songs {
		for _, ch := range s.Charts {
			total++
			if ch.Notes != nil {
				withNote++
			}
		}
	}
	if total == 0 {
		t.Fatal("目录里一个谱面都没有")
	}
	if ratio := float64(withNote) / float64(total); ratio < 0.5 {
		t.Errorf("只有 %d/%d 个谱面解出 note —— json tag 大概写错了（note vs notes）", withNote, total)
	}
	t.Logf("note 覆盖 %d/%d", withNote, total)
}

func TestFind(t *testing.T) {
	c := loadReal(t)

	if s, _, ok := c.Find("egoeimi"); !ok || s.SongID != "egoeimi" {
		t.Errorf("① song_id 精确查找失败")
	}
	// ② 归一化后撞 match_keys：图上写 Cyaëgha，曲库键是 cyaegha
	if s, key, ok := c.Find("Cyaëgha"); !ok || s.SongID != "cyaegha" {
		t.Errorf("② 变音符号归一化查找失败：ok=%v key=%q", ok, key)
	}
	if s, _, ok := c.Find("绿魔王"); !ok || s.SongID != "cyaegha" {
		t.Errorf("② 中文别名查找失败")
	}
	if _, _, ok := c.Find("__不存在的曲__"); ok {
		t.Errorf("查不到的输入不该命中")
	}
	if _, _, ok := c.Find(""); ok {
		t.Errorf("空串不该命中")
	}
}

func TestCandidates(t *testing.T) {
	c := loadReal(t)

	// "cyagha" 正是"把 ë 整个删掉"的产物（归一化写错时的典型现象）。
	// 这种情况下用户不该看到"找不到"，而该看到候选。
	got := c.Candidates("cyagha", 3)
	found := false
	for _, cand := range got {
		if cand.Song.SongID == "cyaegha" {
			found = true
		}
	}
	if !found {
		t.Errorf("Candidates(cyagha) 里应该有 cyaegha，实得 %v", got)
	}

	// 完全不着边的输入不该硬凑候选
	if n := len(c.Candidates("zzzzzzzzzzzzzzzz", 3)); n != 0 {
		t.Errorf("不着边的输入不该给候选，却给了 %d 个", n)
	}
}
