// Package ocr 把 Yurisaki 的 b50 成绩图解析成成绩数据。
// 只支持这一种图源，所以坐标是量死的，不做版式识别。
package ocr

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

// 版式常量，按亮度剖面量出来的。
const (
	Cols, Rows = 5, 10
	ColPitch   = 376
	RowPitch   = 125
	CellX0     = 37
	CellY0     = 440
	CellW      = 340
	CellH      = 100
	JacketPad  = 3
	JacketSize = 94
)

// Cell 是一个格子在原图里的位置。Index 从 1 到 50，先左到右再上到下。
type Cell struct {
	Index int
	Box   image.Rectangle
}

// Grid 返回 50 个格子的位置
func Grid() []Cell {
	cells := make([]Cell, 0, Cols*Rows)
	for r := 0; r < Rows; r++ {
		for c := 0; c < Cols; c++ {
			x, y := CellX0+c*ColPitch, CellY0+r*RowPitch
			cells = append(cells, Cell{
				Index: r*Cols + c + 1,
				Box:   image.Rect(x, y, x+CellW, y+CellH),
			})
		}
	}
	return cells
}

// JacketBox 是格子里的曲绘缩略图（左边那个方块），后面用它算指纹定曲目
func JacketBox(c Cell) image.Rectangle {
	x, y := c.Box.Min.X+JacketPad, c.Box.Min.Y+JacketPad
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
