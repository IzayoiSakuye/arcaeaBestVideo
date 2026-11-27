package score

import "testing"

func TestQAConfirmationBindsSongDifficultySourceAndRange(t *testing.T) {
	base := Song{SongID: "qa", Difficulty: DiffRef{Name: "FTR"}, Video: &VideoRef{URL: "https://www.bilibili.com/video/BV1op4y1W7M2?p=1"}, Clip: &ClipRange{StartMs: 60123, DurationMs: 2000}}
	base.Clip.ConfirmedKey = ClipKey(base)
	if !ClipConfirmed(base) {
		t.Fatal("确认结果不能用于原歌曲")
	}
	for _, field := range []string{"song", "difficulty", "source", "start", "duration", "legacy"} {
		t.Run(field, func(t *testing.T) {
			s := base
			v, c := *base.Video, *base.Clip
			s.Video, s.Clip = &v, &c
			switch field {
			case "song":
				s.SongID = "other"
			case "difficulty":
				s.Difficulty.Name = "BYD"
			case "source":
				s.Video.URL = "https://www.bilibili.com/video/BV1op4y1W7M2?p=2"
			case "start":
				s.Clip.StartMs++
			case "duration":
				s.Clip.DurationMs++
			case "legacy":
				s.Clip.ConfirmedKey = ""
			}
			if ClipConfirmed(s) {
				t.Fatal("修改来源或范围后错误保留确认")
			}
		})
	}
}
