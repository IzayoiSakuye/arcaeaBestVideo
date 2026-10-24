package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"rhythm-game-bvideo/internal/catalog"
	"rhythm-game-bvideo/internal/tools"
)

const (
	sampleImage = "../../dist/b50-sample.jpg"
	fixturePath = "../score/testdata/b50-from-image.json"
)

// 不依赖真实图：格子该有 50 个、不重叠、按序排列
func TestGridShape(t *testing.T) {
	cells := Grid()
	if len(cells) != Cols*Rows {
		t.Fatalf("格子数 %d，期望 %d", len(cells), Cols*Rows)
	}
	for i, c := range cells {
		if c.Index != i+1 {
			t.Errorf("第 %d 个格子的 Index 是 %d", i, c.Index)
		}
		if c.Box.Dx() != CellW || c.Box.Dy() != CellH {
			t.Errorf("#%d 尺寸不对：%v", c.Index, c.Box)
		}
		if !JacketBox(c).In(c.Box) {
			t.Errorf("#%d 曲绘框 %v 超出格子 %v", c.Index, JacketBox(c), c.Box)
		}
	}
	if cells[5].Box.Min.X != cells[0].Box.Min.X || cells[5].Box.Min.Y <= cells[0].Box.Min.Y {
		t.Errorf("换行不对：#1=%v #6=%v", cells[0].Box, cells[5].Box)
	}
}

// 导出一张画了框的调试图，用眼睛确认套没套准
func TestWriteOverlayForEyeball(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skipf("没有样例图：%v", err)
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	for _, c := range Grid() {
		stroke(rgba, c.Box, color.RGBA{255, 0, 0, 255}, 2)
		stroke(rgba, JacketBox(c), color.RGBA{0, 160, 255, 255}, 2)
	}
	out := filepath.Join("..", "..", ".cache", "grid-debug.png")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, out, rgba)
	t.Logf("调试图：%s", out)
}

// 难度代号图上没有，靠 (song_id, 定数) 反查曲库。这条量的是图上这 50 行里
// 定数够不够唯一 —— 不唯一的行只能靠判定和（P+F+L 撞物量）消歧，而判定行还没读。
// 曲名用夹具的标题代替 pHash 的结果，所以这是"最好情况"下的上界。
func TestConstantResolvesDifficulty(t *testing.T) {
	rows := fixtureRows(t)
	cata, err := catalog.Load(filepath.Join("..", "..", "data", "arcaea-catalog.json"), 1)
	if err != nil {
		t.Skipf("没有曲库：%v", err)
	}

	uniq, dup, miss := 0, 0, 0
	for _, r := range rows {
		song, _, ok := cata.Find(r.Title)
		if !ok {
			if cands := cata.Candidates(r.Title, 1); len(cands) > 0 {
				song = cands[0].Song
			} else {
				miss++
				t.Logf("#%d 标题 %q 查不到曲目", r.Index, r.Title)
				continue
			}
		}
		var hits []string
		for _, c := range song.Charts {
			if c.Constant != nil && *c.Constant == r.Constant {
				hits = append(hits, fmt.Sprintf("%s(%s)", c.Difficulty, levelOf(c)))
			}
		}
		switch len(hits) {
		case 1:
			uniq++
		case 0:
			miss++
			t.Logf("#%d %s 定数 %v 在曲库里一个都对不上", r.Index, song.SongID, r.Constant)
		default:
			dup++
			t.Logf("#%d %s 定数 %v 命中多个难度：%v", r.Index, song.SongID, r.Constant, hits)
		}
	}
	t.Logf("(曲目, 定数) 唯一定位难度：%d/50（多义 %d，对不上 %d）", uniq, dup, miss)
	if uniq < 45 {
		t.Errorf("只有 %d/50 能唯一定位，判定行不读的话这些行会直接 fatal", uniq)
	}
}

func levelOf(c *catalog.Chart) string {
	if c.Level == nil {
		return "-"
	}
	return *c.Level
}

// 真值来自夹具：图上那 50 首是抄录并验证过的。
// 这条同时验坐标系（切得准不准）和曲绘指纹（认不认得出歌）。
func TestJacketCropIdentifiesSong(t *testing.T) {
	img, err := Load(sampleImage)
	if err != nil {
		t.Skipf("没有样例图：%v", err)
	}
	titles := fixtureTitles(t)
	cata, err := catalog.Load(filepath.Join("..", "..", "data", "arcaea-catalog.json"), 1)
	if err != nil {
		t.Skipf("没有曲库：%v", err)
	}
	ff := tools.Detect(tools.Tools{}).Tools.FFmpeg
	if ff == "" {
		t.Skip("没找到 ffmpeg")
	}

	dir := t.TempDir()
	hit := 0
	for _, c := range Grid() {
		want, _, ok := cata.Find(titles[c.Index-1])
		if !ok {
			// 图上长曲名是截断的，夹具里的标题是补全的 —— 查不到就用候选兜
			cands := cata.Candidates(titles[c.Index-1], 1)
			if len(cands) == 0 {
				t.Errorf("#%d 标题 %q 在曲库里找不到", c.Index, titles[c.Index-1])
				continue
			}
			want = cands[0].Song
		}

		path := filepath.Join(dir, "jacket.png")
		writePNG(t, path, Crop(img, JacketBox(c)))
		hA, err := catalog.AHash(context.Background(), ff, path)
		if err != nil {
			t.Fatal(err)
		}
		hD, err := catalog.DHash(context.Background(), ff, path)
		if err != nil {
			t.Fatal(err)
		}

		top := cata.MatchJacket(hA, hD, 1)
		if len(top) == 0 {
			t.Errorf("#%d 没有候选", c.Index)
			continue
		}
		if top[0].Song.SongID == want.SongID {
			hit++
		} else {
			t.Logf("未命中 #%d %q：认成 %s（dHash 差 %d 位）",
				c.Index, titles[c.Index-1], top[0].Song.SongID, top[0].DHashDist)
		}
	}
	// 用 dHash + 同时比默认/BYD 封面，实测 49/50；未命中的那个是形似撞车，
	// 后面靠"定数 + 判定数"消歧。下限防坐标或指纹回归。
	t.Logf("曲绘定曲目：命中 %d/50", hit)
	if hit < 45 {
		t.Errorf("命中 %d/50，明显低于已知的 49 —— 坐标或指纹可能坏了", hit)
	}
}

// fixtureRow 只取曲绘匹配和定数反查所需的真值。
type fixtureRow struct {
	Index    int     `json:"index"`
	Title    string  `json:"title"`
	Constant float64 `json:"constant"`
}

func fixtureRows(t *testing.T) []fixtureRow {
	t.Helper()
	b, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Skipf("没有夹具：%v", err)
	}
	var f struct {
		Songs []fixtureRow `json:"songs"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Songs) != Rows*Cols {
		t.Fatalf("夹具里 %d 行，期望 %d", len(f.Songs), Rows*Cols)
	}
	return f.Songs
}

func fixtureTitles(t *testing.T) []string {
	rows := fixtureRows(t)
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Title)
	}
	return out
}

func stroke(dst *image.RGBA, r image.Rectangle, c color.RGBA, w int) {
	for x := r.Min.X; x < r.Max.X; x++ {
		for i := 0; i < w; i++ {
			dst.SetRGBA(x, r.Min.Y+i, c)
			dst.SetRGBA(x, r.Max.Y-1-i, c)
		}
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for i := 0; i < w; i++ {
			dst.SetRGBA(r.Min.X+i, y, c)
			dst.SetRGBA(r.Max.X-1-i, y, c)
		}
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
