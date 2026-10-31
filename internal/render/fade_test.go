package render

import (
	"fmt"
	"strings"
	"testing"
)

func TestVideoFadeTiming(t *testing.T) {
	for _, tc := range []struct{ duration, fade int }{{10000, 1000}, {2000, 500}} {
		got := videoFade(tc.duration)
		if !strings.Contains(got, "fade=t=in:st=0:d="+fmt.Sprintf("%.3f", float64(tc.fade)/1000)) ||
			!strings.Contains(got, "fade=t=out:st="+fmt.Sprintf("%.3f", float64(tc.duration-tc.fade)/1000)) {
			t.Fatalf("%d ms 淡化区间错误：%s", tc.duration, got)
		}
	}
	args := CompositeArgs(Layers{}, "out.mp4")
	if !strings.Contains(strings.Join(args, " "), videoFade(SecPerSong*1000)) {
		t.Fatal("歌曲合成未应用视频淡化")
	}
}
