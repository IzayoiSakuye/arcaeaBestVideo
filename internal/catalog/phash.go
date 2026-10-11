package catalog

import (
	"bytes"
	"context"
	"fmt"
	"math/bits"
	"os/exec"
	"sort"
	"strconv"
)

// grayPixels 调 ffmpeg 把图缩到指定尺寸的灰度裸数据。
func grayPixels(ctx context.Context, ffmpeg, file, filter string, need int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-y", "-i", file,
		"-vf", filter, "-f", "rawvideo", "-pix_fmt", "gray", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg 解码失败 %s: %w (%s)", file, err, stderr.String())
	}
	if len(out) < need {
		return nil, fmt.Errorf("灰度数据不足 %d < %d: %s", len(out), need, file)
	}
	return out, nil
}

// aHashBits  计算 8×8 均值哈希
func aHashBits(px []byte) uint64 {
	sum := 0
	for i := 0; i < 64; i++ {
		sum += int(px[i])
	}
	avg := float64(sum) / 64
	var v uint64
	for i := 0; i < 64; i++ {
		if float64(px[i]) >= avg {
			v |= 1 << (63 - i)
		}
	}
	return v
}

// dHashBits 计算 9×8 水平差分哈希
func dHashBits(px []byte) uint64 {
	var v uint64
	i := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if px[y*9+x] > px[y*9+x+1] {
				v |= 1 << (63 - i)
			}
			i++
		}
	}
	return v
}

// AHash 算一张图的 8×8 均值哈希
func AHash(ctx context.Context, ffmpeg, file string) (uint64, error) {
	px, err := grayPixels(ctx, ffmpeg, file, "scale=8:8", 64)
	if err != nil {
		return 0, err
	}
	return aHashBits(px), nil
}

// DHash 算一张图的 9×8 差分哈希
func DHash(ctx context.Context, ffmpeg, file string) (uint64, error) {
	px, err := grayPixels(ctx, ffmpeg, file, "scale=9:8", 72)
	if err != nil {
		return 0, err
	}
	return dHashBits(px), nil
}

// Hamming 两个指纹差几位，0 = 完全一样
func Hamming(a, b uint64) int { return bits.OnesCount64(a ^ b) }

// AHashBits 取曲库里存着的指纹（十六进制）
func (s *Song) AHashBits() (uint64, bool) {
	if s.PHash == nil || s.PHash.AHash == nil {
		return 0, false
	}
	v, err := strconv.ParseUint(*s.PHash.AHash, 16, 64)
	return v, err == nil
}

// JacketMatch 是按曲绘找出来的候选
type JacketMatch struct {
	Song *Song
	Dist int
}

// MatchJacket 按指纹找候选，距离升序
func (c *Catalog) MatchJacket(h uint64, n int) []JacketMatch {
	var out []JacketMatch
	for _, s := range c.Songs {
		sh, ok := s.AHashBits()
		if !ok {
			continue
		}
		out = append(out, JacketMatch{Song: s, Dist: Hamming(h, sh)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Dist < out[j].Dist })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
