//go:build riscv64 && riscv64.rva23u64 && !noasm

package jxl

//go:noescape
func epf1RVV(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	mul, scales *float32, y, x0, x1 int)

//go:noescape
func xybSRGBRVV(x, y, b *float32, n int, k *[29][8]float32)

//go:noescape
func gaborishRVV(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	epf1Asm = epf1RVV
	xybSRGBAsm = xybSRGBRVV
	gaborishAsm = gaborishRVV
}
