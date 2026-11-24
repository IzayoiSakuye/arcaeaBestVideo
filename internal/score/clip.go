package score

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
)

// ClipKey 绑定实际来源及范围，旧工程和修改后的范围均须重新确认。
func ClipKey(song Song) string {
	if song.Clip == nil || song.Video == nil || song.Clip.StartMs < 0 || song.Clip.DurationMs < 1000 {
		return ""
	}
	source := song.Video.URL
	if song.Video.LocalPath != "" {
		path, err := filepath.Abs(song.Video.LocalPath)
		if err != nil {
			return ""
		}
		fi, err := os.Stat(path)
		if err != nil || !fi.Mode().IsRegular() {
			return ""
		}
		source = fmt.Sprintf("local:%s:%d:%d", path, fi.Size(), fi.ModTime().UnixNano())
	}
	if source == "" || song.SongID == "" || song.Difficulty.Name == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("clip-v1\x00%s\x00%s\x00%s\x00%d\x00%d", song.SongID, song.Difficulty.Name, source, song.Clip.StartMs, song.Clip.DurationMs)))
	return fmt.Sprintf("%x", sum[:])
}

func ClipConfirmed(song Song) bool {
	key := ClipKey(song)
	return key != "" && song.Clip.ConfirmedKey == key
}
