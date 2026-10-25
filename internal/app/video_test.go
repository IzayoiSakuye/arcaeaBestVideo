package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

func TestCancelVideoDownload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{videoCancel: cancel}
	a.CancelVideoDownload()
	if ctx.Err() != context.Canceled {
		t.Fatal("应停止当前录像获取")
	}
}

func TestManualVideoSourceSurvivesReview(t *testing.T) {
	a := testApp(t)
	const link = "https://www.bilibili.com/video/BV1op4y1W7M2?p=5"
	if got, err := a.ApplyVideoLink("complete", "FTR", link+"&spm=abc"); err != nil || got != link {
		t.Fatalf("保存链接失败：%q, %v", got, err)
	}
	if _, err := a.ApplyVideoLink("complete", "BYD", link); err == nil {
		t.Fatal("不应保存不存在的难度")
	}
	next := &App{root: a.root}
	review, err := next.Review(testProject())
	if err != nil || review.Project.Songs[0].Video == nil || review.Project.Songs[0].Video.URL != link {
		t.Fatalf("重新导入后未恢复链接：%+v, %v", review, err)
	}
	explicit := testProject()
	explicit.Songs[0].Video = &score.VideoRef{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}
	review, err = next.Review(explicit)
	if err != nil || review.Project.Songs[0].Video.URL != explicit.Songs[0].Video.URL {
		t.Fatalf("导入内容应优先于来源记录：%+v, %v", review, err)
	}
	if _, err := next.ApplyVideoLink("complete", "FTR", ""); err != nil {
		t.Fatal(err)
	}
	review, err = next.Review(testProject())
	if err != nil || review.Project.Songs[0].Video != nil {
		t.Fatalf("清除后不应恢复旧来源：%+v, %v", review, err)
	}
	path := filepath.Join(t.TempDir(), "local.mp4")
	if err := next.saveVideoSource("complete", "FTR", &score.VideoRef{LocalPath: path}); err != nil {
		t.Fatal(err)
	}
	review, err = a.Review(testProject())
	if err != nil || review.Project.Songs[0].Video == nil || review.Project.Songs[0].Video.LocalPath != path {
		t.Fatalf("重新导入后未恢复本地文件路径：%+v, %v", review, err)
	}
}

func TestPreviewHandlerSupportsRangeAndRejectsUnknownPath(t *testing.T) {
	a := &App{}
	file := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(file, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	preview := a.previewURL(file)
	request := httptest.NewRequest(http.MethodGet, preview, nil)
	request.Header.Set("Range", "bytes=2-4")
	response := httptest.NewRecorder()
	NewPreviewHandler(a).ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "234" {
		t.Fatalf("Range 预览失败：%d %q", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	NewPreviewHandler(a).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/preview/00000000000000000000000000000000.mp4", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("未知路径应被拒绝：%d", response.Code)
	}
}

func TestVideoInfoFollowsSelectedSource(t *testing.T) {
	root := t.TempDir()
	const indexed = "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"
	const changed = "https://www.bilibili.com/video/BV1op4y1W7M2?p=2"
	cata := &catalog.Catalog{Songs: []*catalog.Song{{SongID: "song", Charts: []*catalog.Chart{{Difficulty: "FTR", Video: &catalog.Video{Sources: []catalog.VideoSource{{URL: indexed, OwnerName: "UP"}}}}}}}}
	a := &App{root: root, cata: cata}
	a.cataOnce.Do(func() {})
	cache := resource.VideoCachePath(filepath.Join(root, ".cache", "clips"), "song", "FTR", indexed)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	song := score.Song{SongID: "song", Difficulty: score.DiffRef{Name: "FTR"}}
	if got := a.videoInfo(cata, song); got.Status != "ready" || got.URL != indexed || got.PreviewURL == "" {
		t.Fatalf("索引缓存应可预览：%+v", got)
	}
	song.Video = &score.VideoRef{URL: changed}
	if got := a.videoInfo(cata, song); got.Status != "download" || got.PreviewURL != "" {
		t.Fatalf("换链接后旧缓存不应继续就绪：%+v", got)
	}
	song.Video = &score.VideoRef{URL: "https://example.com/video/BV1op4y1W7M2"}
	if got := a.videoInfo(cata, song); got.Status != "invalid" {
		t.Fatalf("无效链接应阻止生成：%+v", got)
	}
	song.Video = &score.VideoRef{LocalPath: cache}
	if got := a.videoInfo(cata, song); got.Status != "ready" || got.URL != "" {
		t.Fatalf("本地文件应可预览：%+v", got)
	}
}
