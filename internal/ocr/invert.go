package ocr

import (
	"image"
	"image/color"
)

// invert 把浅字深底翻成深字浅底，方便文字识别代码
type invert struct{ img image.Image }

func lightOnDark(img image.Image) image.Image { return invert{img} }

func (v invert) ColorModel() color.Model { return color.RGBA64Model }

func (v invert) Bounds() image.Rectangle { return v.img.Bounds() }

func (v invert) At(x, y int) color.Color {
	r, g, b, a := v.img.At(x, y).RGBA()
	return color.RGBA64{
		R: 65535 - uint16(r),
		G: 65535 - uint16(g),
		B: 65535 - uint16(b),
		A: uint16(a),
	}
}
