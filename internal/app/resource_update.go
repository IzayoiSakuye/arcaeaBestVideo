package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/resource"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const resourceManifestURL = "https://github.com/IzayoiSakuye/arcaea-b50-resources/releases/latest/download/latest.json"
const resourceCDNManifestURL = "https://assets.sakuye.dpdns.org/arc-data/latest.json"
const resourceR2ManifestURL = "https://pub-af1517927a52401bb02d6f98bba5199e.r2.dev/arc-data/latest.json"
const resourceReleaseAPI = "https://api.github.com/repos/IzayoiSakuye/arcaea-b50-resources/releases/latest"

var resourceZipProxies = []string{"https://ghfast.top/", "https://gh-proxy.org/"}

var resourceVersionPattern = regexp.MustCompile(`^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:\.(?:0|[1-9]\d*))?$`)
var legacyResourceVersionPattern = regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}(?:\.\d+)?$`)

type resourceManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Game          string `json:"game"`
	DataVersion   string `json:"data_version"`
	Archive       struct {
		Path   string `json:"path"`
		URL    string `json:"url"`
		Size   int64  `json:"size"`
		SHA256 string `json:"sha256"`
	} `json:"archive"`
	Files     []resourceFile `json:"files"`
	sourceURL string
}

type resourceFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type ResourceUpdateStatus struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	RestartRequired bool   `json:"restart_required"`
}

type ResourceProgress struct {
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
}

func resourceStoreDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rhythm-game-bvideo", "resources"), nil
}

// 允许配置额外镜像；默认源失败时按顺序尝试其他源，旧资源始终留在本地
func resourceManifestURLs() []string {
	urls := []string{}
	for _, s := range strings.Split(os.Getenv("ARCAEA_RESOURCE_MIRRORS"), ";") {
		if s = strings.TrimSpace(s); s != "" {
			urls = append(urls, s)
		}
	}
	return append(urls, resourceCDNManifestURL, resourceR2ManifestURL, resourceReleaseAPI, resourceManifestURL)
}

func resourceHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 30 * time.Second
	return &http.Client{Transport: transport, Timeout: 45 * time.Second}
}

func validSHA256(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

func validResourcePath(p string) bool {
	return !strings.Contains(p, "\\") && path.Clean(p) == p && filepath.IsLocal(filepath.FromSlash(p)) &&
		(p == "meta.json" || strings.HasPrefix(p, "data/"))
}

func (m resourceManifest) validate() error {
	if m.SchemaVersion != 1 || m.Game != "arcaea" || !(resourceVersionPattern.MatchString(m.DataVersion) || legacyResourceVersionPattern.MatchString(m.DataVersion)) {
		return fmt.Errorf("资源清单版本或游戏不受支持")
	}
	u, err := url.Parse(m.Archive.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		m.Archive.Path == "" || filepath.Base(m.Archive.Path) != m.Archive.Path || !strings.HasSuffix(m.Archive.Path, ".zip") ||
		m.Archive.Size <= 0 || m.Archive.Size > 1<<30 || !validSHA256(m.Archive.SHA256) {
		return fmt.Errorf("资源归档地址或校验信息无效")
	}
	if len(m.Files) == 0 || len(m.Files) > 20000 {
		return fmt.Errorf("资源文件清单为空或过大")
	}
	seen := make(map[string]bool, len(m.Files))
	var total int64
	for _, f := range m.Files {
		if !validResourcePath(f.Path) || seen[f.Path] || f.Size < 0 || f.Size > 100<<20 || !validSHA256(f.SHA256) {
			return fmt.Errorf("资源文件记录无效：%s", f.Path)
		}
		seen[f.Path] = true
		total += f.Size
	}
	if total > 1<<30 {
		return fmt.Errorf("资源解包大小超出限制")
	}
	for _, required := range []string{"meta.json", "data/arcaea-catalog.json", "data/arcaea-backgrounds.json"} {
		if !seen[required] {
			return fmt.Errorf("资源包缺少 %s", required)
		}
	}
	return nil
}

func readManifest(ctx context.Context, client *http.Client, urls []string) (resourceManifest, error) {
	var failures []string
	for _, source := range urls {
		if source == resourceReleaseAPI {
			m, err := readManifestFromReleaseAPI(ctx, client, source)
			if err == nil {
				return m, nil
			}
			failures = append(failures, err.Error())
			continue
		}
		u, err := url.Parse(source)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			failures = append(failures, "更新源地址无效")
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		res, err := client.Do(req)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			failures = append(failures, fmt.Sprintf("%s 返回 HTTP %d", u.Host, res.StatusCode))
			continue
		}
		var m resourceManifest
		data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
		res.Body.Close()
		if err == nil && len(data) <= 2<<20 {
			err = json.Unmarshal(data, &m)
		}
		if err == nil {
			err = m.validate()
		}
		if err == nil {
			m.sourceURL = source
			return m, nil
		}
		failures = append(failures, fmt.Sprintf("%s 清单无效：%v", u.Host, err))
	}
	return resourceManifest{}, fmt.Errorf("资源更新检查失败：%s", strings.Join(failures, "；"))
}

// GitHub API 可在 Release 页面握手失败时提供清单和附件校验值
func readManifestFromReleaseAPI(ctx context.Context, client *http.Client, endpoint string) (resourceManifest, error) {
	apiURL, err := url.Parse(endpoint)
	if err != nil || apiURL.Scheme != "https" || apiURL.Host == "" {
		return resourceManifest{}, fmt.Errorf("GitHub API 地址无效")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return resourceManifest{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	res, err := client.Do(req)
	if err != nil {
		return resourceManifest{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return resourceManifest{}, fmt.Errorf("GitHub API 返回 HTTP %d", res.StatusCode)
	}
	var release struct {
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"url"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&release); err != nil {
		return resourceManifest{}, err
	}
	for _, asset := range release.Assets {
		if asset.Name != "latest.json" {
			continue
		}
		assetURL, err := url.Parse(asset.URL)
		if err != nil || assetURL.Scheme != "https" || assetURL.Host != apiURL.Host {
			return resourceManifest{}, fmt.Errorf("GitHub 清单附件地址无效")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
		if err != nil {
			return resourceManifest{}, err
		}
		req.Header.Set("Accept", "application/octet-stream")
		res, err := client.Do(req)
		if err != nil {
			return resourceManifest{}, err
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return resourceManifest{}, fmt.Errorf("GitHub 清单附件返回 HTTP %d", res.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(res.Body, 2<<20+1))
		res.Body.Close()
		if err != nil || len(data) > 2<<20 {
			return resourceManifest{}, fmt.Errorf("GitHub 清单附件读取失败或过大：%v", err)
		}
		hash := sha256.Sum256(data)
		if asset.Digest != "sha256:"+hex.EncodeToString(hash[:]) {
			return resourceManifest{}, fmt.Errorf("GitHub 清单附件 SHA-256 校验失败")
		}
		var m resourceManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return resourceManifest{}, err
		}
		if err := m.validate(); err != nil {
			return resourceManifest{}, err
		}
		m.sourceURL = resourceManifestURL
		return m, nil
	}
	return resourceManifest{}, fmt.Errorf("GitHub Release 缺少 latest.json")
}

func compareResourceVersion(a, b string) int {
	// 旧版日期号必须排在游戏版本号之前，才能从已安装资源升级。
	if oldA, oldB := legacyResourceVersionPattern.MatchString(a), legacyResourceVersionPattern.MatchString(b); oldA != oldB {
		if oldA {
			return -1
		}
		return 1
	}
	aa, bb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 4; i++ {
		var x, y int
		if i < len(aa) {
			x, _ = strconv.Atoi(aa[i])
		}
		if i < len(bb) {
			y, _ = strconv.Atoi(bb[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func resourceAvailable(m resourceManifest, version, digest string) bool {
	return version == "" || compareResourceVersion(m.DataVersion, version) > 0 ||
		(compareResourceVersion(m.DataVersion, version) == 0 && !strings.EqualFold(m.Archive.SHA256, digest))
}

func validateResourceRoot(root string) error {
	c, err := catalog.Load(filepath.Join(root, "data", "arcaea-catalog.json"), 1)
	if err != nil {
		return err
	}
	bg, err := resource.LoadBgManifest(filepath.Join(root, "data", "arcaea-backgrounds.json"))
	if err != nil {
		return err
	}
	check := func(p string) error {
		if !validResourcePath(p) || !strings.HasPrefix(p, "data/") {
			return fmt.Errorf("素材路径无效：%s", p)
		}
		st, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
			return fmt.Errorf("素材缺失：%s", p)
		}
		return nil
	}
	for _, song := range c.Songs {
		if song.Jacket != nil && song.Jacket.Bundled && song.Jacket.Local256 != nil {
			if err := check(*song.Jacket.Local256); err != nil {
				return err
			}
		}
		for _, chart := range song.Charts {
			if chart.Jacket != nil && chart.Jacket.Bundled && chart.Jacket.Local256 != nil {
				if err := check(*chart.Jacket.Local256); err != nil {
					return err
				}
			}
		}
	}
	for _, entry := range bg.Bgs {
		for _, file := range entry.Files {
			if file.Path != "" {
				if err := check(file.Path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func installedResource(store string) (string, string, string) {
	entries, err := os.ReadDir(filepath.Join(store, "versions"))
	if err != nil {
		return "", "", ""
	}
	var root, version, digest string
	var newest time.Time
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(store, "versions", entry.Name())
		b, err := os.ReadFile(filepath.Join(candidate, "latest.json"))
		if err != nil {
			continue
		}
		var m resourceManifest
		if json.Unmarshal(b, &m) != nil || m.validate() != nil || entry.Name() != m.DataVersion+"-"+m.Archive.SHA256[:12] || validateResourceRoot(candidate) != nil {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		cmp := compareResourceVersion(m.DataVersion, version)
		if root == "" || cmp > 0 || (cmp == 0 && info.ModTime().After(newest)) {
			root, version, digest, newest = candidate, m.DataVersion, m.Archive.SHA256, info.ModTime()
		}
	}
	return root, version, digest
}

func installResource(ctx context.Context, client *http.Client, store string, m resourceManifest, progress func(int64, int64)) (string, error) {
	versions := filepath.Join(store, "versions")
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return "", err
	}
	final := filepath.Join(versions, m.DataVersion+"-"+m.Archive.SHA256[:12])
	if err := validateResourceRoot(final); err == nil {
		return final, nil
	}
	if _, err := os.Stat(final); err == nil {
		return "", fmt.Errorf("已有同版本资源损坏：%s", final)
	}
	stage, err := os.MkdirTemp(versions, ".part-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "archive.part")
	if err := downloadResourceArchive(ctx, client, archive, m, progress); err != nil {
		return "", err
	}
	if err := extractResourceArchive(archive, stage, m.Files); err != nil {
		return "", err
	}
	if err := validateResourceRoot(stage); err != nil {
		return "", fmt.Errorf("资源内容校验失败：%w", err)
	}
	if err := os.Remove(archive); err != nil {
		return "", err
	}
	manifestBytes, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "latest.json"), manifestBytes, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(stage, final); err != nil {
		return "", err
	}
	return final, nil
}

func resourceArchiveURLs(m resourceManifest) []string {
	urls := []string{}
	if m.sourceURL != "" && m.sourceURL != resourceManifestURL {
		if source, err := url.Parse(m.sourceURL); err == nil {
			archiveURL := source.ResolveReference(&url.URL{Path: m.Archive.Path}).String()
			if archiveURL != m.Archive.URL {
				urls = append(urls, archiveURL)
			}
		}
	}
	if strings.HasPrefix(m.Archive.URL, "https://github.com/") {
		for _, prefix := range resourceZipProxies {
			urls = append(urls, prefix+m.Archive.URL)
		}
	}
	urls = append(urls, m.Archive.URL)
	return urls
}

func downloadResourceArchive(ctx context.Context, client *http.Client, archive string, m resourceManifest, progress func(int64, int64)) error {
	var failures []string
	for _, source := range resourceArchiveURLs(m) {
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if err = downloadResourceArchiveFrom(ctx, client, source, archive, m, progress); err == nil {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		failures = append(failures, err.Error())
	}
	return fmt.Errorf("资源包下载失败：%s", strings.Join(failures, "；"))
}

func downloadResourceArchiveFrom(ctx context.Context, client *http.Client, source, archive string, m resourceManifest, progress func(int64, int64)) error {
	f, err := os.OpenFile(archive, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > m.Archive.Size {
		if err := f.Truncate(0); err != nil {
			return err
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	downloaded, err := io.Copy(h, f)
	if err != nil {
		return err
	}
	if downloaded == m.Archive.Size && strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.Archive.SHA256) {
		return nil
	}
	if downloaded == m.Archive.Size {
		if err := f.Truncate(0); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		downloaded = 0
		h.Reset()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	if downloaded > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusPartialContent {
		var start, end, total int64
		if n, err := fmt.Sscanf(res.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total); err != nil || n != 3 || start != downloaded || end < start || end >= total || total != m.Archive.Size {
			return fmt.Errorf("资源包续传范围无效")
		}
	} else if res.StatusCode == http.StatusOK {
		// 来源不支持 Range 时重新下载，不能将完整响应接在旧文件后面。
		if err := f.Truncate(0); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		downloaded = 0
		h.Reset()
	} else {
		return fmt.Errorf("下载资源失败：HTTP %d", res.StatusCode)
	}
	buf := make([]byte, 128<<10)
	lastProgress := time.Time{}
	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			downloaded += int64(n)
			if downloaded > m.Archive.Size {
				return fmt.Errorf("资源包超过清单声明的大小")
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			h.Write(buf[:n])
			if progress != nil && time.Since(lastProgress) >= 250*time.Millisecond {
				progress(downloaded, m.Archive.Size)
				lastProgress = time.Now()
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				return readErr
			}
			break
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if progress != nil {
		progress(downloaded, m.Archive.Size)
	}
	if downloaded != m.Archive.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.Archive.SHA256) {
		return fmt.Errorf("资源包大小或 SHA-256 校验失败")
	}
	return nil
}

func extractResourceArchive(archive, root string, files []resourceFile) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) != len(files) {
		return fmt.Errorf("ZIP 条目数与清单不符")
	}
	expected := make(map[string]resourceFile, len(files))
	for _, f := range files {
		expected[f.Path] = f
	}
	seen := make(map[string]bool, len(files))
	for _, entry := range z.File {
		want, ok := expected[entry.Name]
		if !ok || seen[entry.Name] || !entry.Mode().IsRegular() || int64(entry.UncompressedSize64) != want.Size {
			return fmt.Errorf("ZIP 条目无效：%s", entry.Name)
		}
		seen[entry.Name] = true
		dest := filepath.Join(root, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		src, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			src.Close()
			return err
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(src, want.Size+1))
		closeErr := out.Close()
		src.Close()
		if copyErr != nil || closeErr != nil || n != want.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want.SHA256) {
			return fmt.Errorf("资源文件校验失败：%s", entry.Name)
		}
	}
	return nil
}

func (a *App) CheckResourceUpdate() (ResourceUpdateStatus, error) {
	st := ResourceUpdateStatus{CurrentVersion: a.dataVersion}
	ctx, cancel := context.WithTimeout(a.context(), 90*time.Second)
	defer cancel()
	m, err := readManifest(ctx, resourceHTTPClient(), resourceManifestURLs())
	if err != nil {
		return st, err
	}
	st.LatestVersion = m.DataVersion
	st.UpdateAvailable = resourceAvailable(m, a.dataVersion, a.dataDigest)
	return st, nil
}

// UpdateResources 只安装新版本；当前进程继续使用旧资源，重启后才切换。
func (a *App) UpdateResources() (ResourceUpdateStatus, error) {
	a.mu.Lock()
	if a.resourceUpdating {
		a.mu.Unlock()
		return ResourceUpdateStatus{}, fmt.Errorf("资源更新正在进行")
	}
	a.resourceUpdating = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.resourceUpdating = false
		a.mu.Unlock()
	}()
	st := ResourceUpdateStatus{CurrentVersion: a.dataVersion}
	ctx, cancel := context.WithTimeout(a.context(), 90*time.Second)
	defer cancel()
	m, err := readManifest(ctx, resourceHTTPClient(), resourceManifestURLs())
	if err != nil {
		return st, err
	}
	st.LatestVersion = m.DataVersion
	st.UpdateAvailable = resourceAvailable(m, a.dataVersion, a.dataDigest)
	if !st.UpdateAvailable {
		return st, nil
	}
	store, err := resourceStoreDir()
	if err != nil {
		return st, err
	}
	_, err = installResource(a.context(), &http.Client{Timeout: 20 * time.Minute}, store, m, func(done, total int64) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "resource:progress", ResourceProgress{Downloaded: done, Total: total})
		}
	})
	if err != nil {
		return st, err
	}
	st.RestartRequired = true
	st.UpdateAvailable = false
	return st, nil
}
