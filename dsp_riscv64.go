//go:build riscv64 && riscv64.rva23u64 && !noasm

package jxl

//go:noescape
func xybSRGBRVV(x, y, b *float32, n int, k *[29][8]float32)

//go:noescape
func gaborishRVV(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	xybSRGBAsm = xybSRGBRVV
	gaborishAsm = gaborishRVV
}
