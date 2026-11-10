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
		return nil, fmt.Errorf("未提供曲库路径")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取曲库 %s 失败：%w", path, err)
	}
	var catalog Catalog
	if err := json.Unmarshal(content, &catalog); err != nil {
		return nil, fmt.Errorf("解析曲库 %s 失败：%w", path, err)
	}

	if catalog.SchemaVersion != supportedSchema {
		return nil, fmt.Errorf("曲库数据版本为 %d，应用仅支持版本 %d", catalog.SchemaVersion, supportedSchema)
	}

	if catalog.Songs == nil || len(catalog.Songs) == 0 {
		return nil, fmt.Errorf("曲库中没有歌曲")
	}
	for _, song := range catalog.Songs {
		err := validateSongInfo(song)
		if err != nil {
			return nil, fmt.Errorf("曲库歌曲信息无效：%w", err)
		}
	}
	return &catalog, nil
}

// JacketPath 用于快速获取歌曲封面的本地路径
func (c *Catalog) JacketPath(SongID, diff string) (string, error) {
	if SongID == "" {
		return "", fmt.Errorf("歌曲 ID 无效")
	}
	song, ok := c.Song(SongID)
	if !ok {
		return "", fmt.Errorf("曲库中找不到歌曲 %s（%s）", SongID, diff)
	}
	if ch, ok := c.Chart(SongID, diff); ok && usableJacket(ch.Jacket) {
		return *ch.Jacket.Local256, nil
	}
	if usableJacket(song.Jacket) {
		return *song.Jacket.Local256, nil
	}
	return "", fmt.Errorf("找不到歌曲 %s（%s）的曲绘", SongID, diff)
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
		return errors.New("歌曲记录为空")
	}
	if song.TitleNorm == "" {
		return errors.New("规范化曲名 title_norm 为空")
	}
	if song.TitleEn == "" && song.TitleJP == "" {
		return errors.New("英文曲名 title_en 和日文曲名 title_jp 均为空")
	}
	if song.Jacket == nil {
		return errors.New("曲绘信息为空")
	}
	if song.Charts == nil || len(song.Charts) == 0 {
		return errors.New("谱面列表为空")
	}
	return nil
}
