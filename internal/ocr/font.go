package ocr

// 成绩图上两处数字用的是同一字体、两种字号：
// 定数/单曲ptt 是小字（12px 高），分数是大字（21px 高），所以要两套模板。
// 模板由 glyphs_gen.go 提供，见那边的生成说明。
var (
	smallFont = NewFont(smallGlyphArt)
	bigFont   = NewFont(bigGlyphArt)
)
