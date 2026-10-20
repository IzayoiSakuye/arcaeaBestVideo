package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Song 获取歌曲信息
func (c *Catalog) Song(songID string) (*Song, bool) {
	if songID == "" {
		return nil, false
	}

	for _, song := range c.Songs {
		if song.SongID == songID {
			return song, true
		}
	}
	return nil, false
}

// Chart 获取谱面相关信息
func (c *Catalog) Chart(songID string, difficulty string) (*Chart, bool) {
	if songID == "" {
		return nil, false
	}

	var selectSong *Song
	for _, song := range c.Songs {
		if song.SongID == songID {
			selectSong = song
		}
	}
	if selectSong == nil {
		return nil, false
	}
	for _, chart := range selectSong.Charts {
		if chart.Difficulty == difficulty {
			return chart, true
		}
	}
	return nil, false
}

// Load 读取并校验目录文件
func Load(path string, supportedSchema int) (*Catalog, error) {
	if path == "" {
		return nil, fmt.Errorf("catalog: path required")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("catalog: unable to read %s: %w", path, err)
	}
	var catalog Catalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		return nil, fmt.Errorf("catalog: unable to unmarshal %s: %w", path, err)
	}

	if catalog.SchemaVersion != supportedSchema {
		return nil, fmt.Errorf("catalog: data version is %d, but the app version supported %d", catalog.SchemaVersion, supportedSchema)
	}

	if catalog.Songs == nil || len(catalog.Songs) == 0 {
		return nil, fmt.Errorf("catalog: no songs found")
	}
	for _, song := range catalog.Songs {
		err := validateSongInfo(song)
		if err != nil {
			return nil, fmt.Errorf("catalog: invalid song info: %w", err)
		}
	}
	return &catalog, nil
}

// JacketPath 用于快速获取歌曲封面的本地路径
func (c *Catalog) JacketPath(SongID, diff string) (string, error) {
	if SongID == "" {
		return "", fmt.Errorf("catalog: invalid song id")
	}
	song, ok := c.Song(SongID)
	if !ok {
		return "", fmt.Errorf("catalog: unable to find song %s (%s)", SongID, diff)
	}
	if ch, ok := c.Chart(SongID, diff); ok && usableJacket(ch.Jacket) {
		return *ch.Jacket.Local256, nil
	}
	if usableJacket(song.Jacket) {
		return *song.Jacket.Local256, nil
	}
	return "", fmt.Errorf("catalog: unable to find jacket for song %s (%s)", SongID, diff)
}

// usableJacket 判断该封面是否可以使用
func usableJacket(j *Jacket) bool {
	if j != nil && j.Bundled && j.Local256 != nil && *j.Local256 != "" {
		return true
	}
	return false
}

// validateSongInfo 校验一条曲目记录是否可用
func validateSongInfo(song *Song) error {
	if song == nil {
		return errors.New("entry is nil")
	}
	if song.TitleNorm == "" {
		return errors.New("title_norm is empty")
	}
	if song.TitleEn == "" && song.TitleJP == "" {
		return errors.New("both title_en and title_jp are empty")
	}
	if song.Jacket == nil {
		return errors.New("jacket is nil")
	}
	if song.Charts == nil || len(song.Charts) == 0 {
		return errors.New("charts is empty")
	}
	return nil
}
