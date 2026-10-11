package catalog

// Catalog 是一个视频用到的歌曲目录
type Catalog struct {
	SchemaVersion int     `json:"schema_version"`
	Game          string  `json:"game"`
	GeneratedAt   string  `json:"generated_at"`
	Songs         []*Song `json:"songs"`
}

// Song 是某一首歌的具体信息
type Song struct {
	SongID string `json:"song_id"`

	TitleEn     string   `json:"title_en"` // 标题显示，和给内部看的键
	TitleJP     string   `json:"title_jp"`
	TitleNorm   string   `json:"title_norm"`
	Artist      string   `json:"artist"`
	Set         string   `json:"set"` // 曲包显示，和给内部看的键
	SetFriendly string   `json:"set_friendly"`
	Version     string   `json:"version"`  // 歌曲版本
	TimeSec     *int     `json:"time_sec"` // 歌曲时长
	Jacket      *Jacket  `json:"jacket"`
	PHash       *PHash   `json:"phash"`
	BG          string   `json:"bg"`
	Charts      []*Chart `json:"charts"`
	MatchKeys   []string `json:"match_keys"`
}

// Jacket (应名为Cover, 这里是AI爬数据时命名错误, 懒得改了) 是某一首歌对应的曲绘信息
type Jacket struct {
	//图在不在本地
	Bundled bool `json:"bundled"`
	// 本地缩略图路径
	Local256 *string `json:"local_256"`
	// 原图像素
	Native *string `json:"native"`
	// 远程曲绘来源
	ThumbURL256 *string `json:"thumb_url_256"`
	FullURL     *string `json:"full_url"`
}

// Chart 是歌曲对应谱面的信息
type Chart struct {
	Difficulty string  `json:"difficulty"`
	Level      *string `json:"level"`
	// 定数
	Constant       *float64 `json:"constant"`
	ConstantSource *string  `json:"constant_source"`
	Video          *Video   `json:"video"`
	Jacket         *Jacket  `json:"jacket"`
	Notes          *int     `json:"note"` // 物量：数据里的键是单数 note，写成 notes 会永远解出 nil
}

// Video 是歌曲对应的视频信息表，如果有多个视频来源会填入slice
type Video struct {
	Sources []VideoSource `json:"sources"`
}

// PHash 是曲绘对应的哈希信息，用于OCR匹配
type PHash struct {
	AHash *string `json:"ahash"`
	DHash *string `json:"dhash"`
}

// VideoSource 是单个视频源信息
type VideoSource struct {
	BVID        *string  `json:"bvid"`
	P           int      `json:"p"`
	URL         string   `json:"url"`
	DurationSec int      `json:"duration_sec"`
	PartTitle   string   `json:"part_title"`
	Collections []string `json:"collections"`
	VideoWidth  *int     `json:"video_width"`
	VideoHeight *int     `json:"video_height"`
	OwnerName   string   `json:"owner_name"`
}
