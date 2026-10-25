package job

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"rhythm-game-bvideo/internal/render"
)

// SegInput 负责为每段处理幂等提供数据结构
type SegInput struct {
	Clip     string            `json:"clip"`
	Bg       string            `json:"bg"`
	Jacket   string            `json:"jacket"`
	Template string            `json:"template"`
	TextHash string            `json:"text_hash"`
	Win      render.RectWin    `json:"win"`
	Value    map[string]string `json:"value"`
}

// isFile 判断传入的是不是文件
func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// hash 用于幂等去重的证据
func (in SegInput) hash() string {
	b, err := json.Marshal(in)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// reusable 用于去重
func reusable(ctx context.Context, ffprobe, dir, seg string, in SegInput) bool {
	if !isFile(seg) {
		return false
	}
	b, err := os.ReadFile(filepath.Join(dir, "input.json"))
	if err != nil {
		return false
	}
	want := in.hash()
	if want == "" || strings.TrimSpace(string(b)) != want {
		return false
	}
	info, err := render.Probe(ctx, ffprobe, seg)
	return err == nil && info.PixFmt == "yuv420p"
}
