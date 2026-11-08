package render

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/score"
	"rhythm-game-bvideo/internal/tools"
)

type thoughtImageValue struct {
	Source string `json:"source"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
}

// Thought 将感想文字逐页渲染，再按相同时长拼成可独立预览的片段
func Thought(ctx context.Context, thought score.Thought, audioPath, templatesDir, workDir, ffmpeg, ffprobe, edge string) (string, error) {
	if strings.TrimSpace(thought.Name) == "" {
		return "", fmt.Errorf("请填写感想片段名称")
	}
	if thought.DurationMs < 1000 || thought.DurationMs > 600000 {
		return "", fmt.Errorf("感想时长须在 1 秒到 10 分钟之间")
	}
	pages := thought.EffectivePages()
	if len(pages) == 0 || len(pages) > 20 {
		return "", fmt.Errorf("感想文字须为 1 到 20 段")
	}
	for i, page := range pages {
		if strings.TrimSpace(page.Text) == "" {
			return "", fmt.Errorf("第 %d 段感想文字为空", i+1)
		}
		for _, im := range page.Images {
			if im.X < 0 || im.Y < 0 || im.X >= OutW || im.Y >= OutH || im.Width < 40 || im.Width > OutW || im.Path == "" {
				return "", fmt.Errorf("第 %d 段装饰图位置或宽度无效", i+1)
			}
		}
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", err
	}
	b, err := json.Marshal(thought)
	if err != nil {
		return "", err
	}
	b = append(b, []byte("video-fade-1s")...)
	for _, path := range []string{audioPath, filepath.Join(templatesDir, "thought-bg.png"), filepath.Join(templatesDir, "thought.html")} {
		if path == "" {
			continue
		}
		fi, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		b = append(b, []byte(fmt.Sprintf("%s:%d:%d", path, fi.Size(), fi.ModTime().UnixNano()))...)
	}
	for _, page := range pages {
		for _, im := range page.Images {
			fi, err := os.Stat(im.Path)
			if err != nil {
				return "", err
			}
			b = append(b, []byte(fmt.Sprintf("%s:%d:%d", im.Path, fi.Size(), fi.ModTime().UnixNano()))...)
		}
	}
	hash := sha256.Sum256(b)
	key := fmt.Sprintf("%x", hash[:12])
	output := filepath.Join(workDir, key+".mp4")
	if info, err := Probe(ctx, ffprobe, output); err == nil && validThoughtSegment(info, thought.DurationMs) {
		return output, nil
	}
	framesDir := filepath.Join(workDir, key)
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", err
	}
	// 预览和成片可能同时启动 Edge
	profileDir, err := os.MkdirTemp(workDir, "edge-profile-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(profileDir)
	frames := make([]string, 0, len(pages))
	for i, page := range pages {
		values := map[string]string{"text": page.Text}
		images := make([]thoughtImageValue, 0, len(page.Images))
		for _, im := range page.Images {
			absImage, err := filepath.Abs(im.Path)
			if err != nil {
				return "", err
			}
			images = append(images, thoughtImageValue{Source: (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(absImage)}).String(), X: im.X, Y: im.Y, Width: im.Width})
		}
		if len(images) > 0 {
			encoded, err := json.Marshal(images)
			if err != nil {
				return "", err
			}
			values["images"] = string(encoded)
		}
		frame := filepath.Join(framesDir, fmt.Sprintf("%02d.png", i))
		if err := os.Remove(frame); err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err := RenderText(ctx, values, edge, filepath.Join(templatesDir, "thought.html"), profileDir, frame); err != nil {
			return "", fmt.Errorf("第 %d 段感想画面生成失败：%w", i+1, err)
		}
		frames = append(frames, frame)
	}
	listFile := filepath.Join(framesDir, "frames.txt")
	var list strings.Builder
	for _, frame := range frames {
		quoted := strings.ReplaceAll(filepath.ToSlash(frame), "'", `'\''`)
		fmt.Fprintf(&list, "file '%s'\nduration %.6f\n", quoted, float64(thought.DurationMs)/1000/float64(len(frames)))
	}
	fmt.Fprintf(&list, "file '%s'\n", strings.ReplaceAll(filepath.ToSlash(frames[len(frames)-1]), "'", `'\''`))
	if err := os.WriteFile(listFile, []byte(list.String()), 0o644); err != nil {
		return "", err
	}
	args := []string{"-y", "-v", "error", "-f", "concat", "-safe", "0", "-i", listFile}
	if audioPath == "" {
		args = append(args, "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo")
	} else {
		args = append(args, "-ss", strconv.FormatFloat(float64(thought.Audio.StartMs)/1000, 'f', 3, 64), "-i", audioPath)
	}
	filter := "[0:v]fps=30,scale=1920:1080,setsar=1,scale=out_range=tv,format=yuv420p," + videoFade(thought.DurationMs) + "[v]"
	audioMap := "1:a:0"
	if strings.HasPrefix(thought.Audio.Kind, "op") {
		samples := (thought.Audio.EndMs - thought.Audio.StartMs) * 48
		if samples <= 0 {
			return "", fmt.Errorf("OP 音频截取范围无效")
		}
		filter += fmt.Sprintf(";[1:a]aresample=48000,atrim=end_sample=%d,asetpts=PTS-STARTPTS,aloop=loop=-1:size=%d[a]", samples, samples)
		audioMap = "[a]"
	}
	args = append(args, "-filter_complex", filter,
		"-map", "[v]", "-map", audioMap, "-t", strconv.FormatFloat(float64(thought.DurationMs)/1000, 'f', 3, 64),
		"-c:v", "libx264", "-preset", Preset, "-crf", strconv.Itoa(CRF), "-pix_fmt", "yuv420p", "-color_range", "tv",
		"-g", strconv.Itoa(GOP), "-keyint_min", strconv.Itoa(GOP), "-sc_threshold", "0",
		"-c:a", "aac", "-b:a", "192k", "-ar", "48000", "-ac", "2", output+".part.mp4")
	limit := 2*time.Minute + time.Duration(thought.DurationMs)*time.Millisecond*5
	processCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	cmd := exec.CommandContext(processCtx, ffmpeg, args...)
	tools.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		os.Remove(output + ".part.mp4")
		return "", fmt.Errorf("感想视频生成失败：%w：%s", err, strings.TrimSpace(string(out)))
	}
	part := output + ".part.mp4"
	info, err := Probe(ctx, ffprobe, part)
	if err != nil || !validThoughtSegment(info, thought.DurationMs) {
		os.Remove(part)
		return "", fmt.Errorf("感想视频参数验证失败：%v", err)
	}
	if err := os.Remove(output); err != nil && !os.IsNotExist(err) {
		os.Remove(part)
		return "", err
	}
	if err := os.Rename(part, output); err != nil {
		os.Remove(part)
		return "", err
	}
	return output, nil
}

func validThoughtSegment(info StreamInfo, durationMs int) bool {
	return info.CodecName == "h264/aac" && info.Width == OutW && info.Height == OutH &&
		info.PixFmt == "yuv420p" && info.AvgFrameRate == "30/1" && info.SAR == "1:1" &&
		info.SampleRate == "48000" && info.Channels == 2 && abs(info.DurationMs-durationMs) < 100
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
