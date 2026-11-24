package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/tools"
)

// PreviewStreams 仅在后端保存取流地址及请求头。
type PreviewStreams struct {
	VideoURL     string
	AudioURL     string
	VideoHeaders http.Header
	AudioHeaders http.Header
	DurationMs   int
	Owner        string
}

func BilibiliMediaURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return strings.HasSuffix(host, ".bilivideo.com") || strings.HasSuffix(host, ".bilivideo.cn")
}

func ResolvePreview(ctx context.Context, ytDlp, source string) (PreviewStreams, error) {
	var result PreviewStreams
	link, err := VideoURL(source)
	if err != nil || !strings.HasPrefix(link, "https://www.bilibili.com/") {
		return result, fmt.Errorf("在线预览仅支持 B站视频页")
	}
	if ytDlp == "" {
		return result, fmt.Errorf("请先配置 yt-dlp")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ytDlp, "--ignore-config", "--no-plugin-dirs", "--no-playlist", "--sleep-requests", "1", "--socket-timeout", "10", "--retries", "1", "--extractor-retries", "1",
		"--skip-download", "--dump-single-json", "-f", "b[height<=480][vcodec^=avc1]/bv[height<=480][vcodec^=avc1]+ba[acodec^=mp4a]", link)
	tools.HideWindow(cmd)
	b, err := cmd.Output()
	if err != nil {
		return result, fmt.Errorf("在线预览解析失败，请检查网络或更新 yt-dlp")
	}
	type stream struct {
		URL     string            `json:"url"`
		VCodec  string            `json:"vcodec"`
		ACodec  string            `json:"acodec"`
		Headers map[string]string `json:"http_headers"`
	}
	var info struct {
		stream
		Duration float64  `json:"duration"`
		Owner    string   `json:"uploader"`
		Formats  []stream `json:"requested_formats"`
	}
	if err := json.Unmarshal(b, &info); err != nil || math.IsNaN(info.Duration) || math.IsInf(info.Duration, 0) || info.Duration <= 0 || info.Duration > 86400 {
		return result, fmt.Errorf("预览媒体信息或时长无效")
	}
	formats := info.Formats
	if len(formats) == 0 {
		formats = []stream{info.stream}
	}
	hasAudio := false
	for _, f := range formats {
		if !BilibiliMediaURL(f.URL) {
			return result, fmt.Errorf("预览媒体地址不在允许的 B站域名内")
		}
		headers := make(http.Header)
		if len(f.Headers) == 0 {
			f.Headers = info.Headers
		}
		for _, name := range []string{"User-Agent", "Accept", "Accept-Language"} {
			if value := f.Headers[name]; value != "" && !strings.ContainsAny(value, "\r\n") {
				headers.Set(name, value)
			}
		}
		headers.Set("Referer", "https://www.bilibili.com/")
		if strings.HasPrefix(f.VCodec, "avc1") {
			result.VideoURL, result.VideoHeaders = f.URL, headers
			if strings.HasPrefix(f.ACodec, "mp4a") {
				result.AudioURL = ""
				hasAudio = true
			}
		} else if strings.HasPrefix(f.ACodec, "mp4a") {
			result.AudioURL, result.AudioHeaders = f.URL, headers
			hasAudio = true
		}
	}
	if result.VideoURL == "" || !hasAudio {
		return result, fmt.Errorf("没有可用的 H.264/AAC 预览流")
	}
	result.DurationMs, result.Owner = int(math.Round(info.Duration*1000)), info.Owner
	return result, nil
}
