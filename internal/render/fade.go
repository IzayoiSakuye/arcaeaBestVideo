package render

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"rhythm-game-bvideo/internal/tools"
)

// FadeAudio 只重编码音轨，保留已渲染的视频和无损拼接参数
func FadeAudio(ctx context.Context, ffmpeg, ffprobe, source, dest string, fadeIn, fadeOut bool) (string, error) {
	info, err := Probe(ctx, ffprobe, source)
	if err != nil {
		return "", err
	}
	seconds := float64(info.DurationMs) / 1000
	if seconds <= 0 {
		return "", fmt.Errorf("片段时长无效：%s", source)
	}
	fade := 0.4
	if seconds < 1.6 {
		fade = seconds / 4
	}
	filters := make([]string, 0, 2)
	if fadeIn {
		filters = append(filters, "afade=t=in:st=0:d="+strconv.FormatFloat(fade, 'f', 3, 64))
	}
	if fadeOut {
		filters = append(filters, "afade=t=out:st="+strconv.FormatFloat(seconds-fade, 'f', 3, 64)+":d="+strconv.FormatFloat(fade, 'f', 3, 64))
	}
	if len(filters) == 0 {
		return source, nil
	}
	part := dest + ".part.mp4"
	args := []string{"-y", "-v", "error", "-i", source, "-map", "0:v:0", "-map", "0:a:0", "-c:v", "copy", "-af", strings.Join(filters, ","), "-c:a", "aac", "-b:a", "192k", "-ar", "48000", "-ac", "2", part}
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	tools.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(part)
		return "", fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if _, err := Probe(ctx, ffprobe, part); err != nil {
		os.Remove(part)
		return "", err
	}
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		os.Remove(part)
		return "", err
	}
	if err := os.Rename(part, dest); err != nil {
		os.Remove(part)
		return "", err
	}
	return dest, nil
}
