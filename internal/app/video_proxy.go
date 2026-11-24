package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"rhythm-game-bvideo/internal/resource"
)

type remotePreview struct {
	mu       sync.Mutex
	source   string
	ytDlp    string
	streams  resource.PreviewStreams
	resolved time.Time
}

func (a *App) remotePreviewURLs(source, ytDlp string, streams resource.PreviewStreams) (string, string) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", ""
	}
	token := hex.EncodeToString(b)
	a.mu.Lock()
	if a.remotePreviews == nil {
		a.remotePreviews = make(map[string]*remotePreview)
	}
	a.remotePreviews[token] = &remotePreview{source: source, ytDlp: ytDlp, streams: streams, resolved: time.Now()}
	a.mu.Unlock()
	audio := ""
	if streams.AudioURL != "" {
		audio = "/preview/" + token + ".m4a"
	}
	return "/preview/" + token + ".mp4", audio
}

func (p *remotePreview) request(r *http.Request, audio, refresh bool) (*http.Response, error) {
	p.mu.Lock()
	if refresh || time.Since(p.resolved) > 5*time.Minute {
		streams, err := resource.ResolvePreview(r.Context(), p.ytDlp, p.source)
		if err != nil {
			p.mu.Unlock()
			return nil, err
		}
		p.streams, p.resolved = streams, time.Now()
	}
	link, headers := p.streams.VideoURL, p.streams.VideoHeaders.Clone()
	if audio {
		link, headers = p.streams.AudioURL, p.streams.AudioHeaders.Clone()
	}
	p.mu.Unlock()
	if !resource.BilibiliMediaURL(link) {
		return nil, fmt.Errorf("预览媒体地址无效")
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header = headers
	for _, name := range []string{"Range", "If-Range"} {
		if value := r.Header.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	client := &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !resource.BilibiliMediaURL(req.URL.String()) {
			return fmt.Errorf("预览重定向被拒绝")
		}
		return nil
	}}
	return client.Do(req)
}

func serveRemotePreview(w http.ResponseWriter, r *http.Request, p *remotePreview, audio bool) {
	response, err := p.request(r, audio, false)
	if err == nil && (response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 410) {
		response.Body.Close()
		response, err = p.request(r, audio, true)
	}
	if err != nil {
		http.Error(w, "预览取流失败，请重新准备预览", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		if size, ok := strings.CutPrefix(response.Header.Get("Content-Range"), "bytes */"); ok {
			if _, err := strconv.ParseUint(size, 10, 64); err == nil {
				w.Header().Set("Content-Range", "bytes */"+size)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		http.Error(w, "上游预览暂不可用", http.StatusBadGateway)
		return
	}
	for _, name := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if value := response.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "video/mp4")
	if audio {
		w.Header().Set("Content-Type", "audio/mp4")
	}
	w.WriteHeader(response.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, response.Body)
	}
}

// ReleasePreview 由页面离开或替换媒体时调用，解除缓存清理保护。
func (a *App) ReleasePreview(previewURL string) {
	if !strings.HasPrefix(previewURL, "/preview/") {
		return
	}
	a.mu.Lock()
	delete(a.previewLeases, previewURL)
	a.mu.Unlock()
}

func (a *App) activatePreviewLocked(preview string) {
	token := strings.TrimSuffix(strings.TrimPrefix(preview, "/preview/"), filepath.Ext(preview))
	if path := a.previews[token]; path != "" {
		if a.previewLeases == nil {
			a.previewLeases = make(map[string]string)
		}
		a.previewLeases[preview] = path
	}
}
