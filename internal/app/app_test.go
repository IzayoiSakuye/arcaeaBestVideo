package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindRootFromBuildDirectory(t *testing.T) {
	root := t.TempDir()
	for _, file := range []string{"data/arcaea-catalog.json", "templates/text.html"} {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	bin := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findRoot(bin); got != root {
		t.Fatalf("根目录 = %q，期望 %q", got, root)
	}
}
