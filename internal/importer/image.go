// Package importer 把各种来路的成绩变成 score.Project
package importer

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"sync"
	"time"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/ocr"
	"rhythm-game-bvideo/internal/score"
)

// jacketWorkers 处理曲绘的并行数
const jacketWorkers = 4

// JacketMarginOK 第一名比第二名近多少位才算认准。
const JacketMarginOK = 3

// checksumTolerance 校验预期内误差
const checksumTolerance = 0.003

// ImageProvider 把生成的 b50 读成 Project
type ImageProvider struct {
	Cata    *catalog.Catalog
	FFmpeg  string
	OCRRoot string
}

func (p *ImageProvider) Name() string { return "b50 成绩图" }

// Import 读取一张图的信息
func (p *ImageProvider) Import(ctx context.Context, in score.Input) (*score.Draft, error) {
	switch {
	case in.Path == "":
		return nil, fmt.Errorf("需要成绩图路径")
	case p.Cata == nil:
		return nil, fmt.Errorf("缺少曲库信息")
	case p.FFmpeg == "":
		return nil, fmt.Errorf("缺少 ffmpeg")
	}
	img, err := ocr.Load(in.Path)
	if err != nil {
		return nil, fmt.Errorf("导入成绩图失败：%w", err)
	}
	if err := ocr.CheckSheet(img.Bounds()); err != nil {
		return nil, fmt.Errorf("导入成绩图失败：%w", err)
	}

	cells := ocr.Grid()
	reader, err := ocr.FindSheetReader(p.OCRRoot)
	if err != nil {
		return nil, fmt.Errorf("导入成绩图失败：%w", err)
	}
	text, err := reader.ReadSheet(ctx, in.Path, cells)
	if err != nil {
		return nil, fmt.Errorf("导入成绩图失败：%w", err)
	}
	hits, err := p.matchJackets(ctx, img, cells)
	if err != nil {
		return nil, err
	}
	header := text.Header
	proj := &score.Project{
		SchemaVersion: score.SchemaVersion,
		Game:          "arcaea",
		Region:        "intl",
		Player:        playerOf(header),
		SnapshotAt:    time.Now(),
		Source:        score.Source{Type: score.KindImage, Provider: "yurisaki-ocr"},
		Songs:         make([]score.Song, len(cells)),
	}
	draft := &score.Draft{Project: proj}
	draft.Notes = append(draft.Notes, header.Problems...) // 表头没读出来的字段

	confident := 0
	for i, cell := range cells {
		f := text.Fields[i]
		hit, note, ok := pickSong(p.Cata, hits[i], text.Titles[i])
		if note != "" {
			draft.Notes = append(draft.Notes, fmt.Sprintf("#%d %s", cell.Index, note))
		}
		songID := ""
		if hit != nil {
			songID = hit.SongID
		}

		s := score.Song{
			Index:  cell.Index,
			SongID: songID,
			Score: score.ScoreRef{
				Achievement:     float64(f.Score) / 100000,
				ClearType:       f.ClearType,
				Pure:            f.Pure,
				ShinyPerfect:    f.ShinyPerfect,
				Far:             f.Far,
				Lost:            f.Lost,
				UnreadJudgments: f.UnreadJudgments,
			},
		}
		// 难度代号图上没有，留空让 Validate 按定数反查
		s.Difficulty.Constant = f.Constant
		if f.PTT > 0 {
			ptt := f.PTT
			s.Score.PTT = &ptt
		}
		proj.Songs[i] = s

		for _, problem := range f.Problems {
			draft.Notes = append(draft.Notes, fmt.Sprintf("#%d %s", cell.Index, problem))
		}
		if ok {
			confident++
		}
	}
	proj.Source.Confidence = float64(confident) / float64(len(cells))
	draft.Notes = append(draft.Notes, checksums(proj.Songs, header)...)
	return draft, nil
}

// pickSong 在曲绘不确定时用唯一匹配的 OCR 曲名纠正
func pickSong(cata *catalog.Catalog, hits []catalog.JacketMatch, title string) (*catalog.Song, string, bool) {
	var titleSong *catalog.Song
	titleMatches := 0
	key := catalog.NormalizeKey(title)
	if key != "" {
		for _, song := range cata.Songs {
			for _, candidate := range song.MatchKeys {
				if candidate == key {
					titleMatches++
					titleSong = song
					break
				}
			}
		}
	}
	if titleMatches != 1 {
		titleSong = nil
	}
	if len(hits) == 0 {
		if titleSong != nil {
			return titleSong, "曲绘未匹配，已按图中文字匹配曲目，请校对", false
		}
		return nil, "曲绘和图中文字均未匹配曲目，请手动选择", false
	}
	if margin(hits) <= JacketMarginOK {
		if titleSong != nil {
			for _, candidate := range hits {
				if candidate.Song.SongID == titleSong.SongID {
					return titleSong, "曲绘候选接近，已按图中文字匹配曲目，请校对", false
				}
			}
		}
		return hits[0].Song, fmt.Sprintf("曲绘无法准确识别：%s 和 %s 的指纹接近；图中文字为「%s」，请校对", hits[0].Song.SongID, hits[1].Song.SongID, title), false
	}
	if titleSong != nil && titleSong.SongID != hits[0].Song.SongID {
		return hits[0].Song, fmt.Sprintf("曲绘匹配 %s，图中文字匹配 %s，请校对曲目", hits[0].Song.SongID, titleSong.SongID), false
	}
	return hits[0].Song, "", true
}

// playerOf 取玩家 PTT。图上印 5 位小数、游戏里印 3 位，所以取 3 位
func playerOf(h ocr.Header) score.Player {
	return score.Player{
		Rating:      math.Round(h.MaxPotential*1000) / 1000,
		RatingLabel: "Potential",
	}
}

// checksums 拿表头印的平均值反验 50 行 ptt
func checksums(songs []score.Song, h ocr.Header) []string {
	// 图上按 ptt 降序排，前 10 行就是 b10
	sum50, sum10 := 0.0, 0.0
	for i, s := range songs {
		if s.Score.PTT == nil {
			continue
		}
		sum50 += *s.Score.PTT
		if i < 10 {
			sum10 += *s.Score.PTT
		}
	}

	var notes []string
	if h.Top50Avg > 0 {
		if d := sum50 - h.Top50Avg*float64(len(songs)); math.Abs(d) > checksumTolerance {
			notes = append(notes, fmt.Sprintf(
				"表头校验和有误：%d 行 ptt 之和 %.4f，图上 TOP50 平均 ×%d = %.4f（差 %+.4f）",
				len(songs), sum50, len(songs), h.Top50Avg*float64(len(songs)), d))
		}
	}
	if h.Top10Avg > 0 && len(songs) >= 10 {
		if d := sum10 - h.Top10Avg*10; math.Abs(d) > checksumTolerance {
			notes = append(notes, fmt.Sprintf(
				"表头校验和有误：前 10 行 ptt 之和 %.4f，图上 TOP10 平均 ×10 = %.4f（差 %+.4f）",
				sum10, h.Top10Avg*10, d))
		}
	}
	return notes
}

// matchJackets 逐格裁出曲绘算指纹，并调用ffmpeg。
func (p *ImageProvider) matchJackets(ctx context.Context, img image.Image, cells []ocr.Cell) ([][]catalog.JacketMatch, error) {
	hits := make([][]catalog.JacketMatch, len(cells))
	errs := make([]error, len(cells))

	var wg sync.WaitGroup
	slot := make(chan struct{}, jacketWorkers)
	for i := range cells {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			slot <- struct{}{}
			defer func() { <-slot }()
			hits[i], errs[i] = p.matchJacket(ctx, img, cells[i])
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("第 %d 首成绩图识别失败：%w", cells[i].Index, err)
		}
	}
	return hits, nil
}

// margin 算出第一名比第二名近多少位
func margin(hits []catalog.JacketMatch) int {
	if len(hits) < 2 {
		return 64
	}
	return hits[1].DHashDist - hits[0].DHashDist
}

// matchJacket 裁一格曲绘算指纹找曲目
func (p *ImageProvider) matchJacket(ctx context.Context, img image.Image, cell ocr.Cell) ([]catalog.JacketMatch, error) {
	f, err := os.CreateTemp("", "b50-jacket-*.png")
	if err != nil {
		return nil, fmt.Errorf("建临时文件失败: %w", err)
	}
	path := f.Name()
	defer os.Remove(path)

	if err := png.Encode(f, ocr.Crop(img, ocr.JacketBox(cell))); err != nil {
		f.Close()
		return nil, fmt.Errorf("写曲绘裁片失败: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("写曲绘裁片失败: %w", err)
	}

	a, err := catalog.AHash(ctx, p.FFmpeg, path)
	if err != nil {
		return nil, err
	}
	d, err := catalog.DHash(ctx, p.FFmpeg, path)
	if err != nil {
		return nil, err
	}
	top := p.Cata.MatchJacket(a, d, 2)
	return top, nil
}
