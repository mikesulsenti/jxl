package jxl

var epf1Asm func(dst, src *[3]*float32, stride int, sigma *float32, sigmaStride int,
	mul, scales *float32, y, x0, x1 int)

var xybSRGBAsm func(x, y, b *float32, n int, k *[29][8]float32)

var gaborishAsm func(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func init() { dspInit() }
