package app

import (
	"path/filepath"
	"testing"
)

func TestStartRenderRejectsDataDirectory(t *testing.T) {
	a := testApp(t)
	_, err := a.StartRender(testProject(), filepath.Join(a.root, "data", "video.mp4"), false)
	if err == nil {
		t.Fatal("不能把运行时产物写入 data 目录")
	}
}
