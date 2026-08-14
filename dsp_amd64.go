//go:build amd64 && !noasm

package jxl

import "math"

var cbrtConsts = func() (k [7][8]float32) {
	fill := func(row int, v float32) {
		for i := range k[row] {
			k[row][i] = v
		}
	}

	fill(0, math.Float32frombits(0x548C2B4B))
	fill(1, math.Float32frombits(0x7FFFFFFF))
	fill(2, math.Float32frombits(0x7F800000))
	fill(3, math.Float32frombits(0x00800000))
	fill(4, 1<<30)
	fill(5, 1.0/1024)
	fill(6, 1)

	return k
}()

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

var hasAVX2 = cpuidAVX2()

var hasAVX512 = cpuidAVX512()

func cpuidAVX2() bool

func cpuidAVX512() bool

//go:noescape
func epf0AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

//go:noescape
func epf1AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	mul, scales *float32, y, x0, x1 int)

//go:noescape
func epf2AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

//go:noescape
func xybSRGBAVX2(x, y, b *float32, n int, k *[29][8]float32)

//go:noescape
func xybSRGB512(x, y, b *float32, n int, k *[29][16]float32)

//go:noescape
func idct8x8AVX2(data *float32, k *[31][8]float32)

//go:noescape
func gaborishAVX2(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	if !hasAVX2 {
		return
	}

	epf0Asm = epf0AVX2
	epf1Asm = epf1AVX2
	epf2Asm = epf2AVX2
	xybSRGBAsm = xybSRGBAVX2
	gaborishAsm = gaborishAVX2

	if hasAVX512 {
		xybSRGB512Asm = xybSRGB512
	}
	useIDCT8x8 = true
	useIDCTCols = true
	useTranspose8 = true
	useDequant = true
	useQuant = true
	useCbrt = true
	useFDCTCols = true
	useDequant512 = hasAVX512
}

//go:noescape
func idct8Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[31][8]float32)

//go:noescape
func idct16Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[31][8]float32)

//go:noescape
func fdct8Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[35][8]float32)

//go:noescape
func fdct16Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[35][8]float32, scale *[8]float32)

//go:noescape
func fdct32Split(src *float32, srcStride int, half *float32, k *[35][8]float32)

//go:noescape
func fdct32Merge(half *float32, dst *float32, dstStride int, k *[35][8]float32, scale *[8]float32)

func fdctCols8(n int, src []float32, srcStride int, dst []float32, dstStride int) bool {
	if !useFDCTCols {
		return false
	}

	switch n {
	case 32:
		var half [32 * 8]float32

		fdct32Split(&src[0], srcStride, &half[0], &fdctConsts)
		fdct16Cols8AVX2(&half[0], 8, &half[0], 8, &fdctConsts, &fdctConsts[fdctOne])
		fdct16Cols8AVX2(&half[16*8], 8, &half[16*8], 8, &fdctConsts, &fdctConsts[fdctOne])
		fdct32Merge(&half[0], &dst[0], dstStride, &fdctConsts, &fdctConsts[fdctInv32])
	case 8:
		fdct8Cols8AVX2(&src[0], srcStride, &dst[0], dstStride, &fdctConsts)
	case 16:
		fdct16Cols8AVX2(&src[0], srcStride, &dst[0], dstStride, &fdctConsts, &fdctConsts[fdctInv16])
	default:
		return false
	}

	return true
}

//go:noescape
func cbrtAVX2(p *float32, n int, k *[7][8]float32)

func cbrtBlock(p []float32) int {
	n := len(p) &^ 7

	if !useCbrt || n == 0 {
		return 0
	}

	cbrtAVX2(&p[0], n, &cbrtConsts)

	return n
}

//go:noescape
func quantizeAVX2(a *quantArgs, k *[6][8]float32)

func quantize(a *quantArgs) bool {
	if !useQuant {
		return false
	}

	quantizeAVX2(a, &quantConsts)

	return true
}

//go:noescape
func dequantAVX2(a *dequantArgs)

//go:noescape
func dequant512(a *dequantArgs)

func dequant(a *dequantArgs) bool {
	if !useDequant {
		return false
	}

	if useDequant512 && a.n&15 == 0 {
		dequant512(a)

		return true
	}

	dequantAVX2(a)

	return true
}

//go:noescape
func idct32PreAVX2(src *float32, srcStride int, tmp *float32, k *[31][8]float32)

//go:noescape
func idct32PostAVX2(tmp *float32, dst *float32, dstStride int, k *[31][8]float32)

//go:noescape
func transpose8AVX2(src *float32, srcStride int, dst *float32, dstStride int)

func transpose8(src []float32, srcStride int, dst []float32, dstStride int) {
	transpose8AVX2(&src[0], srcStride, &dst[0], dstStride)
}

func idctCols8(n int, src []float32, srcStride int, dst []float32, dstStride int, tmp []float32) bool {
	if !useIDCTCols {
		return false
	}

	switch n {
	case 8:
		idct8Cols8AVX2(&src[0], srcStride, &dst[0], dstStride, &idctConsts)
	case 16:
		idct16Cols8AVX2(&src[0], srcStride, &dst[0], dstStride, &idctConsts)
	case 32:
		if len(tmp) < 320 {
			return false
		}

		idct32PreAVX2(&src[0], srcStride, &tmp[0], &idctConsts)
		idct16Cols8AVX2(&tmp[0], 8, &tmp[0], 8, &idctConsts)
		idct16Cols8AVX2(&tmp[128], 8, &tmp[128], 8, &idctConsts)
		idct32PostAVX2(&tmp[0], &dst[0], dstStride, &idctConsts)
	default:
		return false
	}

	return true
}

func idct8x8(data []float32) bool {
	if !useIDCT8x8 {
		return false
	}

	idct8x8AVX2(&data[0], &idctConsts)

	return true
}
