package jxl

var xybSRGBAsm func(x, y, b *float32, n int, k *[29][8]float32)

var gaborishAsm func(dst, top, mid, bot *float32, n int, k *[3][8]float32)

func init() { dspInit() }
