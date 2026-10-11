package ocr

import "image"

// InkColumns 查看一行文字在哪些列
func InkColumns(img image.Image, r image.Rectangle) []bool {
	out := make([]bool, r.Dx())
	for x := r.Min.X; x < r.Max.X; x++ {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			p := img.At(x, y)
			rr, g, b, _ := p.RGBA()
			if (rr*299+g*587+b*114)/1000/257 < 150 {
				out[x-r.Min.X] = true
				break
			}
		}
	}
	return out
}

// Groups 把有墨的列按空隙切成若干段，返回每段的 x 范围（相对图），
// 空隙小于 minGap 的算同一段。段内的字再用更小的 minGap 切。
func Groups(cols []bool, x0, minGap int) [][2]int {
	var out [][2]int
	start := -1
	gap := 0
	for i, ink := range cols {
		if ink {
			if start < 0 {
				start = i
			}
			gap = 0
			continue
		}
		if start < 0 {
			continue
		}
		gap++
		if gap >= minGap {
			out = append(out, [2]int{x0 + start, x0 + i - gap + 1})
			start = -1
			gap = 0
		}
	}
	if start >= 0 {
		out = append(out, [2]int{x0 + start, x0 + len(cols)})
	}
	return out
}
