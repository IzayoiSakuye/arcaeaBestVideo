package catalog

import (
	"bytes"
	"context"
	"fmt"
	"math/bits"
	"os/exec"
	"sort"
	"strconv"

	"rhythm-game-bvideo/internal/tools"
)

// grayPixels 调 ffmpeg 把图缩到指定尺寸的灰度裸数据
func grayPixels(ctx context.Context, ffmpeg, file, filter string, need int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, ffmpeg, "-v", "error", "-y", "-i", file,
		"-vf", filter, "-f", "rawvideo", "-pix_fmt", "gray", "-")
	tools.HideWindow(cmd)
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

// phashOf 解析一条曲绘记录的指纹
func phashOf(p *PHash) (a, d uint64, hasA, hasD bool) {
	if p == nil || p.AHash == nil {
		return 0, 0, false, false
	}
	av, err := strconv.ParseUint(*p.AHash, 16, 64)
	if err != nil {
		return 0, 0, false, false
	}
	if p.DHash == nil {
		return av, 0, true, false
	}
	dv, err := strconv.ParseUint(*p.DHash, 16, 64)
	if err != nil {
		return av, 0, true, false
	}
	return av, dv, true, true
}

// SongHash 取歌曲默认封面的指纹
func (s *Song) SongHash() (a, d uint64, hasA, hasD bool) { return phashOf(s.PHash) }

// ChartHash 取该难度专属封面的指纹（目前只有 57 张 BYD 有）
func (c *Chart) ChartHash() (a, d uint64, hasA, hasD bool) { return phashOf(c.PHash) }

// JacketMatch 是按曲绘找出来的候选
type JacketMatch struct {
	Song      *Song
	ChartDiff string
	DHashDist int
	AHashDist int
}

// MatchJacket 拿裁出来的曲绘找候选
func (c *Catalog) MatchJacket(cropA, cropD uint64, n int) []JacketMatch {
	const worst = 64

	pick := func(s *Song) (JacketMatch, bool) {
		best, ok := JacketMatch{}, false
		consider := func(a, d uint64, hasA, hasD bool, diff string) {
			if !hasA {
				return
			}
			m := JacketMatch{Song: s, ChartDiff: diff, AHashDist: Hamming(cropA, a), DHashDist: worst}
			if hasD {
				m.DHashDist = Hamming(cropD, d)
			}
			if !ok || m.DHashDist < best.DHashDist {
				best, ok = m, true
			}
		}
		a, d, hasA, hasD := s.SongHash()
		consider(a, d, hasA, hasD, "")
		for _, ch := range s.Charts {
			a, d, hasA, hasD := ch.ChartHash()
			consider(a, d, hasA, hasD, ch.Difficulty)
		}
		return best, ok
	}

	var out []JacketMatch
	for _, s := range c.Songs {
		if m, ok := pick(s); ok {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].DHashDist < out[j].DHashDist })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
