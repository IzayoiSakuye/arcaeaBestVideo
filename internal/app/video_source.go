package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"rhythm-game-bvideo/internal/score"
)

func videoSourcesFile() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rhythm-game-bvideo", "video-sources.json"), nil
}

func videoSourceKey(songID, difficulty string) string {
	return songID + "\x00" + difficulty
}

func loadVideoSources() (map[string]score.VideoRef, error) {
	file, err := videoSourcesFile()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return map[string]score.VideoRef{}, nil
	}
	if err != nil {
		return nil, err
	}
	var sources map[string]score.VideoRef
	if err := json.Unmarshal(b, &sources); err != nil {
		return nil, fmt.Errorf("录像来源记录损坏：%w", err)
	}
	if sources == nil {
		sources = map[string]score.VideoRef{}
	}
	return sources, nil
}

func (a *App) validateVideoSourceKey(songID, difficulty string) error {
	cata, err := a.catalog()
	if err != nil {
		return err
	}
	if _, ok := cata.Chart(songID, difficulty); !ok {
		return fmt.Errorf("请先确定曲目和难度")
	}
	return nil
}

func (a *App) saveVideoSource(songID, difficulty string, source *score.VideoRef) error {
	// shortcut: 多实例同时写入可能互相覆盖，支持多实例编辑时改用跨进程锁
	a.mu.Lock()
	defer a.mu.Unlock()
	sources, err := loadVideoSources()
	if err != nil {
		return err
	}
	key := videoSourceKey(songID, difficulty)
	if source == nil {
		delete(sources, key)
	} else {
		sources[key] = *source
	}
	file, err := videoSourcesFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file), "video-sources-*.part")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), file)
}
