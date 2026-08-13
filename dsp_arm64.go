//go:build arm64 && !noasm

package jxl

//go:noescape
func gaborishNEON(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	gaborishAsm = gaborishNEON
}
