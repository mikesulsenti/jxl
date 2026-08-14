package jxl

import "math"

var epf0Asm func(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

var epf1Asm func(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	mul, scales *float32, y, x0, x1 int)

var epf2Asm func(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

var xybSRGBAsm func(x, y, b *float32, n int, k *[29][8]float32)

var xybSRGB512Asm func(x, y, b *float32, n int, k *[29][16]float32)

var gaborishAsm func(dst, top, mid, bot *float32, n int, k *[3][8]float32)

var useIDCT8x8 bool

var useIDCTCols bool

var useTranspose8 bool

var useDequant bool

var useQuant bool

var useFDCTCols bool

var useDequant512 bool

type dequantArgs struct {
	out    [3]*float32
	coeffs [3]*int32
	mat    [3]*float32
	n      int
	p      [9]float32
}

var quantConsts = func() (k [6][8]float32) {
	fill := func(row int, bits uint32) {
		for i := range k[row] {
			k[row][i] = math.Float32frombits(bits)
		}
	}

	fill(0, 0x3F000000)
	fill(1, 0x4E800000)
	fill(2, 0x7FFFFFFF)
	fill(3, 0x00000001)
	fill(4, 0xCE800000)
	fill(5, 0xBF000000)

	return k
}()

type quantArgs struct {
	in     [3]*float32
	coeffs [3]*int32
	mat    [3]*float32
	n      int
	p      [9]float32
}

func init() { dspInit() }
