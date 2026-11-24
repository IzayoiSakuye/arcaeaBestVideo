package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type VideoState struct {
	Index           int     `json:"index"`
	URL             string  `json:"url"`
	OwnerName       string  `json:"owner_name"`
	Status          string  `json:"status"`
	PreviewURL      string  `json:"preview_url"`
	PreviewAudioURL string  `json:"preview_audio_url,omitempty"`
	SongID          string  `json:"song_id"`
	Difficulty      string  `json:"difficulty"`
	RequestID       string  `json:"request_id"`
	DurationMs      int     `json:"duration_ms"`
	PreviewReady    bool    `json:"preview_ready"`
	Confirmed       bool    `json:"confirmed"`
	MaterialReady   bool    `json:"material_ready"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	TotalBytes      int64   `json:"total_bytes"`
	Progress        float64 `json:"progress"`
	Error           string  `json:"error,omitempty"`
}

// VideoInfos 查询准备状态，不触发解析或下载。
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
		state.PreviewReady = state.PreviewURL != ""
		if score.ClipConfirmed(song) {
			state.Confirmed = true
			state.MaterialReady = fileReady(a.videoMaterialPath(song))
			if state.MaterialReady {
				state.Status = "ready"
			} else if state.Status == "ready" {
				state.Status = "preview"
			}
		}
		a.mu.Lock()
		if task := a.videoTasks[videoSourceKey(song.SongID, song.Difficulty.Name)]; task != nil {
			if song.Video == nil || videoIdentity(song) == task.Source {
				a.refreshVideoTaskLocked(task)
				state = task.State
				if !score.ClipConfirmed(song) || task.Song.Clip == nil || task.Song.Clip.ConfirmedKey != song.Clip.ConfirmedKey {
					state.Confirmed, state.MaterialReady = false, false
					if state.Status == "ready" {
						state.Status = "preview"
					}
				}
			}
		}
		a.mu.Unlock()
		state.SongID, state.Difficulty = song.SongID, song.Difficulty.Name
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
		if path := resource.VideoCachePath(filepath.Join(a.cacheDir(), "clips"), song.SongID, song.Difficulty.Name, state.URL); fileReady(path) {
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
		path := resource.VideoCachePath(filepath.Join(a.cacheDir(), "clips"), song.SongID, song.Difficulty.Name, source.URL)
		if fileReady(path) {
			state.URL, state.OwnerName, state.Status = source.URL, source.OwnerName, "ready"
			state.PreviewURL = a.previewURL(path)
			break
		}
	}
	if state.Status != "ready" {
		if legacy, err := resource.ClipPath(filepath.Join(a.cacheDir(), "clips"), song.SongID, song.Difficulty.Name); err == nil && fileReady(legacy) {
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

// NewPreviewHandler 只提供已登记的本地文件及 B站预览流。
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
		remote := a.remotePreviews[token]
		a.cacheUsers++
		a.mu.Unlock()
		defer func() { a.mu.Lock(); a.cacheUsers--; a.mu.Unlock() }()
		if remote != nil && (ext == ".mp4" || ext == ".m4a") {
			serveRemotePreview(w, r, remote, ext == ".m4a")
			return
		}
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
		if r.Method == http.MethodGet {
			a.mu.Lock()
			a.activatePreviewLocked(r.URL.Path)
			a.mu.Unlock()
		}
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

// DownloadVideo 保留原绑定名称，返回素材准备状态；完成进度通过 video:event 或 VideoInfos 获取。
func (a *App) DownloadVideo(song score.Song) (VideoState, error) {
	return a.PrepareVideo(song)
}

// CancelVideoDownload 停止素材队列，单项取消使用 CancelVideoPreparation。
func (a *App) CancelVideoDownload() {
	a.mu.Lock()
	var states []VideoState
	for _, task := range a.videoTasks {
		if task.State.Status == "queued" || task.State.Status == "preparing" || task.State.Status == "downloading" {
			if task.Cancel != nil {
				task.Cancel()
			}
			task.State.Status, task.State.Error = "cancelled", "素材获取已取消"
			states = append(states, task.State)
		}
	}
	a.mu.Unlock()
	for _, state := range states {
		a.emitVideo(state)
	}
}
