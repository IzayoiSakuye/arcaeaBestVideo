package resource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真实产物能加载（data/ 不进版本库，克隆的仓库没有 —— 缺了就 Skip）
func TestLoadBgManifestReal(t *testing.T) {
	if _, err := os.Stat(realBgJSON); err != nil {
		t.Skipf("没有数据产物（%s）: %v", realBgJSON, err)
	}
	m, err := LoadBgManifest(realBgJSON)
	if err != nil {
		t.Fatalf("LoadBgManifest(%s): %v", realBgJSON, err)
	}
	if len(m.Songs) == 0 {
		t.Error("Songs 为空")
	}
	if len(m.Bgs) == 0 {
		t.Error("Bgs 为空 —— 顶层 key 是 backgrounds（带 s）")
	}
	if m.OutputAspect == "" {
		t.Error("OutputAspect 为空")
	}
	t.Logf("Songs=%d Bgs=%d OutputAspect=%q", len(m.Songs), len(m.Bgs), m.OutputAspect)

	// 加载出来的 manifest 要能直接用于 BgPath（端到端串一下）
	for id, sb := range m.Songs {
		if sb.Bg == "" {
			continue
		}
		if _, err := m.BgPath(id, "960"); err != nil {
			t.Errorf("加载后 BgPath(%q, \"960\") 失败: %v", id, err)
		}
		break
	}
}

func TestLoadBgManifestErrors(t *testing.T) {
	t.Run("空路径", func(t *testing.T) {
		if _, err := LoadBgManifest(""); err == nil {
			t.Error("空路径应报错")
		}
	})

	t.Run("文件不存在", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "不存在.json")
		_, err := LoadBgManifest(p)
		if err == nil {
			t.Fatal("应报错")
		}
		// 错误信息要带路径，否则用户不知道去哪找
		if !strings.Contains(err.Error(), p) {
			t.Errorf("错误信息里没有路径: %v", err)
		}
	})

	t.Run("JSON 非法", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "broken.json")
		if err := os.WriteFile(p, []byte(`{"songs": [ 这不是 json`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadBgManifest(p)
		if err == nil {
			t.Fatal("应报错")
		}
		// ⚠️ 别把真正的解析错误吞掉换成别的 err（这种坑本项目踩过）
		if strings.Contains(err.Error(), "%!") {
			t.Errorf("解析错误被丢弃了: %v", err)
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("错误信息里没有路径: %v", err)
		}
	})

	// 「文件存在但内容不对」的唯一防线
	t.Run("songs 为空", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "empty.json")
		if err := os.WriteFile(p, []byte(`{"output_aspect":"16:9","songs":{},"backgrounds":{}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadBgManifest(p)
		if err == nil {
			t.Fatal("songs 为空应报错 —— 否则后面 BgPath 会一路返回空路径")
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("错误信息里没有路径: %v", err)
		}
	})
}

// tag 写错时字段会静默变空 —— 这条用一份手写的最小 JSON 钉住键名
func TestLoadBgManifestFieldNames(t *testing.T) {
	p := filepath.Join(t.TempDir(), "mini.json")
	body := `{
      "output_aspect": "16:9",
      "songs": {
        "s1": {"bg":"b1","bg_inverse":"b1i","official_id":"s1","matched_via":"id"}
      },
      "backgrounds": {
        "b1": {"name":"b1","file":"Background_b1.jpg","native":"1920x1440",
               "files":{"960":{"path":"data/backgrounds/960/Background_b1.jpg","bytes":123}}}
      }
    }`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadBgManifest(p)
	if err != nil {
		t.Fatalf("加载最小清单失败: %v", err)
	}

	sb := m.Songs["s1"]
	if sb.Bg != "b1" || sb.BgInverse != "b1i" || sb.OfficialID != "s1" || sb.MatchedVia != "id" {
		t.Errorf("SongBg 字段没解析对: %+v", sb)
	}
	be := m.Bgs["b1"]
	if be.Name != "b1" || be.File != "Background_b1.jpg" || be.Native != "1920x1440" {
		t.Errorf("BgEntry 字段没解析对: %+v", be)
	}
	if got := be.Files["960"].Path; got != "data/backgrounds/960/Background_b1.jpg" {
		t.Errorf("BgFile.Path = %q", got)
	}
	if got := be.Files["960"].Bytes; got != 123 {
		t.Errorf("BgFile.Bytes = %v（类型应是数字，不是 string）", got)
	}

	// 加载之后 BgPath 要能直接用
	got, err := m.BgPath("s1", "960")
	if err != nil {
		t.Fatalf("BgPath: %v", err)
	}
	if got != "data/backgrounds/960/Background_b1.jpg" {
		t.Errorf("BgPath = %q", got)
	}
}
