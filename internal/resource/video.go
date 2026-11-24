package resource

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/tools"
)

var bvidPath = regexp.MustCompile(`^/video/(BV[0-9A-Za-z]{10})/?$`)
var youtubeID = regexp.MustCompile(`^[0-9A-Za-z_-]{11}$`)

// VideoURL 只接收明确的视频页，避免把任意地址交给 yt-dlp。
func VideoURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Port() != "" || u.User != nil {
		return "", fmt.Errorf("请输入完整的 B站或 YouTube 视频链接")
	}
	switch strings.ToLower(u.Hostname()) {
	case "www.bilibili.com":
		m := bvidPath.FindStringSubmatch(u.Path)
		if m == nil {
			return "", fmt.Errorf("链接中未找到 BV 号")
		}
		q := u.Query()
		if p := q.Get("p"); p != "" {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 {
				return "", fmt.Errorf("分 P 参数无效")
			}
			q = url.Values{"p": {strconv.Itoa(n)}}
		} else {
			q = nil
		}
		return (&url.URL{Scheme: "https", Host: "www.bilibili.com", Path: "/video/" + m[1], RawQuery: q.Encode()}).String(), nil
	case "youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be":
		id := ""
		if strings.EqualFold(u.Hostname(), "youtu.be") {
			id = strings.Trim(u.Path, "/")
		} else if u.Path == "/watch" {
			id = u.Query().Get("v")
		}
		if !youtubeID.MatchString(id) {
			return "", fmt.Errorf("链接中未找到 YouTube 视频 ID")
		}
		return "https://www.youtube.com/watch?v=" + id, nil
	default:
		return "", fmt.Errorf("仅支持 B站或 YouTube 视频链接")
	}
}

// VideoCachePath 将曲目、谱面和来源一起放进键
func VideoCachePath(dir, songID, difficulty, sourceURL string) string {
	sum := sha256.Sum256([]byte(songID + "\x00" + difficulty + "\x00" + sourceURL + "\x00full"))
	return filepath.Join(dir, hex.EncodeToString(sum[:16])+".mp4")
}

func ValidVideo(ctx context.Context, ffprobe, path string) error {
	if !isFile(path) {
		return fmt.Errorf("录像文件不存在：%s", path)
	}
	info, err := render.Probe(ctx, ffprobe, path)
	if err != nil || info.Width <= 0 || info.Height <= 0 {
		return fmt.Errorf("录像文件无法读取视频画面：%s", path)
	}
	return nil
}

// DownloadVideo 下载完整录像到临时目录，验证成功后才进入缓存
func DownloadVideo(ctx context.Context, ytDlp, ffmpeg, ffprobe, dir, songID, difficulty, sourceURL string) (string, string, error) {
	return DownloadVideoProgress(ctx, ytDlp, ffmpeg, ffprobe, dir, songID, difficulty, sourceURL, 0, 0, nil)
}

type DownloadProgress struct {
	DownloadedBytes int64 `json:"downloaded_bytes"`
	TotalBytes      int64 `json:"total_bytes"`
}

func RangeCachePath(dir, songID, difficulty, source string, startMs, durationMs int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d\x00range-copy-v1", songID, difficulty, source, startMs, durationMs)))
	return filepath.Join(dir, hex.EncodeToString(sum[:16])+".mp4")
}

type RangeOrigin struct {
	Source       string `json:"source"`
	StartMs      int    `json:"start_ms"`
	DurationMs   int    `json:"duration_ms"`
	MediaStartMs int    `json:"media_start_ms"`
	Owner        string `json:"owner"`
}

func CachedVideoOwner(path string) string {
	b, err := os.ReadFile(path + ".json")
	var origin RangeOrigin
	if err != nil || json.Unmarshal(b, &origin) != nil {
		return ""
	}
	return origin.Owner
}

func ValidRange(ctx context.Context, ffprobe, path, source string, startMs, durationMs int) bool {
	b, err := os.ReadFile(path + ".json")
	var origin RangeOrigin
	if err != nil || json.Unmarshal(b, &origin) != nil || origin.Source != source || origin.StartMs != startMs || origin.DurationMs != durationMs || origin.MediaStartMs != 0 {
		return false
	}
	info, err := render.Probe(ctx, ffprobe, path)
	return err == nil && validSectionInfo(info, durationMs) && validSectionPreroll(ctx, ffprobe, path)
}

func validSectionInfo(info render.StreamInfo, durationMs int) bool {
	// 保留切点前的解码关键帧，最终输出仍按用户确认的时长裁切。
	return info.CodecName == "h264/aac" && info.Width > 0 && info.Height > 0 && info.DurationMs >= durationMs-100 && info.DurationMs <= durationMs+10000
}

func validSectionPreroll(ctx context.Context, ffprobe, path string) bool {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffprobe, "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1", "-show_entries", "packet=pts_time,flags", "-of", "json", path)
	tools.HideWindow(cmd)
	b, err := cmd.Output()
	var data struct {
		Packets []struct {
			PTS   string `json:"pts_time"`
			Flags string `json:"flags"`
		} `json:"packets"`
	}
	if err != nil || json.Unmarshal(b, &data) != nil || len(data.Packets) != 1 {
		return false
	}
	pts, err := strconv.ParseFloat(data.Packets[0].PTS, 64)
	return err == nil && !math.IsNaN(pts) && !math.IsInf(pts, 0) && pts >= -10 && pts <= 0.05 && strings.Contains(data.Packets[0].Flags, "K")
}

// DownloadVideoProgress 的区间文件从零计时；原录像时间只记录在旁车文件中。
func DownloadVideoProgress(ctx context.Context, ytDlp, ffmpeg, ffprobe, dir, songID, difficulty, sourceURL string, startMs, durationMs int, progress func(DownloadProgress)) (string, string, error) {
	if ffprobe == "" {
		return "", "", fmt.Errorf("请先配置 ffprobe")
	}
	link, err := VideoURL(sourceURL)
	if err != nil {
		return "", "", err
	}
	target := VideoCachePath(dir, songID, difficulty, link)
	section := durationMs > 0
	if startMs < 0 || durationMs < 0 || (section && (durationMs < 1000 || startMs > 86400000-durationMs)) {
		return "", "", fmt.Errorf("录像区间无效")
	}
	if section {
		target = RangeCachePath(dir, songID, difficulty, link, startMs, durationMs)
	}
	ready := func() bool {
		if section {
			return ValidRange(ctx, ffprobe, target, link, startMs, durationMs)
		}
		return ValidVideo(ctx, ffprobe, target) == nil
	}
	if ready() {
		return target, CachedVideoOwner(target), nil
	}
	if ytDlp == "" || ffmpeg == "" {
		return "", "", fmt.Errorf("请先配置 yt-dlp 和 ffmpeg")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	tempDir, err := os.MkdirTemp(dir, "video-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tempDir)
	tmp := filepath.Join(tempDir, "clip.mp4")
	ownerFile := filepath.Join(tempDir, "owner.txt")
	format := "bv*[vcodec^=avc1]+ba/b[vcodec^=avc1]"
	isYouTube := strings.HasPrefix(link, "https://www.youtube.com/watch?")
	if isYouTube {
		format = "bv*[ext=mp4]+ba[ext=m4a]/b[ext=mp4]/bv*+ba/b"
	}
	args := []string{
		"--ignore-config", "--no-plugin-dirs", "--no-playlist", "--progress", "--newline", "--progress-delta", "0.5",
		"--progress-template", "download:PROG|%(progress.downloaded_bytes)s|%(progress.total_bytes,progress.total_bytes_estimate)s",
		"--limit-rate", "4M", "--concurrent-fragments", "1", "--sleep-requests", "1", "--retries", "2", "--fragment-retries", "2", "--socket-timeout", "15", "--ffmpeg-location", ffmpeg,
		"-f", format,
		"--merge-output-format", "mp4",
		"--remux-video", "mp4", "--print-to-file", "after_move:%(uploader)s", ownerFile,
		"-o", filepath.Join(tempDir, "clip.%(ext)s")}
	if isYouTube {
		args = append(args, "--js-runtimes", "node")
	}
	if section {
		args = append(args, "--download-sections", fmt.Sprintf("*%.3f-%.3f", float64(startMs)/1000, float64(startMs+durationMs)/1000),
			"--downloader-args", "ffmpeg_i:-readrate 2")
	}
	cmd := exec.CommandContext(ctx, ytDlp, append(args, link)...)
	tools.HideWindow(cmd)
	tools.CancelProcessTree(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	// 不把第三方诊断中的取流签名地址传给前端。
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("无法启动 yt-dlp：%w", err)
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), "|")
		if len(parts) == 3 && parts[0] == "PROG" && progress != nil {
			done, _ := strconv.ParseInt(parts[1], 10, 64)
			total, _ := strconv.ParseInt(parts[2], 10, 64)
			progress(DownloadProgress{DownloadedBytes: done, TotalBytes: total})
		}
	}
	if scanner.Err() != nil {
		_ = cmd.Cancel()
	}
	if err := cmd.Wait(); err != nil {
		return "", "", fmt.Errorf("下载录像失败，请检查网络及工具配置：%w", err)
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	if section {
		info, err := render.Probe(ctx, ffprobe, tmp)
		if err != nil || !validSectionInfo(info, durationMs) || !validSectionPreroll(ctx, ffprobe, tmp) {
			return "", "", fmt.Errorf("区间素材参数或时长验证失败")
		}
	}
	if err := ValidVideo(ctx, ffprobe, tmp); err != nil {
		return "", "", err
	}
	ownerBytes, _ := os.ReadFile(ownerFile)
	owner := strings.TrimSpace(string(ownerBytes))
	if owner == "NA" {
		owner = ""
	}
	if ready() {
		return target, owner, nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", "", err
	}
	{
		b, err := json.Marshal(RangeOrigin{Source: link, StartMs: startMs, DurationMs: durationMs, Owner: owner})
		if err != nil {
			return "", "", err
		}
		meta := filepath.Join(tempDir, "origin.json")
		if err := os.WriteFile(meta, b, 0o600); err != nil {
			return "", "", err
		}
		if err := os.Rename(meta, target+".json"); err != nil {
			return "", "", err
		}
	}
	return target, owner, nil
}
