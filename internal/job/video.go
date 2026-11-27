package job

import (
	"context"
	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/resource"
	"rhythm-game-bvideo/internal/score"
)

func (j *Job) videoClip(ctx context.Context, song score.Song, chart *catalog.Chart) (string, error) {
	path, _, err := resource.VideoMaterial(ctx, j.deps.Tools.FFprobe, j.cfg.ClipsDir, song)
	return path, err
}
