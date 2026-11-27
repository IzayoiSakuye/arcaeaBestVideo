package resource

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"rhythm-game-bvideo/internal/render"
	"rhythm-game-bvideo/internal/score"
)

// VideoMaterial 只读取已确认且就绪的素材，生成时不再联网补下载。
func VideoMaterial(ctx context.Context, ffprobe, dir string, song score.Song) (string, int, error) {
	if !score.ClipConfirmed(song) {
		return "", 0, fmt.Errorf("片段尚未保存，或来源及范围已改变，请重新确认")
	}
	start := song.Clip.StartMs
	path := song.Video.LocalPath
	if path != "" {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return "", 0, err
		}
	} else {
		link, err := VideoURL(song.Video.URL)
		if err != nil {
			return "", 0, err
		}
		if strings.HasPrefix(link, "https://www.bilibili.com/") {
			path = RangeCachePath(dir, song.SongID, song.Difficulty.Name, link, start, song.Clip.DurationMs)
			if !ValidRange(ctx, ffprobe, path, link, start, song.Clip.DurationMs) {
				return "", 0, fmt.Errorf("已确认的 B站区间素材尚未就绪，请准备或重试下载")
			}
			return path, 0, nil
		}
		path = VideoCachePath(dir, song.SongID, song.Difficulty.Name, link)
	}
	info, err := render.Probe(ctx, ffprobe, path)
	if err != nil || info.Width <= 0 || info.Height <= 0 || info.SampleRate == "" || start > info.DurationMs || song.Clip.DurationMs > info.DurationMs-start+50 {
		return "", 0, fmt.Errorf("录像未就绪或已确认区间超出录像时长")
	}
	return path, start, nil
}
