package resource

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
)

// 真实产物路径（data/ 不进版本库，克隆的仓库没有 —— 缺了就 Skip）
const realBgJSON = "../../data/arcaea-backgrounds.json"

func loadReal(t *testing.T) BgManifest {
	t.Helper()
	b, err := os.ReadFile(realBgJSON)
	if err != nil {
		t.Skipf("没有数据产物（%s）: %v", realBgJSON, err)
	}
	var m BgManifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("解析 %s 失败: %v", realBgJSON, err)
	}
	return m
}

// ⚠️ 这条一次抓三类问题：json tag 拼错 / 字段类型写错 / 顶层 key 写错。
// 它们全都是**静默失败** —— 不报错，只是字段恒为空值。
func TestBgManifestParsesRealFile(t *testing.T) {
	m := loadReal(t)

	if len(m.Songs) == 0 {
		t.Error("Songs 为空 —— 检查 json tag 与顶层 key")
	}
	if len(m.Bgs) == 0 {
		t.Error("Bgs 为空 —— 顶层 key 是 backgrounds（带 s），不是 background")
	}
	if m.OutputAspect == "" {
		t.Error("OutputAspect 为空")
	}
	t.Logf("Songs=%d Bgs=%d OutputAspect=%q", len(m.Songs), len(m.Bgs), m.OutputAspect)

	// 逐字段核对：判据是「**至少有一首**非空」。
	//
	// ⚠️ 不能只取第一首 —— Go 的 map 遍历顺序是随机的，而实测
	//    `bg_inverse` 有 90/554 首、`bg` 有 1/554 首**本来就是空串**
	//    （不是 tag 错，是数据就这样）。取第一首会时红时绿。
	countNonEmpty := func(get func(SongBg) string) int {
		n := 0
		for _, v := range m.Songs {
			if get(v) != "" {
				n++
			}
		}
		return n
	}
	for _, f := range []struct {
		name string
		get  func(SongBg) string
	}{
		{"Bg", func(s SongBg) string { return s.Bg }},
		{"BgInverse", func(s SongBg) string { return s.BgInverse }},
		{"OfficialID", func(s SongBg) string { return s.OfficialID }},
		{"MatchedVia", func(s SongBg) string { return s.MatchedVia }},
	} {
		n := countNonEmpty(f.get)
		if n == 0 {
			t.Errorf("SongBg.%s 全为空 —— json tag 写错了", f.name)
			continue
		}
		t.Logf("SongBg.%-11s 非空 %d/%d", f.name, n, len(m.Songs))
	}

	// BgEntry 同理：逐条查，只要存在一条完整的就算 tag 对
	okEntry := 0
	for _, be := range m.Bgs {
		if be.Name == "" || be.File == "" || be.Native == "" || len(be.Files) == 0 {
			continue
		}
		complete := true
		for _, f := range be.Files {
			if f.Path == "" {
				complete = false
			}
		}
		if complete {
			okEntry++
		}
	}
	if okEntry == 0 {
		t.Error("BgEntry 一条完整的都没有 —— json tag 写错了")
	}
	t.Logf("BgEntry 完整 %d/%d", okEntry, len(m.Bgs))
	// ⚠️ BgFile.Bytes 的真实类型是**数字**（实测 63111）。
	//    写成 string 的话上面那个 Unmarshal 就会直接报错 —— 不用再单独断言。
}

// BgPath 应该直接回传清单里现成的路径，而不是自己拼字符串
func TestBgPathReturnsManifestPath(t *testing.T) {
	m := loadReal(t)

	var songID string
	for id, sb := range m.Songs {
		if sb.Bg != "" {
			songID = id
			break
		}
	}
	if songID == "" {
		t.Skip("清单里没有可用曲目")
	}

	got, err := m.BgPath(songID, "960")
	if err != nil {
		t.Fatalf("BgPath(%q, \"960\"): %v", songID, err)
	}
	want := m.Bgs[m.Songs[songID].Bg].Files["960"].Path

	if got != want {
		t.Errorf("BgPath = %q\n  期望 %q（清单里 files[\"960\"].path 就是现成的）", got, want)
	}
	if !strings.HasSuffix(got, ".jpg") {
		t.Errorf("扩展名不对：%q（产物是 .jpg，不是 .png）", got)
	}
	if !strings.Contains(got, "/960/") {
		t.Errorf("路径里缺尺寸子目录：%q", got)
	}
	if _, err := os.Stat(filepath.Join("..", "..", filepath.FromSlash(got))); err != nil {
		t.Errorf("清单里的路径在磁盘上不存在: %v", err)
	}
}

func TestBgPathErrors(t *testing.T) {
	m := loadReal(t)
	if _, err := m.BgPath("", "960"); err == nil {
		t.Error("空 songID 应报错")
	}
	if _, err := m.BgPath("绝对不存在的曲目", "960"); err == nil {
		t.Error("清单里没有的曲目应报错")
	}
}

// ── ClipPath ─────────────────────────────────────────────────────────────────

func TestClipPathFindsMp4(t *testing.T) {
	dir := t.TempDir()
	// 命名约定：<song_id>.mp4
	if err := os.WriteFile(filepath.Join(dir, "testify.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ClipPath(dir, "testify", "BYD")
	if err != nil {
		t.Fatalf("应能找到 testify.mp4: %v", err)
	}
	if filepath.Base(got) != "testify.mp4" {
		t.Errorf("文件名 = %q，期望 testify.mp4", filepath.Base(got))
	}
}

func TestClipPathMissingGivesActionableError(t *testing.T) {
	dir := t.TempDir()
	_, err := ClipPath(dir, "testify", "BYD")
	if err == nil {
		t.Fatal("缺失时应报错，而不是返回空路径")
	}
	msg := err.Error()
	// 报错要能回答「我该做什么」
	if !strings.Contains(msg, "testify") {
		t.Errorf("错误信息里没有曲目名: %v", err)
	}
	if !strings.Contains(msg, "BYD") {
		t.Errorf("错误信息里没有难度: %v", err)
	}
	if !strings.Contains(msg, ".mp4") {
		t.Errorf("错误信息里没告诉用户文件该叫什么: %v", err)
	}
}

// ── TemplateFor ──────────────────────────────────────────────────────────────

func TestTemplateFor(t *testing.T) {
	i := func(v int) *int { return &v }
	cases := []struct {
		name string
		src  *catalog.VideoSource
		want bool
	}{
		{"1920x1080 = 16:9", &catalog.VideoSource{VideoWidth: i(1920), VideoHeight: i(1080)}, true},
		{"1280x720 = 16:9", &catalog.VideoSource{VideoWidth: i(1280), VideoHeight: i(720)}, true},
		{"1920x1440 = 4:3", &catalog.VideoSource{VideoWidth: i(1920), VideoHeight: i(1440)}, false},
		{"缺画幅字段", &catalog.VideoSource{}, false},
		{"nil 源", nil, false},
	}
	for _, c := range cases {
		if got := TemplateFor(c.src); got != c.want {
			t.Errorf("%s: TemplateFor = %v, 期望 %v", c.name, got, c.want)
		}
	}
}

// ── isFile ───────────────────────────────────────────────────────────────────

func TestIsFile(t *testing.T) {
	if isFile("") {
		t.Error(`isFile("") 应为 false`)
	}
	if isFile(t.TempDir()) {
		t.Error("目录应为 false")
	}
	if isFile(filepath.Join(t.TempDir(), "nope.mp4")) {
		t.Error("不存在的路径应为 false")
	}
}
