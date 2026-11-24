package resource

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"rhythm-game-bvideo/internal/score"
)

func qaMediaTools(t *testing.T) (string, string) {
	t.Helper()
	f, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 ffmpeg")
	}
	p, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("需要 ffprobe")
	}
	return f, p
}

func qaMediaRun(t *testing.T, command string, args ...string) []byte {
	t.Helper()
	out, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("媒体命令失败：%v %s", err, out)
	}
	return out
}

func TestQARangeMaterialActualCutAndReuse(t *testing.T) {
	f, p := qaMediaTools(t)
	dir := t.TempDir()
	original := filepath.Join(dir, "original.mp4")
	qaMediaRun(t, f, "-y", "-v", "error", "-f", "lavfi", "-i", "testsrc2=s=160x90:r=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "7", "-c:v", "libx264", "-g", "60", "-keyint_min", "60", "-sc_threshold", "0", "-c:a", "aac", original)
	const source = "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"
	for _, tc := range []struct{ start, duration int }{{0, 1000}, {123, 1000}, {2123, 2000}, {5990, 1000}} {
		t.Run(fmt.Sprintf("%d_%d", tc.start, tc.duration), func(t *testing.T) {
			path := RangeCachePath(dir, "qa", "FTR", source, tc.start, tc.duration)
			qaMediaRun(t, f, "-y", "-v", "error", "-ss", fmt.Sprintf("%.3f", float64(tc.start)/1000), "-i", original, "-t", fmt.Sprintf("%.3f", float64(tc.duration)/1000), "-c", "copy", path)
			b, err := json.Marshal(RangeOrigin{Source: source, StartMs: tc.start, DurationMs: tc.duration})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path+".json", b, 0600); err != nil {
				t.Fatal(err)
			}
			if !ValidRange(context.Background(), p, path, source, tc.start, tc.duration) {
				t.Fatal("合法的流复制素材被拒绝")
			}
			song := score.Song{SongID: "qa", Difficulty: score.DiffRef{Name: "FTR"}, Video: &score.VideoRef{URL: source}, Clip: &score.ClipRange{StartMs: tc.start, DurationMs: tc.duration}}
			song.Clip.ConfirmedKey = score.ClipKey(song)
			for i := 0; i < 2; i++ {
				got, offset, err := VideoMaterial(context.Background(), p, dir, song)
				if err != nil || got != path || offset != 0 {
					t.Fatalf("重复生成不得再次应用原录像偏移：%q %d %v", got, offset, err)
				}
			}
			frame := func(file string, start int) string {
				return strings.TrimSpace(string(qaMediaRun(t, f, "-v", "error", "-ss", fmt.Sprintf("%.3f", float64(start)/1000), "-i", file, "-an", "-frames:v", "1", "-f", "md5", "-")))
			}
			audio := func(file string, start int) []byte {
				return qaMediaRun(t, f, "-v", "error", "-ss", fmt.Sprintf("%.3f", float64(start)/1000), "-i", file, "-vn", "-t", "0.2", "-f", "f32le", "-")
			}
			want, got := audio(original, tc.start), audio(path, 0)
			if len(want) != len(got) || len(want) == 0 {
				t.Fatalf("切点前 200ms 音频样本数不同：%d/%d", len(want), len(got))
			}
			difference := 0.0
			for i := 0; i < len(want); i += 4 {
				v := float64(math.Float32frombits(binary.LittleEndian.Uint32(want[i:i+4]))) - float64(math.Float32frombits(binary.LittleEndian.Uint32(got[i:i+4])))
				difference += v * v
			}
			difference = math.Sqrt(difference / float64(len(want)/4))
			// AAC 浮点解码允许低于 -80dB 的数值误差；错位一个采样就会超限。
			if difference > 0.0001 {
				t.Errorf("切点前 200ms 音频未对齐：PCM 差值 RMS=%g", difference)
			}
			t.Logf("原始偏移=%dms，素材偏移=0；首帧一致，音频差值 RMS=%g", tc.start, difference)
			if want, got := frame(original, tc.start), frame(path, 0); want != got {
				t.Errorf("原录像精确切点与短素材首帧不一致：want=%s got=%s", want, got)
				t.Logf("短素材首包：%s", qaMediaRun(t, p, "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1", "-show_entries", "packet=pts_time,dts_time,flags", "-of", "json", path))
				all := string(qaMediaRun(t, f, "-v", "error", "-i", original, "-an", "-f", "framemd5", "-"))
				for _, line := range strings.Split(all, "\n") {
					if strings.HasSuffix(line, strings.TrimPrefix(got, "MD5=")) {
						t.Logf("短素材首帧对应原录像帧（30fps）：%s", line)
					}
				}
				for _, delta := range []int{-34, 34} {
					if tc.start+delta >= 0 {
						t.Logf("原始切点 %+dms 首帧：%s", delta, frame(original, tc.start+delta))
					}
				}
			}
			if ValidRange(context.Background(), p, path, source, tc.start+1, tc.duration) {
				t.Error("旁车范围变化后仍能复用")
			}
		})
	}
}

func TestQAPrerollRejectsUnsafeKeyframes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("探测替身使用 POSIX shell")
	}
	for _, tc := range []struct {
		pts, flags string
		valid      bool
	}{{"-10", "K_", true}, {"-10.001", "KD", false}, {"0.05", "K_", true}, {"0.051", "K_", false}, {"0", "__", false}, {"NaN", "K_", false}, {"Inf", "K_", false}} {
		t.Run(tc.pts+tc.flags, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ffprobe")
			data := fmt.Sprintf(`{"packets":[{"pts_time":%q,"flags":%q}]}`, tc.pts, tc.flags)
			if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' '"+data+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if got := validSectionPreroll(context.Background(), path, "fixture.mp4"); got != tc.valid {
				t.Errorf("预卷边界判断=%t，期望 %t", got, tc.valid)
			}
		})
	}
}

func TestQAResolvePreviewFiltersHeadersAndRejectsInvalidMedia(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("解析替身使用 POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "yt-dlp")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf '%s\\n' \"$QA_STREAM_JSON\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	const source = "https://www.bilibili.com/video/BV1op4y1W7M2"
	valid := `{"duration":2,"uploader":"QA","url":"https://qa.bilivideo.com/media","vcodec":"avc1.64001f","acodec":"mp4a.40.2","http_headers":{"User-Agent":"qa","Cookie":"secret","Authorization":"secret","Referer":"https://evil.test/","Accept":"bad\r\nheader"}}`
	t.Setenv("QA_STREAM_JSON", valid)
	streams, err := ResolvePreview(context.Background(), path, source)
	if err != nil {
		t.Fatal(err)
	}
	if streams.VideoHeaders.Get("Cookie") != "" || streams.VideoHeaders.Get("Authorization") != "" || streams.VideoHeaders.Get("Accept") != "" || streams.VideoHeaders.Get("Referer") != "https://www.bilibili.com/" || streams.VideoHeaders.Get("User-Agent") != "qa" {
		t.Fatalf("解析结果保留不安全请求头：%v", streams.VideoHeaders)
	}
	for _, data := range []string{strings.Replace(valid, "https://qa.bilivideo.com/media", "https://localhost/media", 1), strings.Replace(valid, `"duration":2`, `"duration":0`, 1), strings.Replace(valid, "mp4a.40.2", "none", 1), strings.Replace(valid, "avc1.64001f", "vp9", 1), "not json"} {
		t.Setenv("QA_STREAM_JSON", data)
		if _, err := ResolvePreview(context.Background(), path, source); err == nil {
			t.Error("无效流被接受")
		}
	}
}

func TestQARangeKeysAndPrerollLimits(t *testing.T) {
	base := RangeCachePath("cache", "song", "FTR", "source", 1000, 2000)
	for _, other := range []string{RangeCachePath("cache", "other", "FTR", "source", 1000, 2000), RangeCachePath("cache", "song", "BYD", "source", 1000, 2000), RangeCachePath("cache", "song", "FTR", "other", 1000, 2000), RangeCachePath("cache", "song", "FTR", "source", 1001, 2000), RangeCachePath("cache", "song", "FTR", "source", 1000, 2001), VideoCachePath("cache", "song", "FTR", "source")} {
		if other == base {
			t.Errorf("缓存键冲突：%s", other)
		}
	}
	for _, raw := range []string{"https://example.com/x", "https://bilivideo.com/x", "https://x.bilivideo.com.evil.test/x", "http://x.bilivideo.com/x", "https://x.bilivideo.com:443/x", "https://user@x.bilivideo.com/x"} {
		if BilibiliMediaURL(raw) {
			t.Errorf("非法代理来源：%s", raw)
		}
	}
}

func TestQARealBilibiliApplicationMaterial(t *testing.T) {
	yt := os.Getenv("QA_REAL_YTDLP")
	if yt == "" {
		t.Skip("设置 QA_REAL_YTDLP 才运行真实网络验收")
	}
	f, p := qaMediaTools(t)
	ctx := context.Background()
	const source = "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"
	streams, err := ResolvePreview(ctx, yt, source)
	if err != nil {
		t.Fatal(err)
	}
	if streams.DurationMs < 62123 || streams.VideoURL == "" {
		t.Fatalf("预览不可用：duration=%d", streams.DurationMs)
	}
	dir := t.TempDir()
	path, _, err := DownloadVideoProgress(ctx, yt, f, p, dir, "qa", "FTR", source, 60123, 2000, nil)
	if err != nil {
		t.Fatal(err)
	}
	song := score.Song{SongID: "qa", Difficulty: score.DiffRef{Name: "FTR"}, Video: &score.VideoRef{URL: source}, Clip: &score.ClipRange{StartMs: 60123, DurationMs: 2000}}
	song.Clip.ConfirmedKey = score.ClipKey(song)
	for i := 0; i < 2; i++ {
		got, offset, err := VideoMaterial(ctx, p, dir, song)
		if err != nil || got != path || offset != 0 {
			t.Fatalf("真实素材读取错误：%q %d %v", got, offset, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("真实区间素材字节数=%d，原始起点=60123，渲染起点=0", info.Size())
}
