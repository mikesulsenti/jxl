//go:build amd64 && !noasm

package jxl

var hasAVX2 = cpuidAVX2()

func cpuidAVX2() bool

//go:noescape
func gaborishAVX2(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func dspInit() {
	if !hasAVX2 {
		return
	}

	gaborishAsm = gaborishAVX2
}
