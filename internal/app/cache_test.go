package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearCachePreservesVideosUnlessRequested(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"pipeline", "thought", "clips"} {
		path := filepath.Join(root, ".cache", name, "sample")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{root: root}
	if err := a.ClearCache(false); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pipeline", "thought"} {
		if _, err := os.Stat(filepath.Join(root, ".cache", name)); !os.IsNotExist(err) {
			t.Fatalf("%s 缓存应已删除：%v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".cache", "clips", "sample")); err != nil {
		t.Fatalf("录像缓存不应删除：%v", err)
	}
	if err := a.ClearCache(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cache", "clips")); !os.IsNotExist(err) {
		t.Fatalf("录像缓存应已删除：%v", err)
	}
}
