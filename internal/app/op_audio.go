package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const opManifestURL = "https://assets.sakuye.dpdns.org/arc-data/op-latest.json"

type opManifest struct {
	SchemaVersion int      `json:"schema_version"`
	Game          string   `json:"game"`
	Resource      string   `json:"resource"`
	AudioVersion  string   `json:"audio_version"`
	Files         []opFile `json:"files"`
}

type opFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}

type OPAudioFileStatus struct {
	Kind            string `json:"kind"`
	Size            int64  `json:"size"`
	Installed       bool   `json:"installed"`
	UpdateAvailable bool   `json:"update_available"`
}

type OPAudioStatus struct {
	AudioVersion string              `json:"audio_version"`
	Files        []OPAudioFileStatus `json:"files"`
}

type OPAudioProgress struct {
	Kind       string `json:"kind"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
}

func opAudioDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rhythm-game-bvideo", "op-audio"), nil
}

func opAudioPath(kind string) string {
	if len(kind) != 3 || !strings.HasPrefix(kind, "op") || kind[2] < '1' || kind[2] > '7' {
		return ""
	}
	dir, err := opAudioDir()
	if err != nil {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var found string
	var newest time.Time
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, kind+"-") || !strings.HasSuffix(name, ".wav") || len(name) != len(kind)+1+64+4 || !validSHA256(name[len(kind)+1:len(name)-4]) {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.Mode().IsRegular() && info.Size() > 44 && (found == "" || info.ModTime().After(newest)) {
			found, newest = filepath.Join(dir, name), info.ModTime()
		}
	}
	return found
}

func readOPManifest(ctx context.Context, client *http.Client, endpoint string) (opManifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return opManifest{}, err
	}
	res, err := client.Do(req)
	if err != nil {
		return opManifest{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return opManifest{}, fmt.Errorf("OP 清单返回 HTTP %d", res.StatusCode)
	}
	var m opManifest
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&m); err != nil {
		return m, fmt.Errorf("OP 清单解析失败：%w", err)
	}
	base, _ := url.Parse(endpoint)
	if m.SchemaVersion != 1 || m.Game != "arcaea" || m.Resource != "op-audio" || !(resourceVersionPattern.MatchString(m.AudioVersion) || legacyResourceVersionPattern.MatchString(m.AudioVersion)) || len(m.Files) != 7 {
		return m, fmt.Errorf("OP 清单格式无效")
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		kind := strings.TrimSuffix(filepath.Base(f.Path), ".wav")
		u, err := url.Parse(f.URL)
		if f.Path != "templates/audio/"+kind+".wav" || len(kind) != 3 || !strings.HasPrefix(kind, "op") || kind[2] < '1' || kind[2] > '7' || seen[kind] || f.Size < 44 || f.Size > 100<<20 || !validSHA256(f.SHA256) || err != nil || u.Scheme != "https" || u.Host != base.Host || u.User != nil || !strings.EqualFold(filepath.Base(u.Path), kind+".wav") {
			return m, fmt.Errorf("OP 清单文件记录无效：%s", f.Path)
		}
		seen[kind] = true
	}
	return m, nil
}

func (a *App) CheckOPAudioUpdate() (OPAudioStatus, error) {
	ctx, cancel := context.WithTimeout(a.context(), 45*time.Second)
	defer cancel()
	m, err := readOPManifest(ctx, resourceHTTPClient(), opManifestURL)
	if err != nil {
		return OPAudioStatus{}, err
	}
	status := OPAudioStatus{AudioVersion: m.AudioVersion, Files: make([]OPAudioFileStatus, 0, 7)}
	dir, err := opAudioDir()
	if err != nil {
		return status, err
	}
	for _, f := range m.Files {
		kind := strings.TrimSuffix(filepath.Base(f.Path), ".wav")
		_, err := os.Stat(filepath.Join(dir, kind+"-"+strings.ToLower(f.SHA256)+".wav"))
		status.Files = append(status.Files, OPAudioFileStatus{Kind: kind, Size: f.Size, Installed: opAudioPath(kind) != "", UpdateAvailable: err != nil})
	}
	return status, nil
}

func installOPAudio(ctx context.Context, client *http.Client, dir string, f opFile, progress func(int64, int64)) error {
	kind := strings.TrimSuffix(filepath.Base(f.Path), ".wav")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	final := filepath.Join(dir, kind+"-"+strings.ToLower(f.SHA256)+".wav")
	if st, err := os.Stat(final); err == nil && st.Mode().IsRegular() && st.Size() == f.Size {
		if existing, err := os.Open(final); err == nil {
			h := sha256.New()
			_, copyErr := io.Copy(h, existing)
			closeErr := existing.Close()
			if copyErr == nil && closeErr == nil && strings.EqualFold(hex.EncodeToString(h.Sum(nil)), f.SHA256) {
				return nil
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("OP 下载失败：HTTP %d", res.StatusCode)
	}
	part, err := os.CreateTemp(dir, kind+"-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(part.Name())
	h := sha256.New()
	buf := make([]byte, 128<<10)
	var done int64
	lastProgress := time.Time{}
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > f.Size {
				part.Close()
				return fmt.Errorf("OP 音频超过清单声明的大小")
			}
			if _, err := part.Write(buf[:n]); err != nil {
				part.Close()
				return err
			}
			h.Write(buf[:n])
			if progress != nil && time.Since(lastProgress) >= 250*time.Millisecond {
				progress(done, f.Size)
				lastProgress = time.Now()
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				part.Close()
				return readErr
			}
			break
		}
	}
	if err := part.Close(); err != nil {
		return err
	}
	if done != f.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), f.SHA256) {
		return fmt.Errorf("OP 音频大小或 SHA-256 校验失败")
	}
	if progress != nil {
		progress(done, f.Size)
	}
	file, err := os.Open(part.Name())
	if err != nil {
		return err
	}
	var header [12]byte
	_, readErr := io.ReadFull(file, header[:])
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return fmt.Errorf("OP 音频不是有效的 WAV 文件")
	}
	if err := os.Remove(final); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(part.Name(), final); err != nil {
		return err
	}
	return nil
}

func (a *App) UpdateOPAudio(kind string) error {
	if len(kind) != 3 || !strings.HasPrefix(kind, "op") || kind[2] < '1' || kind[2] > '7' {
		return fmt.Errorf("OP 编号无效")
	}
	a.mu.Lock()
	if a.opUpdating {
		a.mu.Unlock()
		return fmt.Errorf("OP 音频更新正在进行")
	}
	a.opUpdating = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.opUpdating = false
		a.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(a.context(), 20*time.Minute)
	defer cancel()
	m, err := readOPManifest(ctx, resourceHTTPClient(), opManifestURL)
	if err != nil {
		return err
	}
	dir, err := opAudioDir()
	if err != nil {
		return err
	}
	for _, f := range m.Files {
		if f.Path == "templates/audio/"+kind+".wav" {
			return installOPAudio(ctx, &http.Client{Timeout: 20 * time.Minute}, dir, f, func(done, total int64) {
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "op:progress", OPAudioProgress{Kind: kind, Downloaded: done, Total: total})
				}
			})
		}
	}
	return fmt.Errorf("OP 清单缺少 %s", kind)
}
