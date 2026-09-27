// 按照Bg的JSON文件写的数据类型层
package resource

type BgManifest struct {
	OutputAspect string             `json:"output_aspect"`
	Bgs          map[string]BgEntry `json:"backgrounds"`
	Songs        map[string]SongBg  `json:"songs"`
}

type SongBg struct {
	Bg         string `json:"bg"`
	BgInverse  string `json:"bg_inverse"`
	OfficialID string `json:"official_id"`
	MatchedVia string `json:"matched_via"`
}

type BgEntry struct {
	Name   string            `json:"name"`
	File   string            `json:"file"`
	Native string            `json:"native"`
	Files  map[string]BgFile `json:"files"`
	Songs  []string          `json:"songs"`
}

type BgFile struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
}
