package render

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"rhythm-game-bvideo/internal/tools"
)

// Templates 是两种格式的谱面确认视频嵌入定位点
var Templates = map[bool]Template{
	true:  {Source: "templates/bg16-9.png", Win: RectWin{20, 157, 934, 526}},
	false: {Source: "templates/bg4-3.png", Win: RectWin{58, 95, 855, 641}},
}

// JacketSlot 是曲绘的嵌入定位点，两种格式都一样
var JacketSlot = RectWin{1289, 312, 160, 159}

// BackgroundArgs 是背景层的 ffmpeg 参数
func BackgroundArgs(bgPath, dest string) []string {
	return []string{
		"-y", "-v", "error",
		"-i", bgPath,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,gblur=sigma=%d",
			OutW, OutH, OutW, OutH, BlurSigma),
		dest,
	}
}

// BuildBackground 负责背景搭建
func BuildBackground(ctx context.Context, ffmpeg, bgPath, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpeg, BackgroundArgs(bgPath, dest)...)
	tools.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build background error: %w (ctx: %v, out=%s)",
			err, ctx.Err(), strings.TrimSpace(string(out)))
	}
	return nil
}

// RenderText 用于在无头浏览器上渲染文字
func RenderText(ctx context.Context, values map[string]string, edge, textHTMLPath, profileDir, dest string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Edge 可能返回成功却不写新图；旧截图必须先移除，避免误当成本次结果。
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("移除旧文字截图失败：%w", err)
	}

	cmd := exec.CommandContext(ctx, edge, TextArgs(textHTMLPath, profileDir, dest, values)...)
	tools.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("render text error: %w, output=%s", err, strings.TrimSpace(string(out)))
	}
	if !waitForScreenshot(ctx, dest) {
		return fmt.Errorf("edge render text error, output=%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Edge 有时先返回成功，再异步写出截图
func waitForScreenshot(ctx context.Context, path string) bool {
	ready := func() bool {
		fi, err := os.Stat(path)
		return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
	}
	if ready() {
		return true
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if ready() {
				return true
			}
		case <-timer.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// TextArgs 是文字层的 Edge 参数（**不含 edge 路径本身**）
func TextArgs(textHTMLPath, profileDir, dest string, values map[string]string) []string {
	// 拼接URL查询参数（必须是 make 过的 map，nil map 上 Set 会 panic）
	qu := url.Values{}
	for k, v := range values {
		qu.Set(k, v)
	}
	htmlPath := url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(textHTMLPath), RawQuery: qu.Encode()}
	return []string{
		"--headless=new",
		"--disable-gpu",
		"--hide-scrollbars",
		"--force-device-scale-factor=1",
		"--window-size=1920,1080",
		"--default-background-color=00000000",
		"--virtual-time-budget=4000",
		"--user-data-dir=" + profileDir,
		"--screenshot=" + dest,
		htmlPath.String(),
	}
}

// Composite 用于把各层组装起来
func Composite(ctx context.Context, l Layers, dest, ffmpeg string) error {
	timeout := 30 * time.Second
	if l.DurationMs > 0 {
		timeout += 5 * time.Duration(l.DurationMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpeg, CompositeArgs(l, dest)...)
	tools.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w (ctx=%v, out=%s)",
			err, ctx.Err(), strings.TrimSpace(string(out)))
	}
	return nil
}

// CompositeArgs 是五层合成的 ffmpeg 参数
func CompositeArgs(l Layers, dest string) []string {
	// 各层参数
	w := l.Win
	durationMs := SecPerSong * 1000
	if l.DurationMs > 0 {
		durationMs = l.DurationMs
	}
	filter := strings.Join([]string{
		fmt.Sprintf("[0:v]fps=%d,scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1[vid]", FPS, w.W, w.H, w.W, w.H),
		fmt.Sprintf("[1:v]format=rgba[bg];[bg][vid]overlay=%d:%d:format=auto[o1]", w.X, w.Y),
		fmt.Sprintf("[2:v]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,format=rgba[jk]", JacketSlot.W, JacketSlot.H, JacketSlot.W, JacketSlot.H),
		fmt.Sprintf("[o1][jk]overlay=%d:%d:format=auto[o2]", JacketSlot.X, JacketSlot.Y),
		"[3:v]format=rgba[ch]",
		"[o2][ch]overlay=0:0:format=auto[o3]",
		"[4:v]format=rgba[tx]",
		"[o3][tx]overlay=0:0:format=auto,scale=out_range=tv,format=yuv420p,setsar=1," + videoFade(durationMs) + "[v]",
	}, ";")
	args := []string{
		"-y", "-v", "error",
	}
	if l.StartMs > 0 {
		args = append(args, "-ss", strconv.FormatFloat(float64(l.StartMs)/1000, 'f', 3, 64))
	}
	args = append(args, "-i", l.Clip)
	duration := strconv.Itoa(SecPerSong)
	if l.DurationMs > 0 {
		duration = strconv.FormatFloat(float64(l.DurationMs)/1000, 'f', 3, 64)
	}
	return append(args,
		"-loop", "1", "-framerate", "30", "-i", l.BG,
		"-loop", "1", "-framerate", "30", "-i", l.Jacket,
		"-loop", "1", "-framerate", "30", "-i", l.Template,
		"-loop", "1", "-framerate", "30", "-i", l.Text,
		"-filter_complex", filter,
		"-map", "[v]",
		"-map", "0:a:0",
		"-c:a", "aac", "-b:a", "192k", "-ar", "48000", "-ac", "2",
		"-t", duration,
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "18",
		"-pix_fmt", "yuv420p",
		"-color_range", "tv",
		"-g", "60", "-keyint_min", "60", "-sc_threshold", "0",
		dest,
	)
}

// Concat 用于将多个视频片段拼接起来
func Concat(ctx context.Context, ffmpeg, ffprobe, listFile, outFile string, segs []string) error {
	// 校验规格
	var baseInfo StreamInfo
	var baseSeg string
	for i, seg := range segs {
		info, err := Probe(ctx, ffprobe, seg)
		if err != nil {
			return err
		}
		if i == 0 {
			baseInfo, baseSeg = info, seg
			continue
		}
		if fingerprint(info) != fingerprint(baseInfo) {
			return fmt.Errorf("The %dth ffmpeg output mismatch, \n  base %s\n         %s\n  difference %s\n         %s",
				i+1, filepath.Base(baseSeg), fingerprint(baseInfo), filepath.Base(seg), fingerprint(info))
		}

	}

	// 调用ffmpeg拼接
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	err := concatList(listFile, segs)
	if err != nil {
		return fmt.Errorf("concat list error: %w", err)
	}
	cmd := exec.CommandContext(ctx, ffmpeg, ConcatArgs(listFile, outFile)...)
	tools.HideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("concat error: %w (ctx=%v, out=%s)",
			err, ctx.Err(), strings.TrimSpace(string(out)))
	}
	return nil
}

// ConcatArgs 是 concat demuxer 的参数（**不含 ffmpeg 路径本身**）
func ConcatArgs(listFile, outFile string) []string {
	return []string{
		"-y", "-v", "error",
		"-f", "concat", "-safe", "0",
		"-i", listFile,
		"-c", "copy",
		"-movflags", "+faststart",
		outFile,
	}
}

// concatList 用于拼接各个文件
func concatList(listFile string, segs []string) error {
	var b strings.Builder
	for _, s := range segs {
		p := filepath.ToSlash(s)               // 正反斜杠转换
		p = strings.ReplaceAll(p, "'", `'\''`) // 单引号转义
		b.WriteString("file '")
		b.WriteString(p)
		b.WriteString("'\n")
	}
	return os.WriteFile(listFile, []byte(b.String()), 0o644)
}

// fingerprint 用于方便校验一致性，是各项参数拼接的签名
func fingerprint(i StreamInfo) string {
	return fmt.Sprintf("%s/%dx%d/%s/%s/%s|%sHz/%dch",
		i.CodecName, i.Width, i.Height, i.PixFmt, i.AvgFrameRate, i.SAR,
		i.SampleRate, i.Channels)
}

// Probe 负责校验参数一致性
func Probe(ctx context.Context, ffprobe string, file string) (StreamInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var raw struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Streams []struct {
			CodecType    string `json:"codec_type"`
			CodecName    string `json:"codec_name"`
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			PixFmt       string `json:"pix_fmt"`
			AvgFrameRate string `json:"avg_frame_rate"`
			SAR          string `json:"sample_aspect_ratio"`
			SampleRate   string `json:"sample_rate"`
			Channels     int    `json:"channels"`
		} `json:"streams"`
	}

	// ffprobe执行
	cmd := exec.CommandContext(ctx, ffprobe,
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name,width,height,pix_fmt,avg_frame_rate,sample_aspect_ratio,sample_rate,channels:format=duration",
		"-of", "json", file)
	tools.HideWindow(cmd)

	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return StreamInfo{}, fmt.Errorf("ffprobe %s failed: %w, stderr=%s", file, err, ee.Stderr)
		}
		return StreamInfo{}, fmt.Errorf("ffprobe %s failed: %w", file, err)
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return StreamInfo{}, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}

	// 合并：视频流填前半，音频流填后半
	var info StreamInfo
	var vCodec, aCodec string
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			info.Width, info.Height = s.Width, s.Height
			info.PixFmt, info.AvgFrameRate, info.SAR = s.PixFmt, s.AvgFrameRate, s.SAR
			vCodec = s.CodecName
		case "audio":
			info.SampleRate, info.Channels = s.SampleRate, s.Channels
			aCodec = s.CodecName
		}
	}
	// 视频流和音频流拼接
	info.CodecName = vCodec + "/" + aCodec // "h264/aac"
	if seconds, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil && !math.IsNaN(seconds) && !math.IsInf(seconds, 0) && seconds > 0 {
		info.DurationMs = int(math.Round(seconds * 1000))
	}
	return info, nil
}

// isFile 判断传入的是不是文件
func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}
