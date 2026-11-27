package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

func TestQA416HeadersAndSuccessStreams(t *testing.T) {
	a := &App{}
	video, audio := a.remotePreviewURLs("https://www.bilibili.com/video/BV1op4y1W7M2", "", resource.PreviewStreams{
		VideoURL: "https://qa.bilivideo.com/video", AudioURL: "https://qa.bilivideo.com/audio",
		VideoHeaders: http.Header{}, AudioHeaders: http.Header{},
	})
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, header := range []string{"bytes */0", "bytes */12345", "bytes */18446744073709551615", "", "bytes */", "bytes */-1", "bytes */18446744073709551616", "bytes 0-9/10", "items */10", "bytes */10 signature=secret", "bytes */10, bytes */20", "bytes */10\nsecret"} {
		for _, method := range []string{"GET", "HEAD"} {
			for index, url := range []string{video, audio} {
				t.Run(fmt.Sprintf("416/%q/%s/%d", header, method, index), func(t *testing.T) {
					http.DefaultTransport = qaRoundTripper(func(*http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: 416, Header: http.Header{"Content-Range": {header}, "Content-Length": {"25"}, "X-Diagnostic": {"secret"}}, Body: io.NopCloser(strings.NewReader("signature=secret"))}, nil
					})
					w := httptest.NewRecorder()
					NewPreviewHandler(a).ServeHTTP(w, httptest.NewRequest(method, url, nil))
					want := ""
					if header == "bytes */0" || header == "bytes */12345" || header == "bytes */18446744073709551615" {
						want = header
					}
					if w.Code != 416 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != "0" || w.Header().Get("Content-Range") != want || w.Header().Get("X-Diagnostic") != "" {
						t.Fatalf("416 隔离或长度错误：%d %v %q", w.Code, w.Header(), w.Body.String())
					}
				})
			}
		}
	}
	for _, status := range []int{200, 206} {
		for _, method := range []string{"GET", "HEAD"} {
			for index, url := range []string{video, audio} {
				t.Run(fmt.Sprintf("%d/%s/%d", status, method, index), func(t *testing.T) {
					http.DefaultTransport = qaRoundTripper(func(*http.Request) (*http.Response, error) {
						return &http.Response{StatusCode: status, Header: http.Header{"Content-Length": {"3"}, "Content-Range": {"bytes 2-4/10"}}, Body: io.NopCloser(strings.NewReader("234"))}, nil
					})
					w := httptest.NewRecorder()
					NewPreviewHandler(a).ServeHTTP(w, httptest.NewRequest(method, url, nil))
					want := "234"
					if method == "HEAD" {
						want = ""
					}
					if w.Code != status || w.Body.String() != want || w.Header().Get("Content-Length") != "3" || w.Header().Get("Content-Range") != "bytes 2-4/10" {
						t.Fatalf("成功响应回归：%d %v %q", w.Code, w.Header(), w.Body.String())
					}
				})
			}
		}
	}
}

func TestQACacheInvalidationRecoveryMatrix(t *testing.T) {
	for _, source := range []string{"youtube", "bilibili", "local"} {
		for _, damage := range []string{"missing", "empty", "directory"} {
			for _, entry := range []string{"query", "prepare", "retry"} {
				t.Run(source+"/"+damage+"/"+entry, func(t *testing.T) {
					a, song, probe := qaVideoApp(t)
					fixture, err := os.ReadFile(song.Video.LocalPath)
					if err != nil {
						t.Fatal(err)
					}
					qaSidecar(t, a, probe)
					t.Setenv("QA_FIXTURE", song.Video.LocalPath)
					if source == "youtube" {
						song.Video = &score.VideoRef{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}
					}
					if source == "bilibili" {
						song.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}
					}
					if _, err := a.PrepareVideo(song); err != nil {
						t.Fatal(err)
					}
					qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
					selection, err := a.ConfirmVideoRange(song, score.ClipRange{StartMs: 500, DurationMs: 1000})
					if err != nil {
						t.Fatal(err)
					}
					qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
					song = selection.Song
					query := func() VideoState {
						states, err := a.VideoInfos(&score.Project{Songs: []score.Song{song}})
						if err != nil {
							t.Fatal(err)
						}
						return states[0]
					}
					before := query()
					if !before.MaterialReady || !before.Confirmed {
						t.Fatalf("测试前置素材未就绪：%+v", before)
					}
					reused, err := a.PrepareVideo(song)
					if err != nil || reused.RequestID != before.RequestID || !reused.MaterialReady {
						t.Fatalf("有效缓存未复用：%+v %v", reused, err)
					}
					target := a.videoMaterialPath(song)
					originalInfo, err := os.Stat(target)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(target); err != nil {
						t.Fatal(err)
					}
					if damage == "empty" {
						if err := os.WriteFile(target, nil, 0600); err != nil {
							t.Fatal(err)
						}
					}
					if damage == "directory" {
						if err := os.Mkdir(target, 0700); err != nil {
							t.Fatal(err)
						}
					}
					var observed VideoState
					switch entry {
					case "query":
						observed = query()
					case "prepare":
						observed, err = a.PrepareVideo(song)
					case "retry":
						observed, err = a.RetryVideoPreparation(song.SongID, "FTR", before.RequestID)
					}
					t.Logf("失效入口：status=%s material=%v preview=%v confirmed=%v error=%v", observed.Status, observed.MaterialReady, observed.PreviewReady, observed.Confirmed, err)
					if observed.MaterialReady {
						t.Errorf("失效素材仍 ready：%+v", observed)
					}
					if source != "bilibili" && (observed.PreviewReady || observed.PreviewURL != "" || observed.PreviewAudioURL != "") {
						t.Errorf("失效本地预览未撤销：%+v", observed)
					}
					if entry == "query" && (observed.Status != "failed" || observed.RequestID != before.RequestID || !observed.Confirmed || observed.Progress != -1) {
						t.Errorf("查询须保留确认和原请求并转失败：%+v", observed)
					}
					if song.Clip.ConfirmedKey != selection.Song.Clip.ConfirmedKey || song.Clip.StartMs != 500 || song.Clip.DurationMs != 1000 {
						t.Fatal("Project 已保存范围丢失")
					}
					qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
					// 目录替代须先由用户纠正，本测试只恢复专用临时目录。
					if damage == "directory" {
						if err := os.Remove(target); err != nil {
							t.Fatal(err)
						}
					}
					if source == "local" {
						if err := os.WriteFile(target, fixture, 0600); err != nil {
							t.Fatal(err)
						}
						if err := os.Chtimes(target, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
							t.Fatal(err)
						}
					}
					current := query()
					if current.Status == "failed" || current.Status == "cancelled" {
						next, retryErr := a.RetryVideoPreparation(song.SongID, "FTR", current.RequestID)
						if retryErr != nil {
							t.Fatalf("恢复后重试失败：%v", retryErr)
						}
						if next.RequestID == current.RequestID {
							t.Error("重试未更换 request_id")
						}
					}
					qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
					after := query()
					if after.RequestID == before.RequestID || !after.PreviewReady || !after.MaterialReady || !after.Confirmed {
						t.Errorf("恢复后应保持保存范围并就绪：%+v", after)
					}
					if got := a.failVideo(videoSourceKey(song.SongID, "FTR"), before.RequestID, fmt.Errorf("旧请求失败")); got.RequestID != "" {
						t.Error("旧失败覆盖新任务")
					}
					if err := a.CancelVideoPreparation(song.SongID, "FTR", before.RequestID); err == nil {
						t.Error("旧请求可取消新任务")
					}
					if final := query(); final.RequestID != after.RequestID || final.Status != after.Status {
						t.Error("旧请求改变恢复状态")
					}
					callsBefore, _ := os.ReadFile(os.Getenv("QA_SIDECAR_LOG"))
					_, _, materialErr := resource.VideoMaterial(context.Background(), probe, filepath.Join(a.cacheDir(), "clips"), song)
					callsAfter, _ := os.ReadFile(os.Getenv("QA_SIDECAR_LOG"))
					if materialErr != nil {
						t.Errorf("恢复后生成取材失败：%v", materialErr)
					}
					if string(callsBefore) != string(callsAfter) {
						t.Error("生成取材偷偷联网补下载")
					}
				})
			}
		}
	}
}

func TestQAClearDuringPreparationStart(t *testing.T) {
	a, song, probe := qaVideoApp(t)
	qaSidecar(t, a, probe)
	gate := filepath.Join(t.TempDir(), "resolve")
	t.Setenv("QA_RESOLVE_GATE", gate)
	song.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}
	done := make(chan error, 1)
	go func() { _, err := a.PrepareVideo(song); done <- err }()
	// 无论断言是否通过，都先收回准备任务再退出测试。
	defer func() {
		_ = os.WriteFile(gate, nil, 0600)
		if err := <-done; err != nil {
			t.Errorf("清理竞争后准备失败：%v", err)
		}
	}()
	qaWait(t, func() bool { _, err := os.Stat(gate + ".started"); return err == nil })
	if err := a.ClearCache(true); err == nil {
		t.Fatal("准备入口运行中允许清理")
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
}
