//go:build arm64 && !noasm

package jxl

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
	epf1Asm = epf1NEON
	epf2Asm = epf2NEON
	xybSRGBAsm = xybSRGBNEON
	gaborishAsm = gaborishNEON
}
