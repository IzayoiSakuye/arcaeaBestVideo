package catalog

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const fixtureRel = "testdata/catalog-mini.json"

// fixtureSchema 是夹具里写的 schema_version。改 testdata/catalog-mini.json 时同步改这里。
//
// ⚠️ 故意写死，不去读 JSON —— 从文件里读出来的话，
// 「数据版本 == 代码支持版本」就恒成立，Load 里那条版本闸门等于没测。
const fixtureSchema = 1

// supportedSchema 是【代码】能读的目录格式版本（对齐 Load 的第二个参数）。
//
// 它和 fixtureSchema 现在都是 1，但两者是不同的东西：
// 前者是「库能读什么」，后者是「这份数据是什么」。
// 把夹具升到 2 而代码没跟上时，下面会立刻红 —— 这正是想要的行为。
const supportedSchema = 1

// realCatalogPath 是真实目录的位置。克隆仓库后可能还没跑过数据管线，
// 所以凡是碰它的测试一律「没有就 Skip」。
func realCatalogPath() string {
	return filepath.Join("..", "..", "data", "arcaea-catalog.json")
}

// sources 把 *[]VideoSource 解引用成 []VideoSource，nil 安全。
//
// 注：字段类型是 *[]VideoSource，每个调用点都得先解引用一次。
// 若改成 []VideoSource，这个 helper 就能删掉（nil slice 的 len() 本来就是 0）。
func sources(v *Video) []VideoSource {
	if v == nil || v.Sources == nil {
		return nil
	}
	return *v.Sources
}

// loadFixture 用指定的 supportedSchema 加载夹具。
// 参数名不叫 supportedSchema，避免遮蔽同名常量。
func loadFixture(t *testing.T, schema int) *Catalog {
	t.Helper()
	c, err := Load(fixtureRel, schema)
	if err != nil {
		t.Fatalf("加载夹具失败: %v", err)
	}
	return c
}

// ── 加载 ────────────────────────────────────────────────────────────────────

func TestLoadFixture(t *testing.T) {
	c := loadFixture(t, supportedSchema)

	// 这条同时守住两件事：
	//   ① json tag 写错（比如写成 "version"）会让它恒为 0；
	//   ② 有人升了夹具的 schema_version 却忘了改 fixtureSchema 常量。
	if c.SchemaVersion != fixtureSchema {
		t.Errorf("SchemaVersion = %d, 期望 %d", c.SchemaVersion, fixtureSchema)
	}
	if c.Game != "arcaea" {
		t.Errorf("Game = %q, 期望 %q", c.Game, "arcaea")
	}
	if len(c.Songs) != 3 {
		t.Fatalf("Songs = %d 首, 期待夹具里的 3 首", len(c.Songs))
	}
}

func TestLoadRealCatalog(t *testing.T) {
	path := realCatalogPath()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("真实目录不存在（%s）—— 未跑过数据管线或克隆的仓库，跳过", path)
	}

	c, err := Load(path, supportedSchema)
	if err != nil {
		t.Fatalf("加载真实目录失败: %v", err)
	}

	// ⚠️ 不断言固定数量（533 / 1350）—— 数据一刷新就红。
	// 断言结构不变量。
	if len(c.Songs) == 0 {
		t.Fatal("Songs 为空")
	}

	// ⭐ 这条是 `json:"video_source"` 那个 tag bug 的回归哨兵。
	// tag 拼错时 Video.Sources 恒为 nil，全目录一条都解析不出来。
	// 用「至少有一条」而不是「testify 必须有」—— 换曲库也不会误红。
	withVideo := 0
	for _, s := range c.Songs {
		for _, ch := range s.Charts {
			if len(sources(ch.Video)) > 0 {
				withVideo++
			}
		}
	}
	if withVideo == 0 {
		t.Error("全目录没有任何一条谱面解析出 video.sources —— 检查 Video.Sources 的 json tag")
	}
	t.Logf("真实目录: %d 首, %d 条谱面带录像", len(c.Songs), withVideo)
}

func TestLoadErrors(t *testing.T) {
	t.Run("文件不存在", func(t *testing.T) {
		path := filepath.Join("testdata", "绝对不存在的文件.json")
		_, err := Load(path, supportedSchema)
		if err == nil {
			t.Fatal("期望报错，实际成功")
		}
		// 规格 §1.3 要求：错误信息里必须含路径，否则用户不知道去哪找文件
		if !strings.Contains(err.Error(), path) {
			t.Errorf("错误信息里没有路径 %q: %v", path, err)
		}
	})

	t.Run("JSON 非法", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.json")
		if err := os.WriteFile(path, []byte(`{"songs": [ this is not json`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path, supportedSchema)
		if err == nil {
			t.Fatal("期望报错，实际成功")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("错误信息里没有路径: %v", err)
		}
		// ⚠️ 曾经这里格式化的是 os.ReadFile 的 err（此时恒为 nil），
		// 结果是 "%!v(<nil>)"，真正的语法错误被丢掉。这条断言防止它回归。
		// json 的报错里一定带 "invalid character" 之类，且不该出现 %!v
		if strings.Contains(err.Error(), "%!") {
			t.Errorf("解析错误被丢弃了（格式化的是别的 err）: %v", err)
		}
	})

	t.Run("songs 为空", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.json")
		if err := os.WriteFile(path, []byte(`{"schema_version":1,"songs":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path, supportedSchema); err == nil {
			t.Fatal("空 songs 期望报错，实际成功")
		}
	})

	t.Run("path 为空", func(t *testing.T) {
		if _, err := Load("", supportedSchema); err == nil {
			t.Fatal("空 path 期望报错，实际成功")
		}
	})
}

// ── 版本闸门 ────────────────────────────────────────────────────────────────

// TestSchemaVersionGate 覆盖 Load 里的 schema 校验。
//
// 构造"不匹配"不需要另造 JSON —— 夹具的数据是 fixtureSchema，
// 把 Load 的第二个参数换个值就得到了不匹配。
// 比手写一份 schema 不同的 JSON 可靠：手写的那个将来会和真 schema 悄悄漂移。
func TestSchemaVersionGate(t *testing.T) {
	t.Run("版本一致", func(t *testing.T) {
		if _, err := Load(fixtureRel, fixtureSchema); err != nil {
			t.Errorf("数据和代码都是 schema %d，应该成功: %v", fixtureSchema, err)
		}
	})

	// ⚠️ 当前实现是【严格相等】——数据比代码旧、比代码新，一律拦。
	// 这是有意的：schema 一变就意味着格式变了，宁可让用户重下数据包，
	// 也不要拿半懂的格式去渲染。
	// 哪天真要兼容旧数据包，改的是 Load，而这条用例会跟着红 ——
	// 那时候是【有意识地】放开，不是悄悄放开。
	cases := []struct {
		name string
		code int
	}{
		{"代码比数据新", fixtureSchema + 1},
		{"代码比数据旧", fixtureSchema - 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(fixtureRel, tc.code)
			if err == nil {
				t.Fatalf("数据 schema=%d、代码支持 %d，应该报错", fixtureSchema, tc.code)
			}

			// 错误信息必须【同时】给出两边版本号。
			// 只说一句 "version mismatch"，用户不知道该更新应用还是重下数据包 ——
			// 这两件事的动作完全不同（架构决策 6：数据与代码分离发布）。
			msg := err.Error()
			for _, want := range []int{fixtureSchema, tc.code} {
				if !strings.Contains(msg, strconv.Itoa(want)) {
					t.Errorf("错误信息里没有版本号 %d: %v", want, err)
				}
			}
		})
	}

	t.Run("先于 songs 校验", func(t *testing.T) {
		// 版本闸门必须在"数据内容"校验之前。
		// 否则用户拿新版数据包配旧应用时，先看到的会是一堆莫名其妙的字段错误，
		// 而不是"你该更新应用了"。
		path := filepath.Join(t.TempDir(), "future.json")
		if err := os.WriteFile(path, []byte(`{"schema_version":99,"songs":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path, fixtureSchema)
		if err == nil {
			t.Fatal("期望报错，实际成功")
		}
		if !strings.Contains(err.Error(), "99") {
			t.Errorf("应该先报版本问题（99），实际报的是别的（多半是 songs 为空）: %v", err)
		}
	})
}

// ── 查询 ────────────────────────────────────────────────────────────────────

func TestSongLookup(t *testing.T) {
	c := loadFixture(t, supportedSchema)

	s, ok := c.Song("complete")
	if !ok {
		t.Fatal(`Song("complete") 未命中`)
	}
	if s.TitleEn != "Complete Song" {
		t.Errorf("TitleEn = %q", s.TitleEn)
	}
	if s.SetFriendly != "Arcaea" {
		t.Errorf("SetFriendly = %q（tag 写成 set_norm 之类的就会是空）", s.SetFriendly)
	}

	for _, miss := range []string{"", "__不存在的id__"} {
		if _, ok := c.Song(miss); ok {
			t.Errorf("Song(%q) 不该命中", miss)
		}
	}
}

func TestChartLookup(t *testing.T) {
	c := loadFixture(t, supportedSchema)

	if _, ok := c.Chart("complete", "FTR"); !ok {
		t.Error(`Chart("complete","FTR") 未命中`)
	}

	// ⚠️ 曾经这里 return nil, true —— ok 为真但值是 nil，
	// 调用方按惯例写 `if ch, ok := ...; ok { *ch.Level }` 会直接 panic。
	cases := []struct{ song, diff string }{
		{"complete", "PST"}, // 曲子在，难度不在
		{"__不存在__", "FTR"},  // 曲子不在
		{"", "FTR"},         // song_id 为空
	}
	for _, tc := range cases {
		ch, ok := c.Chart(tc.song, tc.diff)
		if ok {
			t.Errorf("Chart(%q,%q) 不该命中", tc.song, tc.diff)
		}
		if ch != nil {
			t.Errorf("Chart(%q,%q) 返回了非 nil 的谱面", tc.song, tc.diff)
		}
	}
}

// ── 字段真的解析出来了吗（tag 回归） ────────────────────────────────────────

func TestVideoParsedFromJSON(t *testing.T) {
	c := loadFixture(t, supportedSchema)

	ch, ok := c.Chart("complete", "FTR")
	if !ok {
		t.Fatal("夹具里应该有 complete/FTR")
	}
	if ch.Video == nil {
		t.Fatal("Video 是 nil —— 检查 Chart.Video 的 json tag")
	}
	src := sources(ch.Video)
	if len(src) != 1 {
		t.Fatalf("sources = %d 条, 期望 1 —— 检查 Video.Sources 的 json tag（写成 video_source 就会是 0）", len(src))
	}

	if src[0].BVID == nil || *src[0].BVID != "BV1xx411c7mD" {
		t.Errorf("BVID 没解析出来: %v", src[0].BVID)
	}
	if src[0].P != 1 {
		t.Errorf("P = %d, 期望 1", src[0].P)
	}
	if src[0].URL == "" {
		t.Error("URL 为空")
	}
	if src[0].DurationSec != 180 {
		t.Errorf("DurationSec = %d, 期望 180", src[0].DurationSec)
	}
	if len(src[0].Collections) != 1 {
		t.Errorf("Collections = %v, 期望 1 个", src[0].Collections)
	}
	// 署名用（铁律 8），不能是空
	if src[0].OwnerName == "" {
		t.Error("OwnerName 为空 —— 署名机制依赖它")
	}
	if src[0].VideoWidth == nil || *src[0].VideoWidth != 1920 {
		t.Errorf("VideoWidth = %v, 期望 1920", src[0].VideoWidth)
	}

	// mixed/BYD 有两个来源，第二个故意不带画幅字段（模拟上游没给）
	mixed, ok := c.Chart("mixed", "BYD")
	if !ok {
		t.Fatal("夹具里应该有 mixed/BYD")
	}
	msrc := sources(mixed.Video)
	if len(msrc) != 2 {
		t.Fatalf("mixed/BYD sources = %d 条, 期望 2", len(msrc))
	}
	if msrc[1].VideoWidth != nil || msrc[1].VideoHeight != nil {
		t.Error("第二个来源不该有画幅字段（夹具里就是没给），别伪造")
	}
	if len(msrc[0].Collections) != 2 {
		t.Errorf("第一个来源应有 2 个合集, 实际 %v", msrc[0].Collections)
	}
	// 第二个来源的 collections 是 []（空数组，不是 null）—— 也是合法形态
	if len(msrc[1].Collections) != 0 {
		t.Errorf("第二个来源应是空合集, 实际 %v", msrc[1].Collections)
	}
}

func TestJacketParsedFromJSON(t *testing.T) {
	c := loadFixture(t, supportedSchema)

	s, _ := c.Song("complete")
	if s.Jacket == nil {
		t.Fatal("Jacket 是 nil")
	}
	if !s.Jacket.Bundled {
		t.Error("Bundled 应为 true")
	}
	if s.Jacket.Local256 == nil || *s.Jacket.Local256 != "data/jackets/256/complete.jpg" {
		t.Errorf("Local256 = %v", s.Jacket.Local256)
	}
	// ⭐ 回归哨兵：tag 曾写成 "full_url_256"，导致这个字段恒为 nil
	if s.Jacket.FullURL == nil {
		t.Fatal("FullURL 是 nil —— 检查 Jacket.FullURL 的 json tag")
	}
	if *s.Jacket.FullURL != "https://example.invalid/Songs_complete.jpg" {
		t.Errorf("FullURL = %q", *s.Jacket.FullURL)
	}
	if s.Jacket.Native == nil || *s.Jacket.Native != "768x768" {
		t.Errorf("Native = %v", s.Jacket.Native)
	}

	// mixed 的 jacket 故意没有 thumb_url_256 这个键
	mixed, _ := c.Song("mixed")
	if mixed.Jacket.ThumbURL256 != nil {
		t.Error("夹具里 mixed 没有 thumb_url_256 键，应为 nil")
	}
}

// TestNullableFieldsAreNil 覆盖「产物里真的会出现的空」。
//
// 重点不是"值等于什么"，而是【没有和零值不能混】：
// PHash=nil 与 PHash=&PHash{} 是两回事，TimeSec=nil 与 TimeSec=0 也是。
func TestNullableFieldsAreNil(t *testing.T) {
	c := loadFixture(t, supportedSchema)

	s, ok := c.Song("sparse")
	if !ok {
		t.Fatal("夹具里应该有 sparse")
	}

	if s.TimeSec != nil {
		t.Errorf("TimeSec 应为 nil，实际 %v", *s.TimeSec)
	}
	if s.PHash != nil {
		t.Errorf("PHash 应为 nil，实际 %+v", *s.PHash)
	}
	// jacket 本身存在，但内容是空的 —— 两种"空"要分清
	if s.Jacket == nil {
		t.Fatal("Jacket 不该是 nil（产物里这个键一直在）")
	}
	if s.Jacket.Bundled {
		t.Error("Bundled 应为 false")
	}
	// ⚠️ local_256 / native 是【整个键不存在】，full_url 是【显式 null】。
	// Go 里都解成 nil，但产物里两种都会出现，代码不能假设只有一种。
	if s.Jacket.Local256 != nil {
		t.Errorf("Local256 应为 nil（键不存在），实际 %v", *s.Jacket.Local256)
	}
	if s.Jacket.Native != nil {
		t.Errorf("Native 应为 nil（键不存在），实际 %v", *s.Jacket.Native)
	}
	if s.Jacket.FullURL != nil {
		t.Errorf("FullURL 应为 nil（显式 null），实际 %v", *s.Jacket.FullURL)
	}

	ch, ok := c.Chart("sparse", "PST")
	if !ok {
		t.Fatal("夹具里应该有 sparse/PST")
	}
	if ch.Level != nil {
		t.Errorf("Level 应为 nil，实际 %v", *ch.Level)
	}
	if ch.Constant != nil {
		t.Errorf("Constant 应为 nil，实际 %v", *ch.Constant)
	}
	if ch.ConstantSource != nil {
		t.Errorf("ConstantSource 应为 nil，实际 %v", *ch.ConstantSource)
	}
	if ch.Video != nil {
		t.Error("Video 应为 nil（这个难度没录像）")
	}
	if len(sources(ch.Video)) != 0 {
		t.Error("Video 为 nil 时 sources() 应安全返回空")
	}

	// mixed 的 phash 只有 ahash，没有 dhash —— 截断图场景会这样
	mixed, _ := c.Song("mixed")
	if mixed.PHash == nil {
		t.Fatal("mixed 应有 phash")
	}
	if mixed.PHash.AHash == nil {
		t.Error("mixed 的 ahash 不该是 nil")
	}
	if mixed.PHash.DHash != nil {
		t.Errorf("mixed 的 dhash 应为 nil，实际 %v", *mixed.PHash.DHash)
	}
}

// ── 最重要的那条：遍历全部，一个字段都不放过 ────────────────────────────────
//
// 它一次性覆盖所有 nullable 字段和所有指针解引用。
// 只要哪里少判一个 nil，这里就会 panic（而不是等线上某个用户触发）。
//
// 以前只跑数据/夹具，现在把真实目录也一起走一遍。
func TestVisitAllNoPanic(t *testing.T) {
	paths := []string{fixtureRel}
	if _, err := os.Stat(realCatalogPath()); err == nil {
		paths = append(paths, realCatalogPath())
	}

	for _, p := range paths {
		c, err := Load(p, supportedSchema)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		visitAll(t, c, p)
	}
}

// visitAll 读一遍目录里【每一个】可空字段，不做任何断言之外的判断。
// 这里故意不做 errcheck 式的"优雅处理"——目标是解引用，好让漏判的 nil 立刻炸出来。
func visitAll(t *testing.T, c *Catalog, label string) {
	t.Helper()

	_ = c.SchemaVersion
	_ = c.Game
	_ = c.GeneratedAt

	charts := 0
	for i, s := range c.Songs {
		if s == nil {
			t.Errorf("%s: songs[%d] 是 nil", label, i)
			continue
		}

		// Song 层：值字段
		_ = s.SongID
		_ = s.TitleEn
		_ = s.TitleJP
		_ = s.TitleNorm
		_ = s.Artist
		_ = s.Set
		_ = s.SetFriendly
		_ = s.Version
		_ = s.BG

		// Song 层：可空字段
		if s.TimeSec != nil {
			_ = *s.TimeSec
		}
		if s.Jacket != nil {
			_ = s.Jacket.Bundled
			if s.Jacket.Local256 != nil {
				_ = *s.Jacket.Local256
			}
			if s.Jacket.Native != nil {
				_ = *s.Jacket.Native
			}
			if s.Jacket.ThumbURL256 != nil {
				_ = *s.Jacket.ThumbURL256
			}
			if s.Jacket.FullURL != nil {
				_ = *s.Jacket.FullURL
			}
		}
		if s.PHash != nil {
			if s.PHash.AHash != nil {
				_ = *s.PHash.AHash
			}
			if s.PHash.DHash != nil {
				_ = *s.PHash.DHash
			}
		}

		for j, ch := range s.Charts {
			if ch == nil {
				t.Errorf("%s: %s.charts[%d] 是 nil", label, s.SongID, j)
				continue
			}
			charts++

			_ = ch.Difficulty
			if ch.Level != nil {
				_ = *ch.Level
			}
			if ch.Constant != nil {
				_ = *ch.Constant
			}
			if ch.ConstantSource != nil {
				_ = *ch.ConstantSource
			}

			if ch.Video == nil {
				continue
			}
			for k, src := range sources(ch.Video) {
				if src.BVID != nil {
					_ = *src.BVID
				}
				_ = src.P
				_ = src.URL
				_ = src.DurationSec
				_ = src.PartTitle
				_ = src.OwnerName
				for _, col := range src.Collections {
					_ = col
				}
				if src.VideoWidth != nil {
					_ = *src.VideoWidth
				}
				if src.VideoHeight != nil {
					_ = *src.VideoHeight
				}
				// 空 BVID / 空 URL 是"数据有问题"，不是"加载失败"：
				// 加载层已经放行，这里只是提醒。真正要拦的是渲染前的素材校验。
				if src.BVID == nil || *src.BVID == "" {
					t.Logf("%s: %s/%s sources[%d] 没有 bvid", label, s.SongID, ch.Difficulty, k)
				}
				if src.URL == "" {
					t.Logf("%s: %s/%s sources[%d] 没有 url", label, s.SongID, ch.Difficulty, k)
				}
			}
		}
	}
	t.Logf("%s: %d 首 / %d 条谱面，全部字段解引用无 panic", label, len(c.Songs), charts)
}
