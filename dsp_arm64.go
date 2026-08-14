//go:build arm64 && !noasm

package jxl

//go:noescape
func epf0NEON(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

//go:noescape
func epf1NEON(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	mul, scales *float32, y, x0, x1 int)

//go:noescape
func epf2NEON(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	k *[6][8]float32, y, x0, x1 int)

//go:noescape
func xybSRGBNEON(x, y, b *float32, n int, k *[29][8]float32)

//go:noescape
func gaborishNEON(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	epf0Asm = epf0NEON
	epf1Asm = epf1NEON
	epf2Asm = epf2NEON
	xybSRGBAsm = xybSRGBNEON
	gaborishAsm = gaborishNEON
	useIDCTCols = true
	useTranspose8 = true
	useDequant = true
	useQuant = true
}

func idct8x8([]float32) bool { return false }

//go:noescape
func idct8Cols8NEON(src *float32, srcStride int, dst *float32, dstStride int, k *[31][8]float32, tmp *float32)

//go:noescape
func idct16Cols8NEON(src *float32, srcStride int, dst *float32, dstStride int, k *[31][8]float32, tmp *float32)

//go:noescape
func idct32PreNEON(src *float32, srcStride int, tmp *float32, k *[31][8]float32)

//go:noescape
func idct32PostNEON(tmp *float32, dst *float32, dstStride int, k *[31][8]float32)

//go:noescape
func transpose8NEON(src *float32, srcStride int, dst *float32, dstStride int)

func transpose8(src []float32, srcStride int, dst []float32, dstStride int) {
	transpose8NEON(&src[0], srcStride, &dst[0], dstStride)
}

func idctCols8(n int, src []float32, srcStride int, dst []float32, dstStride int, tmp []float32) bool {
	if !useIDCTCols {
		return false
	}

	switch n {
	case 8:
		idct8Cols8NEON(&src[0], srcStride, &dst[0], dstStride, &idctConsts, &tmp[0])
	case 16:
		idct16Cols8NEON(&src[0], srcStride, &dst[0], dstStride, &idctConsts, &tmp[0])
	case 32:
		if len(tmp) < 384 {
			return false
		}

		idct32PreNEON(&src[0], srcStride, &tmp[0], &idctConsts)
		idct16Cols8NEON(&tmp[0], 8, &tmp[0], 8, &idctConsts, &tmp[256])
		idct16Cols8NEON(&tmp[128], 8, &tmp[128], 8, &idctConsts, &tmp[256])
		idct32PostNEON(&tmp[0], &dst[0], dstStride, &idctConsts)
	default:
		return false
	}

	return true
}

//go:noescape
func dequantNEON(a *dequantArgs)

func dequant(a *dequantArgs) bool {
	if !useDequant {
		return false
	}

	dequantNEON(a)

	return true
}

//go:noescape
func quantizeNEON(a *quantArgs)

func quantize(a *quantArgs) bool {
	if !useQuant {
		return false
	}

	quantizeNEON(a)

	return true
}

func fdctCols8(int, []float32, int, []float32, int) bool { return false }
