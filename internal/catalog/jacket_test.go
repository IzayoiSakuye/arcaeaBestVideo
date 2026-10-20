package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadRealForJacket 复用 catalog_test.go 的真实目录路径约定：
// go test 的工作目录是包目录，所以真实数据要往上跳两级。
func loadRealForJacket(t *testing.T) *Catalog {
	t.Helper()
	path := realCatalogPath()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("没有真实目录（%s）—— 未跑过数据管线", path)
	}
	c, err := Load(path, supportedSchema)
	if err != nil {
		t.Fatalf("加载失败: %v", err)
	}
	return c
}

// ⭐ 难度专属曲绘优先：tempestissimo 的 BYD 有自己的图（实测存在）
func TestJacketPathPrefersChartOverride(t *testing.T) {
	c := loadRealForJacket(t)
	defaultArt, err := c.JacketPath("tempestissimo", "")
	if err != nil || !strings.Contains(defaultArt, "tempestissimo.jpg") {
		t.Fatalf("未确定难度时应显示歌曲默认曲绘：%q, %v", defaultArt, err)
	}

	got, err := c.JacketPath("tempestissimo", "BYD")
	if err != nil {
		t.Fatalf("JacketPath(tempestissimo, BYD): %v", err)
	}
	if !strings.Contains(got, "tempestissimo_byd") {
		t.Errorf("BYD 应优先用难度专属曲绘（tempestissimo_byd.jpg），实际 %q", got)
	}

	// 没有专属图的难度要回落到歌曲默认
	ftr, err := c.JacketPath("tempestissimo", "FTR")
	if err != nil {
		t.Fatalf("JacketPath(tempestissimo, FTR): %v", err)
	}
	if strings.Contains(ftr, "_byd") {
		t.Errorf("FTR 不该拿到 BYD 的图：%q", ftr)
	}
	if !strings.Contains(ftr, "tempestissimo") {
		t.Errorf("FTR 应回落到歌曲默认曲绘，实际 %q", ftr)
	}
	if ftr == got {
		t.Errorf("FTR 与 BYD 拿到同一张图（%q）—— 说明难度参数没生效", ftr)
	}
}

// 按难度取到的路径，磁盘上必须真的存在
func TestJacketPathExistsOnDisk(t *testing.T) {
	c := loadRealForJacket(t)
	got, err := c.JacketPath("tempestissimo", "BYD")
	if err != nil {
		t.Fatal(err)
	}
	full := filepath.Join("..", "..", filepath.FromSlash(got))
	if _, err := os.Stat(full); err != nil {
		t.Errorf("曲绘路径在磁盘上不存在：%s（catalog 里存的是相对数据包根目录的路径）", got)
	}
}

func TestJacketPathErrors(t *testing.T) {
	c := loadRealForJacket(t)
	if _, err := c.JacketPath("", "BYD"); err == nil {
		t.Error("空 songID 应报错")
	}
	if _, err := c.JacketPath("绝对不存在的曲目", "BYD"); err == nil {
		t.Error("不存在的曲目应报错")
	}
}
