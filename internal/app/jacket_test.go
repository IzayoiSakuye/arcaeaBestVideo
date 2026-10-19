package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
)

func TestJacketReadsOnlyLocalArt(t *testing.T) {
	cata, err := catalog.Load(filepath.Join("..", "catalog", "testdata", "catalog-mini.json"), 1)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := filepath.Join(root, "data", "jackets", "256", "complete.jpg")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{root: root, cata: cata}
	a.cataOnce.Do(func() {})
	got, err := a.Jacket("complete", "FTR")
	if err != nil || got != "data:image/jpeg;base64,aW1hZ2U=" {
		t.Fatalf("曲绘 = %q, %v", got, err)
	}
	escape := "data/jackets/../../outside.jpg"
	song, _ := cata.Song("complete")
	song.Jacket.Local256 = &escape
	if _, err := a.Jacket("complete", "FTR"); err == nil || !strings.Contains(err.Error(), "不在本地曲绘目录") {
		t.Fatalf("未拦截越界路径：%v", err)
	}
}
