package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	if ffprobe == "" {
		return "", "", fmt.Errorf("请先配置 ffprobe")
	}
	link, err := VideoURL(sourceURL)
	if err != nil {
		return "", "", err
	}
	target := VideoCachePath(dir, songID, difficulty, link)
	if ValidVideo(ctx, ffprobe, target) == nil {
		return target, "", nil
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
		"--no-playlist", "--no-progress", "--ffmpeg-location", ffmpeg,
		"-f", format,
		"--merge-output-format", "mp4",
		"--remux-video", "mp4", "--print-to-file", "after_move:%(uploader)s", ownerFile,
		"-o", filepath.Join(tempDir, "clip.%(ext)s")}
	if isYouTube {
		args = append(args, "--js-runtimes", "node")
	}
	cmd := exec.CommandContext(ctx, ytDlp, append(args, link)...)
	tools.HideWindow(cmd)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("下载录像失败：%w：%s", err, strings.TrimSpace(string(output)))
	}
	if err := ValidVideo(ctx, ffprobe, tmp); err != nil {
		return "", "", err
	}
	ownerBytes, _ := os.ReadFile(ownerFile)
	owner := strings.TrimSpace(string(ownerBytes))
	if owner == "NA" {
		owner = ""
	}
	if ValidVideo(ctx, ffprobe, target) == nil {
		return target, owner, nil
	}
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	if err := os.Rename(tmp, target); err != nil {
		return "", "", err
	}
	return target, owner, nil
}
