package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"
)

func qaVideoApp(t *testing.T) (*App, score.Song, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("模拟 sidecar 使用 POSIX shell；Windows 需另行运行验收")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 ffmpeg")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("需要 ffprobe")
	}
	a := testApp(t)
	a.cacheRoot = t.TempDir()
	fixture := filepath.Join(t.TempDir(), "source.mp4")
	if out, err := exec.Command(ffmpeg, "-y", "-v", "error", "-f", "lavfi", "-i", "color=white:s=64x64:r=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "3", "-c:v", "libx264", "-c:a", "aac", fixture).CombinedOutput(); err != nil {
		t.Fatalf("素材生成失败：%v %s", err, out)
	}
	if err := saveToolPaths(tools.Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.CancelVideoDownload()
		qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
	})
	song := score.Song{Index: 1, SongID: "complete", Difficulty: score.DiffRef{Name: "FTR"}, Video: &score.VideoRef{LocalPath: fixture, OwnerName: "QA"}}
	return a, song, ffprobe
}

func TestQAConfirmLocalRangeAndInvalidation(t *testing.T) {
	a, song, probe := qaVideoApp(t)
	prepared, err := a.PrepareVideo(song)
	if err != nil || !prepared.PreviewReady || prepared.Confirmed || prepared.MaterialReady {
		t.Fatalf("准备不能隐式保存：%+v %v", prepared, err)
	}
	if _, _, err := resource.VideoMaterial(context.Background(), probe, a.cacheDir(), song); err == nil {
		t.Fatal("未确认时不得生成")
	}
	for _, clip := range []score.ClipRange{{StartMs: -1, DurationMs: 1000}, {DurationMs: 999}, {StartMs: 2500, DurationMs: 1000}} {
		if _, err := a.ConfirmVideoRange(song, clip); err == nil {
			t.Errorf("无效范围被接受：%+v", clip)
		}
	}
	sel, err := a.ConfirmVideoRange(song, score.ClipRange{StartMs: 500, DurationMs: 1000})
	if err != nil || !sel.State.Confirmed || !sel.State.MaterialReady || !score.ClipConfirmed(sel.Song) {
		t.Fatalf("显式确认失败：%+v %v", sel, err)
	}
	path, offset, err := resource.VideoMaterial(context.Background(), probe, a.cacheDir(), sel.Song)
	if err != nil || path != song.Video.LocalPath || offset != 500 {
		t.Fatalf("本地素材应保留原始偏移：%q %d %v", path, offset, err)
	}
	again, err := a.ConfirmVideoRange(sel.Song, *sel.Song.Clip)
	if err != nil || again.State.RequestID != sel.State.RequestID {
		t.Fatalf("重复保存不应创建新任务：%+v %v", again.State, err)
	}
	changed := sel.Song
	clip := *changed.Clip
	clip.StartMs++
	changed.Clip = &clip
	states, err := a.VideoInfos(&score.Project{Songs: []score.Song{changed}})
	if err != nil || states[0].Confirmed || states[0].MaterialReady {
		t.Fatalf("修改范围须使确认失效：%+v %v", states, err)
	}
	if _, _, err := resource.VideoMaterial(context.Background(), probe, a.cacheDir(), changed); err == nil {
		t.Fatal("修改范围后不应复用已确认素材")
	}
	if err := os.Remove(sel.Song.Video.LocalPath); err != nil {
		t.Fatal(err)
	}
	states, err = a.VideoInfos(&score.Project{Songs: []score.Song{sel.Song}})
	if err != nil {
		t.Fatal(err)
	}
	if states[0].MaterialReady {
		t.Errorf("已删除的本地素材仍被报告就绪：%+v", states[0])
	}
}

func TestQAStartRenderGuardsUnsavedMissingAndProtectedPaths(t *testing.T) {
	a, song, _ := qaVideoApp(t)
	paths, err := loadToolPaths()
	if err != nil {
		t.Fatal(err)
	}
	paths.Edge = paths.FFmpeg
	if err := saveToolPaths(paths); err != nil {
		t.Fatal(err)
	}
	project := testProject()
	project.Songs[0].Difficulty.Name = "FTR"
	project.Songs[0].Video = song.Video
	for _, output := range []string{song.Video.LocalPath, filepath.Join(a.cacheDir(), "result.mp4")} {
		if _, err := a.StartRender(project, output, true); err == nil || (!strings.Contains(err.Error(), "不能覆盖") && !strings.Contains(err.Error(), "可清理")) {
			t.Errorf("未拒绝受保护的输出：%v", err)
		}
	}
	output := filepath.Join(a.root, "result.mp4")
	if _, err := a.StartRender(project, output, false); err == nil || !strings.Contains(err.Error(), "片段尚未保存") {
		t.Fatalf("Wails 入口没有阻止未确认项目：%v", err)
	}
	project.Songs[0].Video = &score.VideoRef{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}
	project.Songs[0].Clip = &score.ClipRange{DurationMs: 1000}
	project.Songs[0].Clip.ConfirmedKey = score.ClipKey(project.Songs[0])
	if _, err := a.StartRender(project, output, false); err == nil || !strings.Contains(err.Error(), "录像未就绪") {
		t.Fatalf("Wails 入口没有阻止缺失素材：%v", err)
	}
	if a.jobID != "" {
		t.Fatal("无效项目启动了后台生成")
	}
}

func TestQACacheGuardsAndPreviewRelease(t *testing.T) {
	a, _, _ := qaVideoApp(t)
	file := filepath.Join(a.cacheDir(), "pipeline", "seg.mp4")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"render", "download", "reader"} {
		var release func()
		switch mode {
		case "render":
			a.jobID = "qa"
		case "download":
			a.videoWorker = true
		case "reader":
			release = a.beginCacheUse()
		}
		if err := a.ClearCache(true); err == nil {
			t.Errorf("%s 占用时不能清理", mode)
		}
		a.jobID, a.videoWorker = "", false
		if release != nil {
			release()
		}
	}
	preview := a.previewURL(file)
	rec := httptest.NewRecorder()
	NewPreviewHandler(a).ServeHTTP(rec, httptest.NewRequest("GET", preview, nil))
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if err := a.ClearCache(false); err == nil {
		t.Fatal("暂停预览期间应保留缓存")
	}
	state, err := a.CacheInfo()
	if err != nil || state.RenderBytes != 10 || state.PreviewCount != 1 {
		t.Fatalf("缓存统计不正确：%+v %v", state, err)
	}
	a.ReleasePreview(preview)
	if err := a.saveVideoSource("complete", "FTR", &score.VideoRef{LocalPath: file}); err != nil {
		t.Fatal(err)
	}
	if err := a.ClearCache(true); err == nil {
		t.Fatal("登记为本地来源的文件不应清理")
	}
	if err := a.saveVideoSource("complete", "FTR", nil); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(a.root, "original.mp4")
	if err := os.WriteFile(outside, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := a.ClearCache(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("释放后应能清理：%v", err)
	}
	if b, err := os.ReadFile(outside); err != nil || string(b) != "original" {
		t.Fatal("缓存外的原文件被修改")
	}
}

type qaRoundTripper func(*http.Request) (*http.Response, error)

func (f qaRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQARemoteProxyRangeHeadAndPrivacy(t *testing.T) {
	a := &App{}
	streams := resource.PreviewStreams{VideoURL: "https://qa.bilivideo.com/video?signature=secret", AudioURL: "https://qa.bilivideo.com/audio?signature=secret", VideoHeaders: http.Header{"User-Agent": {"qa"}, "Referer": {"https://www.bilibili.com/"}}, AudioHeaders: http.Header{"User-Agent": {"qa"}}}
	video, audio := a.remotePreviewURLs("https://www.bilibili.com/video/BV1op4y1W7M2", "", streams)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	var requests int
	http.DefaultTransport = qaRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Range") != "bytes=2-4" || r.Header.Get("If-Range") != "etag" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("转发了不应传递的请求头：%v", r.Header)
		}
		if strings.HasSuffix(r.URL.Path, "audio") && r.Method != "HEAD" {
			t.Error("应请求音频 HEAD")
		}
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": {"bytes 2-4/10"}, "Content-Length": {"3"}, "Set-Cookie": {"secret"}, "Location": {streams.VideoURL}}, Body: io.NopCloser(strings.NewReader("234"))}, nil
	})
	for _, tc := range []struct{ method, url, body, contentType string }{{"GET", video, "234", "video/mp4"}, {"HEAD", audio, "", "audio/mp4"}} {
		r := httptest.NewRequest(tc.method, tc.url, nil)
		r.Header.Set("Range", "bytes=2-4")
		r.Header.Set("If-Range", "etag")
		r.Header.Set("Cookie", "frontend-secret")
		r.Header.Set("Authorization", "frontend-secret")
		w := httptest.NewRecorder()
		NewPreviewHandler(a).ServeHTTP(w, r)
		if w.Code != 206 || w.Body.String() != tc.body || w.Header().Get("Content-Type") != tc.contentType || w.Header().Get("Content-Range") != "bytes 2-4/10" {
			t.Errorf("代理语义错误：%d %v %q", w.Code, w.Header(), w.Body.String())
		}
		if w.Header().Get("Set-Cookie") != "" || w.Header().Get("Location") != "" || bytes.Contains(w.Body.Bytes(), []byte("secret")) {
			t.Error("代理响应泄露敏感信息")
		}
	}
	for _, url := range []string{"/preview/00000000000000000000000000000000.mp4", video + "/../../etc/passwd"} {
		w := httptest.NewRecorder()
		NewPreviewHandler(a).ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != 404 {
			t.Errorf("未知来源未被拒绝：%d", w.Code)
		}
	}
	if requests != 2 {
		t.Fatalf("未知入口发起了上游请求：%d", requests)
	}
	for _, status := range []int{416, 500} {
		http.DefaultTransport = qaRoundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: http.Header{"Content-Range": {"bytes */10"}}, Body: io.NopCloser(strings.NewReader("upstream signature=secret"))}, nil
		})
		w := httptest.NewRecorder()
		NewPreviewHandler(a).ServeHTTP(w, httptest.NewRequest("GET", video, nil))
		want := status
		if status == 500 {
			want = 502
		}
		if w.Code != want {
			t.Errorf("上游 %d 得到 %d", status, w.Code)
		}
		if strings.Contains(w.Body.String(), "secret") {
			t.Errorf("上游 %d 原始错误正文泄露签名：%q", status, w.Body.String())
		}
	}
}

func qaSidecar(t *testing.T, a *App, probe string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
for arg in "$@"; do
  if [ "$arg" = "--dump-single-json" ]; then
    if [ -n "$QA_RESOLVE_GATE" ]; then
      case "$*" in *p=1*) touch "$QA_RESOLVE_GATE.started"; while [ ! -f "$QA_RESOLVE_GATE" ]; do sleep 0.01; done;; esac
    fi
    printf '%s\n' '{"duration":120,"uploader":"QA","requested_formats":[{"url":"https://qa.bilivideo.com/video","vcodec":"avc1.64001f","acodec":"none"},{"url":"https://qa.bilivideo.com/audio","vcodec":"none","acodec":"mp4a.40.2"}]}'
    exit 0
  fi
done
echo start >> "$QA_SIDECAR_LOG"
if [ -n "$QA_GATE" ]; then
  touch "$QA_GATE.started"
  while [ ! -f "$QA_GATE" ]; do sleep 0.01; done
fi
out=''
owner=''
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) shift; out="$1" ;;
    --print-to-file) shift; shift; owner="$1" ;;
    --download-sections) echo section >> "$QA_SIDECAR_LOG" ;;
  esac
  shift
done
printf 'PROG|10|NA\n'
cp "$QA_FIXTURE" "$(dirname "$out")/clip.mp4"
echo QA > "$owner"
`
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	paths, err := loadToolPaths()
	if err != nil {
		t.Fatal(err)
	}
	paths.YtDlp, paths.FFprobe = path, probe
	if err := saveToolPaths(paths); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QA_SIDECAR_LOG", filepath.Join(dir, "calls"))
	t.Setenv("QA_GATE", "")
	t.Setenv("QA_RESOLVE_GATE", "")
	return path
}

func TestQAConcurrentSourceSwitchAndDifficultyIsolation(t *testing.T) {
	a, local, probe := qaVideoApp(t)
	qaSidecar(t, a, probe)
	gate := filepath.Join(t.TempDir(), "release")
	t.Setenv("QA_RESOLVE_GATE", gate)
	song := local
	song.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}
	done := make(chan error, 1)
	go func() { _, err := a.PrepareVideo(song); done <- err }()
	qaWait(t, func() bool { _, err := os.Stat(gate + ".started"); return err == nil })
	if err := a.ClearCache(true); err == nil {
		t.Error("正在解析预览时不得清理")
	}
	nextSong := song
	nextSong.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=2"}
	next, err := a.PrepareVideo(nextSong)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Error("被替换的旧解析成功返回")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("旧解析没有结束")
	}
	states, err := a.VideoInfos(&score.Project{Songs: []score.Song{nextSong}})
	if err != nil || states[0].RequestID != next.RequestID || states[0].Error != "" {
		t.Fatalf("旧请求覆盖新选择：%+v %v", states, err)
	}
	var variants []score.Song
	for _, difficulty := range []string{"BYD", "ETR"} {
		s := nextSong
		s.SongID = "mixed"
		s.Difficulty.Name = difficulty
		variants = append(variants, s)
		if _, err := a.PrepareVideo(s); err != nil {
			t.Fatal(err)
		}
	}
	states, err = a.VideoInfos(&score.Project{Songs: variants})
	if err != nil || len(states) != 2 || states[0].RequestID == states[1].RequestID || states[0].Difficulty == states[1].Difficulty {
		t.Fatalf("同曲不同难度任务互相覆盖：%+v %v", states, err)
	}
}

func TestQARemotePreviewRefreshAndRedirectBoundary(t *testing.T) {
	a, _, probe := qaVideoApp(t)
	yt := qaSidecar(t, a, probe)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprint(expired), func(t *testing.T) {
			calls := 0
			http.DefaultTransport = qaRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				status := 206
				if !expired && calls == 1 {
					status = 403
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("media"))}, nil
			})
			p := &remotePreview{source: "https://www.bilibili.com/video/BV1op4y1W7M2", ytDlp: yt, resolved: time.Now(), streams: resource.PreviewStreams{VideoURL: "https://qa.bilivideo.com/old", VideoHeaders: http.Header{}}}
			if expired {
				p.resolved = time.Now().Add(-6 * time.Minute)
			}
			w := httptest.NewRecorder()
			serveRemotePreview(w, httptest.NewRequest("GET", "/preview/qa.mp4", nil), p, false)
			want := 2
			if expired {
				want = 1
			}
			if w.Code != 206 || calls != want || p.streams.VideoURL != "https://qa.bilivideo.com/video" {
				t.Fatalf("刷新失败：status=%d calls=%d", w.Code, calls)
			}
		})
	}
	calls := 0
	http.DefaultTransport = qaRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://evil.test/secret"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	p := &remotePreview{resolved: time.Now(), streams: resource.PreviewStreams{VideoURL: "https://qa.bilivideo.com/old", VideoHeaders: http.Header{}}}
	w := httptest.NewRecorder()
	serveRemotePreview(w, httptest.NewRequest("GET", "/preview/qa.mp4", nil), p, false)
	if w.Code != 502 || calls != 1 {
		t.Fatalf("非法重定向未被阻止：status=%d calls=%d", w.Code, calls)
	}
}

func qaWait(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !predicate() {
		if time.Now().After(deadline) {
			t.Fatal("等待任务超时")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestQAYouTubeFullDownloadReuseAndCancelRetry(t *testing.T) {
	a, local, probe := qaVideoApp(t)
	qaSidecar(t, a, probe)
	t.Setenv("QA_FIXTURE", local.Video.LocalPath)
	gate := filepath.Join(t.TempDir(), "release")
	t.Setenv("QA_GATE", gate)
	song := local
	song.Video = &score.VideoRef{URL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"}
	state, err := a.PrepareVideo(song)
	if err != nil || state.PreviewReady || state.Progress != -1 {
		t.Fatalf("YouTube 应排队准备完整录像：%+v %v", state, err)
	}
	qaWait(t, func() bool { _, err := os.Stat(gate + ".started"); return err == nil })
	if err := a.ClearCache(true); err == nil {
		t.Error("下载期间允许清理")
	}
	if err := a.CancelVideoPreparation(song.SongID, "FTR", state.RequestID); err != nil {
		t.Fatal(err)
	}
	qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	retried, err := a.RetryVideoPreparation(song.SongID, "FTR", state.RequestID)
	if err != nil || retried.RequestID == state.RequestID {
		t.Fatalf("重试应换任务编号：%+v %v", retried, err)
	}
	if err := a.CancelVideoPreparation(song.SongID, "FTR", state.RequestID); err == nil {
		t.Error("旧请求不得取消新任务")
	}
	qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
	states, err := a.VideoInfos(&score.Project{Songs: []score.Song{song}})
	if err != nil || !states[0].PreviewReady || states[0].MaterialReady {
		t.Fatalf("下载完成仍须确认范围：%+v %v", states, err)
	}
	sel, err := a.ConfirmVideoRange(song, score.ClipRange{StartMs: 500, DurationMs: 1000})
	if err != nil || !sel.State.MaterialReady {
		t.Fatalf("本地选段失败：%+v %v", sel, err)
	}
	for i := 0; i < 2; i++ {
		if _, offset, err := resource.VideoMaterial(context.Background(), probe, filepath.Join(a.cacheDir(), "clips"), sel.Song); err != nil || offset != 500 {
			t.Fatalf("完整录像复用失败：%d %v", offset, err)
		}
	}
	log, err := os.ReadFile(os.Getenv("QA_SIDECAR_LOG"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(log), "start") != 2 || strings.Contains(string(log), "section") {
		t.Fatalf("完整下载取消、重试后不应再次下载区间：%s", log)
	}
	material := resource.VideoCachePath(filepath.Join(a.cacheDir(), "clips"), song.SongID, "FTR", song.Video.URL)
	if err := os.Remove(material); err != nil {
		t.Fatal(err)
	}
	states, err = a.VideoInfos(&score.Project{Songs: []score.Song{sel.Song}})
	if err != nil {
		t.Fatal(err)
	}
	if states[0].MaterialReady {
		t.Errorf("完整录像已删除却仍报告 material_ready=true：%+v", states[0])
	}
	entries, err := os.ReadDir(filepath.Join(a.cacheDir(), "clips"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Errorf("遗留下载临时目录：%s", entry.Name())
		}
	}
}

func TestQARealBilibiliPrepareConfirmAndProxy(t *testing.T) {
	yt := os.Getenv("QA_REAL_YTDLP")
	if yt == "" {
		t.Skip("设置 QA_REAL_YTDLP 才运行真实网络验收")
	}
	a, song, probe := qaVideoApp(t)
	paths, err := loadToolPaths()
	if err != nil {
		t.Fatal(err)
	}
	paths.YtDlp = yt
	if err := saveToolPaths(paths); err != nil {
		t.Fatal(err)
	}
	song.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}
	state, err := a.PrepareVideo(song)
	if err != nil || !state.PreviewReady || state.MaterialReady || state.Confirmed {
		t.Fatalf("真实预览解析失败：duration=%d %v", state.DurationMs, err)
	}
	for _, url := range []string{state.PreviewURL, state.PreviewAudioURL} {
		if url == "" {
			continue
		}
		for _, method := range []string{"GET", "HEAD"} {
			r := httptest.NewRequest(method, url, nil)
			r.Header.Set("Range", "bytes=0-1023")
			w := httptest.NewRecorder()
			NewPreviewHandler(a).ServeHTTP(w, r)
			if w.Code != 206 || w.Header().Get("Content-Range") == "" {
				t.Fatalf("真实代理 %s 失败：status=%d", method, w.Code)
			}
			if method == "HEAD" && w.Body.Len() != 0 {
				t.Fatal("HEAD 返回正文")
			}
		}
	}
	sel, err := a.ConfirmVideoRange(song, score.ClipRange{StartMs: 60123, DurationMs: 2000})
	if err != nil || !sel.State.Confirmed {
		t.Fatalf("真实范围保存失败：%v", err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for {
		a.mu.Lock()
		busy := a.videoWorker
		a.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			a.CancelVideoDownload()
			qaWait(t, func() bool { a.mu.Lock(); defer a.mu.Unlock(); return !a.videoWorker })
			t.Fatal("真实下载超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
	states, err := a.VideoInfos(&score.Project{Songs: []score.Song{sel.Song}})
	if err != nil || !states[0].MaterialReady {
		t.Fatalf("真实区间素材未就绪：%+v %v", states, err)
	}
	for i := 0; i < 2; i++ {
		path, offset, err := resource.VideoMaterial(context.Background(), probe, filepath.Join(a.cacheDir(), "clips"), sel.Song)
		if err != nil || offset != 0 {
			t.Fatalf("真实素材偏移错误：%d %v", offset, err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("应用真实区间读取 %d：原始起点=60123，素材起点=%d，字节数=%d", i, offset, fi.Size())
		if i == 0 {
			dir := t.TempDir()
			for _, name := range []string{"base.png", "overlay.png"} {
				im := image.NewNRGBA(image.Rect(0, 0, 1920, 1080))
				if name == "base.png" {
					draw.Draw(im, im.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
				}
				file, err := os.Create(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, im); err != nil {
					file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			}
			base, overlay := filepath.Join(dir, "base.png"), filepath.Join(dir, "overlay.png")
			seg := filepath.Join(dir, "seg.mp4")
			if err := render.Composite(context.Background(), render.Layers{Clip: path, BG: base, Jacket: base, Template: overlay, Text: overlay, Win: render.Templates[true].Win, StartMs: offset, DurationMs: 2000, AudioFade: render.AudioFade{In: true, Out: true}}, seg, paths.FFmpeg); err != nil {
				t.Fatal(err)
			}
			info, err := render.Probe(context.Background(), probe, seg)
			if err != nil || info.DurationMs < 1950 || info.DurationMs > 2050 || info.CodecName != "h264/aac" || info.PixFmt != "yuv420p" {
				t.Fatalf("真实区间合成失败：%+v %v", info, err)
			}
			t.Logf("真实 B站区间合成：%dms %s %s", info.DurationMs, info.CodecName, info.PixFmt)
		}
	}
}

func TestQARealYouTubePreparation(t *testing.T) {
	if os.Getenv("QA_REAL_YOUTUBE") == "" {
		t.Skip("设置 QA_REAL_YOUTUBE 和 QA_REAL_YTDLP 才运行真实 YouTube 验收")
	}
	yt := os.Getenv("QA_REAL_YTDLP")
	if yt == "" {
		t.Fatal("缺少 QA_REAL_YTDLP")
	}
	a, song, _ := qaVideoApp(t)
	paths, err := loadToolPaths()
	if err != nil {
		t.Fatal(err)
	}
	paths.YtDlp = yt
	if err := saveToolPaths(paths); err != nil {
		t.Fatal(err)
	}
	song.Video = &score.VideoRef{URL: "https://www.youtube.com/watch?v=jNQXAC9IVRw"}
	state, err := a.PrepareVideo(song)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		a.mu.Lock()
		busy := a.videoWorker
		a.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("真实 YouTube 获取超时")
		}
		time.Sleep(50 * time.Millisecond)
	}
	states, err := a.VideoInfos(&score.Project{Songs: []score.Song{song}})
	if err != nil {
		t.Fatal(err)
	}
	if states[0].RequestID != state.RequestID || !states[0].PreviewReady {
		t.Fatalf("真实 YouTube 未就绪：status=%s error=%s", states[0].Status, states[0].Error)
	}
}

func TestQABilibiliPrepareDoesNotDownloadAndSourceIsolation(t *testing.T) {
	a, local, probe := qaVideoApp(t)
	qaSidecar(t, a, probe)
	song := local
	song.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}
	state, err := a.PrepareVideo(song)
	if err != nil || !state.PreviewReady || state.PreviewAudioURL == "" || state.MaterialReady || state.Confirmed {
		t.Fatalf("B站准备只应解析预览：%+v %v", state, err)
	}
	if _, err := os.Stat(os.Getenv("QA_SIDECAR_LOG")); !os.IsNotExist(err) {
		t.Fatal("未确认即执行下载")
	}
	other := song
	other.Video = &score.VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=2"}
	next, err := a.PrepareVideo(other)
	if err != nil || next.RequestID == state.RequestID {
		t.Fatalf("换源应换任务编号：%+v %v", next, err)
	}
	if err := a.CancelVideoPreparation(song.SongID, "FTR", state.RequestID); err == nil {
		t.Fatal("旧请求取消了新来源")
	}
	if got := a.failVideo(videoSourceKey(song.SongID, "FTR"), state.RequestID, fmt.Errorf("旧请求失败")); got.RequestID != "" {
		t.Fatalf("旧失败污染新任务：%+v", got)
	}
	states, err := a.VideoInfos(&score.Project{Songs: []score.Song{other}})
	if err != nil || states[0].RequestID != next.RequestID || states[0].Error != "" {
		t.Fatalf("换源状态丢失：%+v %v", states, err)
	}
	b, err := json.Marshal(states)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("bilivideo")) {
		t.Fatalf("状态泄露上游媒体地址：%s", b)
	}
}
