package tools

// Gcd 计算视频尺寸比例用
func Gcd(a, b int64) int64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
