package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestThoughtPreviewAssets(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	a := &App{root: root}
	image, err := a.ThoughtImage(filepath.Join(root, "templates", "thought-bg.png"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewPreviewHandler(a)
	for _, path := range []string{"/thought-template", "/thought-bg.png", image.PreviewURL} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("%s: HTTP %d，内容长度 %d", path, response.Code, response.Body.Len())
		}
	}
}

func TestThoughtAudioOptionsFindSuppliedFiles(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "templates", "audio", "op2.wav")); err != nil {
		t.Skip("尚未提供 OP 音频")
	}
	a := &App{root: root}
	options, err := a.ThoughtAudioOptions()
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 7 {
		t.Fatalf("OP 选项数：%d", len(options))
	}
	for i := 1; i < 7; i++ {
		if !options[i].Available || options[i].DurationMs <= 0 {
			t.Fatalf("%s 未识别：%+v", options[i].Kind, options[i])
		}
	}
}
