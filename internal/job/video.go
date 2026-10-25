package job

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

func (j *Job) videoClip(ctx context.Context, song score.Song, chart *catalog.Chart) (string, error) {
	if song.Video != nil && song.Video.LocalPath != "" {
		path, err := filepath.Abs(song.Video.LocalPath)
		if err != nil {
			return "", err
		}
		if err := resource.ValidVideo(ctx, j.deps.Tools.FFprobe, path); err != nil {
			return "", err
		}
		return path, nil
	}
	var urls []string
	if song.Video != nil && song.Video.URL != "" {
		urls = append(urls, song.Video.URL)
	} else if chart.Video != nil {
		for _, source := range chart.Video.Sources {
			urls = append(urls, source.URL)
		}
	}
	var failures []string
	if song.Video == nil {
		for _, link := range urls {
			path := resource.VideoCachePath(j.cfg.ClipsDir, song.SongID, song.Difficulty.Name, link)
			if resource.ValidVideo(ctx, j.deps.Tools.FFprobe, path) == nil {
				return path, nil
			}
		}
		if legacy, err := resource.ClipPath(j.cfg.ClipsDir, song.SongID, song.Difficulty.Name); err == nil && resource.ValidVideo(ctx, j.deps.Tools.FFprobe, legacy) == nil {
			return legacy, nil
		}
	}
	for _, link := range urls {
		path, _, err := resource.DownloadVideo(ctx, j.deps.Tools.YtDlp, j.deps.Tools.FFmpeg,
			j.deps.Tools.FFprobe, j.cfg.ClipsDir, song.SongID, song.Difficulty.Name, link)
		if err == nil {
			return path, nil
		}
		failures = append(failures, err.Error())
	}
	if len(urls) == 0 {
		return "", fmt.Errorf("%s %s：缺少录像来源，请在识别信息页指定链接或本地文件", song.SongID, song.Difficulty.Name)
	}
	return "", fmt.Errorf("%s %s：录像获取失败：%s", song.SongID, song.Difficulty.Name, strings.Join(failures, "；"))
}
