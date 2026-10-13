// Package ocr 把 Yurisaki 的 b50 成绩图解析成成绩数据
package ocr

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// 版式常量， 手量
const (
	Cols, Rows = 5, 10
	CellY0     = 440
	RowPitch   = 125
	CellW      = 340
	CellH      = 100
	JacketPadX = 5
	JacketPadY = 5
	JacketSize = 90
	TextX0     = 98
)

// colX 是 5 列的左边界
var colX = [Cols]int{37, 413, 790, 1167, 1543}

const (
	SheetW, SheetH = 1920, 1750
)

// CheckSheet 检查图是不是标准尺寸
func CheckSheet(b image.Rectangle) error {
	if b.Dx() != SheetW || b.Dy() != SheetH {
		return fmt.Errorf("图的尺寸是 %d×%d，需要 %d×%d 尺寸的图，请重新生成一张",
			b.Dx(), b.Dy(), SheetW, SheetH)
	}
	return nil
}

// Cell 是一个格子在原图里的位置, Index 从 1 到 50，先左到右再上到下。
type Cell struct {
	Index int
	Box   image.Rectangle
}

// 格子内部四行文字的纵向范围
var (
	LineTitle     = [2]int{9, 26}  // 曲名 + #rank
	LineScore     = [2]int{31, 53} // 分数
	LinePotential = [2]int{61, 74} // Potential <定数> > <单曲ptt> <通关类型>
	LineJudgment  = [2]int{79, 93} // P: n (+n) F: n L: n
)

// LineBox 取某一行的区域
func LineBox(c Cell, r [2]int) image.Rectangle {
	return image.Rect(
		c.Box.Min.X+TextX0, c.Box.Min.Y+r[0],
		c.Box.Max.X, c.Box.Min.Y+r[1],
	)
}

// Grid 返回 50 个格子的位置
func Grid() []Cell {
	cells := make([]Cell, 0, Cols*Rows)
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			x, y := colX[c], CellY0+r*RowPitch
			cells = append(cells, Cell{
				Index: r*Cols + c + 1,
				Box:   image.Rect(x, y, x+CellW, y+CellH),
			})
		}
	}
	return cells
}

// JacketBox 是格子里的曲绘缩略图，后面用它算指纹定曲目
func JacketBox(c Cell) image.Rectangle {
	x, y := c.Box.Min.X+JacketPadX, c.Box.Min.Y+JacketPadY
	return image.Rect(x, y, x+JacketSize, y+JacketSize)
}

// Load 读一张图
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("解码 %s 失败: %w", path, err)
	}
	if img.Bounds().Dx() == 0 {
		return nil, fmt.Errorf("%s 是空的（格式 %s）", path, format)
	}
	return img, nil
}

// Crop 裁一块出来
func Crop(img image.Image, r image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.Set(x-r.Min.X, y-r.Min.Y, img.At(x, y))
		}
	}
	return dst
}
