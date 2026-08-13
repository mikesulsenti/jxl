//go:build amd64 && !noasm

package jxl

var hasAVX2 = cpuidAVX2()

func cpuidAVX2() bool

//go:noescape
func epf1AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	mul, scales *float32, y, x0, x1 int)

//go:noescape
func epf2AVX2(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

//go:noescape
func xybSRGBAVX2(x, y, b *float32, n int, k *[29][8]float32)

//go:noescape
func idct8x8AVX2(data *float32, k *[31][8]float32)

//go:noescape
func gaborishAVX2(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	if !hasAVX2 {
		return
	}

	epf1Asm = epf1AVX2
	epf2Asm = epf2AVX2
	xybSRGBAsm = xybSRGBAVX2
	gaborishAsm = gaborishAVX2
	useIDCT8x8 = true
	useIDCTCols = true
	useTranspose8 = true
	useDequant = true
}

//go:noescape
func idct8Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[31][8]float32)

//go:noescape
func idct16Cols8AVX2(src *float32, srcStride int, dst *float32, dstStride int, k *[31][8]float32)

//go:noescape
func dequantAVX2(a *dequantArgs)

func dequant(a *dequantArgs) bool {
	if !useDequant {
		return false
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
