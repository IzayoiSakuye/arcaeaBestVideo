package catalog

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"rhythm-game-bvideo/internal/tools"
)

func ffmpegForTest(t *testing.T) string {
	t.Helper()
	if ff := tools.Detect(tools.Tools{}).Tools.FFmpeg; ff != "" {
		return ff
	}
	t.Skip("没找到 ffmpeg")
	return ""
}

func jacketFile(s *Song) (string, bool) {
	if s.Jacket == nil || s.Jacket.Local256 == nil {
		return "", false
	}
	return filepath.Join("..", "..", filepath.FromSlash(*s.Jacket.Local256)), true
}

// 指纹必须与构建期逐比特一致 —— 两边都是 ffmpeg 缩放成灰度 + 同样的位序。
// 这条同时盯住 grayPixels 的参数和 aHashBits/dHashBits 的位序。
func TestHashMatchesStored(t *testing.T) {
	c := loadReal(t)
	ff := ffmpegForTest(t)
	ctx := context.Background()

	checked, badA, badD := 0, 0, 0
	for i, s := range c.Songs {
		if i%5 != 0 { // 全量要几十秒，抽样够用
			continue
		}
		path, ok := jacketFile(s)
		if !ok || s.PHash == nil {
			continue
		}

		if s.PHash.AHash != nil {
			want, err := strconv.ParseUint(*s.PHash.AHash, 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			got, err := AHash(ctx, ff, path)
			if err != nil {
				t.Fatal(err)
			}
			checked++
			if got != want {
				badA++
				t.Errorf("%s AHash：得 %016x，曲库存 %016x（差 %d 位）",
					s.SongID, got, want, Hamming(got, want))
			}
		}
		if s.PHash.DHash != nil {
			want, err := strconv.ParseUint(*s.PHash.DHash, 16, 64)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DHash(ctx, ff, path)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				badD++
				t.Errorf("%s DHash：得 %016x，曲库存 %016x", s.SongID, got, want)
			}
		}
	}
	if checked == 0 {
		t.Skip("没有可比的曲绘")
	}
	t.Logf("抽了 %d 首：AHash 不符 %d，DHash 不符 %d", checked, badA, badD)
}

// 生产场景：b50 图里格子只有 ~48px，缩过一次还得认出自己。
// 真实输入还多一次整图 JPEG 压缩，实际距离只会比这里略大。
func TestMatchJacketRecognizesSelf(t *testing.T) {
	c := loadReal(t)
	ff := ffmpegForTest(t)

	checked, notBest, top1Miss, maxSelf := 0, 0, 0, 0
	var worst string
	for i, s := range c.Songs {
		if i%5 != 0 {
			continue
		}
		self, ok := s.AHashBits()
		path, ok2 := jacketFile(s)
		if !ok || !ok2 {
			continue
		}
		px, err := grayPixels(context.Background(), ff, path, "scale=48:48,scale=8:8", 64)
		if err != nil {
			t.Fatalf("%s: %v", s.SongID, err)
		}
		h := aHashBits(px)
		dSelf := Hamming(h, self)
		checked++
		if dSelf > maxSelf {
			maxSelf, worst = dSelf, s.SongID
		}

		top := c.MatchJacket(h, 3)
		if len(top) == 0 || top[0].Dist != dSelf {
			notBest++
			t.Errorf("%s：自己差 %d 位却不是最优，最近的是 %s(%d)",
				s.SongID, dSelf, top[0].Song.SongID, top[0].Dist)
		}
		if top[0].Song.SongID != s.SongID {
			top1Miss++ // 并列时会这样，不算失败
		}
	}
	if checked == 0 {
		t.Skip("没有可用的曲绘")
	}
	t.Logf("抽了 %d 首：自己不在最优 %d 首；top1 不是自己 %d 首（并列）；自比最大 %d 位（%s）",
		checked, notBest, top1Miss, maxSelf, worst)
	if maxSelf > 12 {
		t.Errorf("自比差 %d 位太大了，阈值或算法可能不对", maxSelf)
	}
}
