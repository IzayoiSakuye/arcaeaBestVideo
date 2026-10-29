package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type VideoState struct {
	Index      int    `json:"index"`
	URL        string `json:"url"`
	OwnerName  string `json:"owner_name"`
	Status     string `json:"status"`
	PreviewURL string `json:"preview_url"`
}

// VideoInfos 只检查本地文件，供校对页展示每首歌的素材状态
func (a *App) VideoInfos(project *score.Project) ([]VideoState, error) {
	if project == nil {
		return nil, fmt.Errorf("缺少成绩数据")
	}
	cata, err := a.catalog()
	if err != nil {
		return nil, err
	}
	out := make([]VideoState, 0, len(project.Songs))
	for i, song := range project.Songs {
		state := a.videoInfo(cata, song)
		state.Index = i + 1
		out = append(out, state)
	}
	return out, nil
}

func (a *App) videoInfo(cata *catalog.Catalog, song score.Song) VideoState {
	state := VideoState{Status: "missing"}
	if song.SongID == "" || song.Difficulty.Name == "" {
		return state
	}
	if song.Video != nil && song.Video.LocalPath != "" {
		state.OwnerName = song.Video.OwnerName
		if fileReady(song.Video.LocalPath) {
			state.Status = "ready"
			state.PreviewURL = a.previewURL(song.Video.LocalPath)
		}
		return state
	}
	if song.Video != nil && song.Video.URL != "" {
		state.URL, state.OwnerName = song.Video.URL, song.Video.OwnerName
		if _, err := resource.VideoURL(state.URL); err != nil {
			state.Status = "invalid"
			return state
		}
		if state.OwnerName == "" {
			if chart, ok := cata.Chart(song.SongID, song.Difficulty.Name); ok && chart.Video != nil {
				for _, source := range chart.Video.Sources {
					if source.URL == state.URL {
						state.OwnerName = source.OwnerName
						break
					}
				}
			}
		}
		if path := resource.VideoCachePath(filepath.Join(a.root, ".cache", "clips"), song.SongID, song.Difficulty.Name, state.URL); fileReady(path) {
			state.Status = "ready"
			state.PreviewURL = a.previewURL(path)
		} else {
			state.Status = "download"
		}
		return state
	}
	chart, ok := cata.Chart(song.SongID, song.Difficulty.Name)
	if !ok || chart.Video == nil || len(chart.Video.Sources) == 0 {
		return state
	}
	for _, source := range chart.Video.Sources {
		if state.URL == "" {
			state.URL, state.OwnerName, state.Status = source.URL, source.OwnerName, "download"
		}
		path := resource.VideoCachePath(filepath.Join(a.root, ".cache", "clips"), song.SongID, song.Difficulty.Name, source.URL)
		if fileReady(path) {
			state.URL, state.OwnerName, state.Status = source.URL, source.OwnerName, "ready"
			state.PreviewURL = a.previewURL(path)
			break
		}
	}
	if state.Status != "ready" {
		if legacy, err := resource.ClipPath(filepath.Join(a.root, ".cache", "clips"), song.SongID, song.Difficulty.Name); err == nil && fileReady(legacy) {
			state.Status = "legacy"
			state.PreviewURL = a.previewURL(legacy)
		}
	}
	return state
}

func fileReady(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}

func (a *App) previewURL(path string) string {
	return a.mediaURL(path, ".mp4")
}

func (a *App) mediaURL(path, ext string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(absolute))
	token := hex.EncodeToString(sum[:16])
	a.mu.Lock()
	if a.previews == nil {
		a.previews = make(map[string]string)
	}
	a.previews[token] = absolute
	a.mu.Unlock()
	return "/preview/" + token + ext
}

// NewPreviewHandler 只提供已登记的本地录像
func NewPreviewHandler(a *App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/thought-bg.png" {
			http.ServeFile(w, r, filepath.Join(a.root, "templates", "thought-bg.png"))
			return
		}
		if r.URL.Path == "/thought-template" {
			http.ServeFile(w, r, filepath.Join(a.root, "templates", "thought.html"))
			return
		}
		ext := filepath.Ext(r.URL.Path)
		token := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/preview/"), ext)
		if !strings.HasPrefix(r.URL.Path, "/preview/") || len(token) != 32 ||
			!strings.Contains("|.mp4|.png|.jpg|.jpeg|.mp3|.wav|.flac|.m4a|.ogg|", "|"+ext+"|") {
			http.NotFound(w, r)
			return
		}
		a.mu.Lock()
		path := a.previews[token]
		a.mu.Unlock()
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil || !fi.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		if !strings.EqualFold(filepath.Ext(path), ext) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(ext))
		http.ServeContent(w, r, "preview"+ext, fi.ModTime(), f)
	})
}

func (a *App) ApplyVideoLink(songID, difficulty, raw string) (string, error) {
	if err := a.validateVideoSourceKey(songID, difficulty); err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return "", a.saveVideoSource(songID, difficulty, nil)
	}
	link, err := resource.VideoURL(raw)
	if err != nil {
		return "", err
	}
	if err := a.saveVideoSource(songID, difficulty, &score.VideoRef{URL: link}); err != nil {
		return "", err
	}
	return link, nil
}

// PickVideoFile 验证用户选择的视频
func (a *App) PickVideoFile(songID, difficulty string) (string, error) {
	if err := a.validateVideoSourceKey(songID, difficulty); err != nil {
		return "", err
	}
	path, err := runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "MP4 视频 (*.mp4)", Pattern: "*.mp4"}},
	})
	if err != nil || path == "" {
		return path, err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	overrides, err := loadToolPaths()
	if err != nil {
		return "", err
	}
	ffprobe := tools.Detect(overrides).Tools.FFprobe
	if ffprobe == "" {
		return "", fmt.Errorf("请先配置 ffprobe")
	}
	if err := resource.ValidVideo(a.context(), ffprobe, abs); err != nil {
		return "", err
	}
	if err := a.saveVideoSource(songID, difficulty, &score.VideoRef{LocalPath: abs}); err != nil {
		return "", err
	}
	return abs, nil
}

// DownloadVideo 按索引顺序尝试来源，先库再手动读取
func (a *App) DownloadVideo(song score.Song) (VideoState, error) {
	var state VideoState
	if song.SongID == "" || song.Difficulty.Name == "" {
		return state, fmt.Errorf("请先确定曲目和难度")
	}
	cata, err := a.catalog()
	if err != nil {
		return state, err
	}
	var sources []catalog.VideoSource
	manualURL := song.Video != nil && song.Video.URL != ""
	if manualURL {
		link, err := resource.VideoURL(song.Video.URL)
		if err != nil {
			return state, err
		}
		owner := a.videoInfo(cata, song).OwnerName
		sources = []catalog.VideoSource{{URL: link, OwnerName: owner}}
	} else if chart, ok := cata.Chart(song.SongID, song.Difficulty.Name); ok && chart.Video != nil {
		sources = chart.Video.Sources
	}
	if len(sources) == 0 {
		return state, fmt.Errorf("这首歌没有可下载的录像链接，请手动填写")
	}
	overrides, err := loadToolPaths()
	if err != nil {
		return state, err
	}
	resolved := tools.Detect(overrides).Tools
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	a.mu.Lock()
	if a.videoCancel != nil {
		a.mu.Unlock()
		cancel()
		return state, fmt.Errorf("已有录像正在获取")
	}
	a.videoCancel = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.videoCancel = nil
		a.mu.Unlock()
		cancel()
	}()
	var failures []string
	for _, source := range sources {
		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("录像获取已停止：%w", err)
		}
		_, fetchedOwner, err := resource.DownloadVideo(ctx, resolved.YtDlp, resolved.FFmpeg, resolved.FFprobe,
			filepath.Join(a.root, ".cache", "clips"), song.SongID, song.Difficulty.Name, source.URL)
		if err == nil {
			if source.OwnerName == "" {
				source.OwnerName = fetchedOwner
			}
			if manualURL {
				if err := a.saveVideoSource(song.SongID, song.Difficulty.Name, &score.VideoRef{URL: source.URL, OwnerName: source.OwnerName}); err != nil {
					return state, fmt.Errorf("录像已获取，但来源记录保存失败：%w", err)
				}
			}
			song.Video = &score.VideoRef{URL: source.URL, OwnerName: source.OwnerName}
			return a.videoInfo(cata, song), nil
		}
		failures = append(failures, err.Error())
	}
	return state, fmt.Errorf("所有录像来源均不可用：%s", strings.Join(failures, "；"))
}

// CancelVideoDownload 停止当前一首录像的获取
func (a *App) CancelVideoDownload() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.videoCancel != nil {
		a.videoCancel()
	}
}
