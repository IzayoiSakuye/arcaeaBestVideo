package resource

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

func TestVideoURL(t *testing.T) {
	got, err := VideoURL("https://www.bilibili.com/video/BV1op4y1W7M2?p=14&spm=abc")
	if err != nil || got != "https://www.bilibili.com/video/BV1op4y1W7M2?p=14" {
		t.Fatalf("链接规范化失败：%q, %v", got, err)
	}
	for _, raw := range []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=12s",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ&list=abc",
		"https://youtu.be/dQw4w9WgXcQ?si=abc",
	} {
		if got, err := VideoURL(raw); err != nil || got != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
			t.Errorf("YouTube 链接规范化失败：%q, %v", got, err)
		}
	}
	for _, raw := range []string{
		"https://www.bilibili.com.evil.test/video/BV1op4y1W7M2",
		"http://www.bilibili.com/video/BV1op4y1W7M2",
		"https://www.bilibili.com/video/BV1op4y1W7M2?p=0",
		"https://www.youtube.com.evil.test/watch?v=dQw4w9WgXcQ",
		"https://www.youtube.com/playlist?list=abc",
		"https://youtu.be/dQw4w9WgXcQ/other",
		"https://www.youtube.com/watch?v=invalid",
		"https://user@www.youtube.com/watch?v=dQw4w9WgXcQ",
	} {
		if _, err := VideoURL(raw); err == nil {
			t.Errorf("应拒绝无效链接：%s", raw)
		}
	}
}

func TestVideoCachePathSeparatesSources(t *testing.T) {
	dir := t.TempDir()
	first := VideoCachePath(dir, "song", "FTR", "https://www.bilibili.com/video/BV1op4y1W7M2?p=1")
	for _, path := range []string{
		VideoCachePath(dir, "song", "BYD", "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"),
		VideoCachePath(dir, "song", "FTR", "https://www.bilibili.com/video/BV1op4y1W7M2?p=2"),
		VideoCachePath(dir, "other", "FTR", "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"),
	} {
		if path == first || filepath.Dir(path) != dir || !strings.HasSuffix(path, ".mp4") {
			t.Fatalf("缓存键未区分来源：%q / %q", first, path)
		}
	}
}

func TestVideoCachePathDoesNotReuseTenSecondClip(t *testing.T) {
	const link = "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"
	old := sha256.Sum256([]byte("song\x00FTR\x00" + link + "\x00start=0;duration=10"))
	oldPath := filepath.Join("clips", hex.EncodeToString(old[:16])+".mp4")
	if got := VideoCachePath("clips", "song", "FTR", link); got == oldPath {
		t.Fatal("完整录像不能复用旧的十秒缓存")
	}
}
