package jxl

type hfTransform uint8

const (
	hfDCT hfTransform = iota
	hfIdentity
	hfDCT2X2
	hfDCT4X4
	hfDCT16X16
	hfDCT32X32
	hfDCT16X8
	hfDCT8X16
	hfDCT32X8
	hfDCT8X32
	hfDCT32X16
	hfDCT16X32
	hfDCT4X8
	hfDCT8X4
	hfAFV0
	hfAFV1
	hfAFV2
	hfAFV3
	hfDCT64X64
	hfDCT64X32
	hfDCT32X64
	hfDCT128X128
	hfDCT128X64
	hfDCT64X128
	hfDCT256X256
	hfDCT256X128
	hfDCT128X256
	numHFTransforms
)

var (
	hfCoveredX = [numHFTransforms]uint8{
		1, 1, 1, 1, 2, 4, 1, 2, 1, 4, 2, 4, 1, 1, 1, 1, 1, 1, 8, 4, 8, 16, 8, 16, 32, 16, 32,
	}
	hfCoveredY = [numHFTransforms]uint8{
		1, 1, 1, 1, 2, 4, 2, 1, 4, 1, 4, 2, 1, 1, 1, 1, 1, 1, 8, 8, 4, 16, 16, 8, 32, 32, 16,
	}
	hfShapeID = [numHFTransforms]uint8{
		0, 1, 1, 1, 2, 3, 4, 4, 5, 5, 6, 6, 1, 1, 1, 1, 1, 1, 7, 8, 8, 9, 10, 10, 11, 12, 12,
	}
)

const numOrders = 13

var orderTransform = [numOrders]hfTransform{
	hfDCT, hfIdentity, hfDCT16X16, hfDCT32X32, hfDCT8X16, hfDCT8X32, hfDCT16X32,
	hfDCT64X64, hfDCT32X64, hfDCT128X128, hfDCT64X128, hfDCT256X256, hfDCT128X256,
}

func (t hfTransform) blocks() (int, int) {
	return int(hfCoveredX[t]), int(hfCoveredY[t])
}

func naturalCoeffOrder(t hfTransform) []uint32 {
	cx, cy := t.blocks()
	xsize := cx * blockDim
	xs := cx / cy
	xsm := xs - 1
	xss := ceilLog2(uint32(xs))

	out := make([]uint32, cx*cy*blockSize)
	cur := cx * cy

	for i := range xsize {
		for j := 0; j <= i; j++ {
			x, y := j, i-j
			if i%2 != 0 {
				x, y = y, x
			}

			if y&xsm != 0 {
				continue
			}

			y >>= xss

			val := cur

			if x < cx && y < cy {
				val = y*cx + x
			} else {
				cur++
			}

			out[val] = uint32(y*xsize + x)
		}
	}

	for ir := 1; ir < xsize; ir++ {
		i := xsize - ir - 1

		for j := 0; j <= i; j++ {
			x, y := xsize-1-(i-j), xsize-1-j
			if i%2 != 0 {
				x, y = y, x
			}

			if y&xsm != 0 {
				continue
			}

			y >>= xss

			out[cur] = uint32(y*xsize + x)
			cur++
		}
	}

	return out
}

var naturalOrders [numOrders][]uint32

func init() {
	for i, t := range orderTransform {
		naturalOrders[i] = naturalCoeffOrder(t)
	}
}

func idct2Top(s int, in, out []float32) {
	n := s / 2

	for y := range n {
		for x := range n {
			c00 := in[y*blockDim+x]
			c01 := in[y*blockDim+n+x]
			c10 := in[(y+n)*blockDim+x]
			c11 := in[(y+n)*blockDim+n+x]

			out[y*2*blockDim+x*2] = c00 + c01 + c10 + c11
			out[y*2*blockDim+x*2+1] = c00 + c01 - c10 - c11
			out[(y*2+1)*blockDim+x*2] = c00 - c01 + c10 - c11
			out[(y*2+1)*blockDim+x*2+1] = c00 - c01 - c10 + c11
		}
	}
}

func afvIDCT4x4(coeffs, pixels []float32) {
	for i := range 16 {
		var v float32

		for j := range 16 {
			v += coeffs[j] * afv4x4Basis[j*16+i]
		}

		pixels[i] = v
	}
}

func afvToPixels(kind int, coeffs, pixels, scratch []float32) {
	afvX := kind & 1
	afvY := kind / 2

	b00, b01, b10 := coeffs[0], coeffs[1], coeffs[blockDim]
	dcs := [3]float32{(b00 + b10 + b01) * 4, b00 + b10 - b01, b00 - b10}

	var coeff [16]float32

	for iy := range 4 {
		for ix := range 4 {
			coeff[iy*4+ix] = coeffs[iy*2*blockDim+ix*2]
		}
	}

	coeff[0] = dcs[0]

	var block [32]float32

	afvIDCT4x4(coeff[:], block[:])

	for iy := range 4 {
		by := iy
		if afvY == 1 {
			by = 3 - iy
		}

		for ix := range 4 {
			bx := ix
			if afvX == 1 {
				bx = 3 - ix
			}

			pixels[(iy+afvY*4)*blockDim+afvX*4+ix] = block[by*4+bx]
		}
	}

	for iy := range 4 {
		for ix := range 4 {
			block[iy*4+ix] = coeffs[iy*2*blockDim+ix*2+1]
		}
	}

	block[0] = dcs[1]

	idct2d(4, 4, block[:16], scratch)

	for iy := range 4 {
		for ix := range 4 {
			pixels[(iy+afvY*4)*blockDim+(1-afvX)*4+ix] = block[iy*4+ix]
		}
	}

	for iy := range 4 {
		for ix := range blockDim {
			block[iy*blockDim+ix] = coeffs[(1+iy*2)*blockDim+ix]
		}
	}

	block[0] = dcs[2]

	idct2d(4, blockDim, block[:], scratch)

	for iy := range 4 {
		for ix := range blockDim {
			pixels[(iy+(1-afvY)*4)*blockDim+ix] = block[iy*blockDim+ix]
		}
	}
}

func transformToPixels(t hfTransform, lf, buf, scratch []float32) {
	cx, cy := t.blocks()

	switch t {
	case hfIdentity:
		buf[0] = lf[0]

		var coeffs [blockSize]float32

		copy(coeffs[:], buf[:blockSize])

		b00, b01, b10, b11 := coeffs[0], coeffs[1], coeffs[blockDim], coeffs[blockDim+1]
		dcs := [4]float32{
			b00 + b01 + b10 + b11,
			b00 + b01 - b10 - b11,
			b00 - b01 + b10 - b11,
			b00 - b01 - b10 + b11,
		}

		for y := range 2 {
			for x := range 2 {
				var residual float32

				for iy := range 4 {
					for ix := range 4 {
						if ix == 0 && iy == 0 {
							continue
						}

						residual += coeffs[(y+iy*2)*blockDim+x+ix*2]
					}
				}

				center := dcs[y*2+x] - residual*(1.0/16)
				buf[(4*y+1)*blockDim+4*x+1] = center

				for iy := range 4 {
					for ix := range 4 {
						if ix == 1 && iy == 1 {
							continue
						}

						buf[(y*4+iy)*blockDim+x*4+ix] = coeffs[(y+iy*2)*blockDim+x+ix*2] + center
					}
				}

				buf[y*4*blockDim+x*4] = coeffs[(y+2)*blockDim+x+2] + center
			}
		}
	case hfDCT2X2:
		buf[0] = lf[0]

		var tmp [blockSize]float32

		copy(tmp[:], buf[:blockSize])
		idct2Top(2, tmp[:], buf)
		idct2Top(4, buf, tmp[:])
		idct2Top(blockDim, tmp[:], buf)
	case hfDCT4X4:
		buf[0] = lf[0]

		var coeffs [blockSize]float32

		copy(coeffs[:], buf[:blockSize])

		b00, b01, b10, b11 := coeffs[0], coeffs[1], coeffs[blockDim], coeffs[blockDim+1]
		dcs := [4]float32{
			b00 + b01 + b10 + b11,
			b00 + b01 - b10 - b11,
			b00 - b01 + b10 - b11,
			b00 - b01 - b10 + b11,
		}

		for y := range 2 {
			for x := range 2 {
				var block [16]float32

				for iy := range 4 {
					for ix := range 4 {
						block[iy*4+ix] = coeffs[(y+iy*2)*blockDim+x+ix*2]
					}
				}

				block[0] = dcs[y*2+x]

				idct2d(4, 4, block[:], scratch)

				for iy := range 4 {
					for ix := range 4 {
						buf[(y*4+iy)*blockDim+x*4+ix] = block[iy*4+ix]
					}
				}
			}
		}
	case hfDCT8X4:
		buf[0] = lf[0]

		var coeffs [blockSize]float32

		copy(coeffs[:], buf[:blockSize])

		dcs := [2]float32{coeffs[0] + coeffs[blockDim], coeffs[0] - coeffs[blockDim]}

		for x := range 2 {
			var block [32]float32

			for iy := range 4 {
				for ix := range blockDim {
					block[iy*blockDim+ix] = coeffs[(x+iy*2)*blockDim+ix]
				}
			}

			block[0] = dcs[x]

			idct2d(blockDim, 4, block[:], scratch)

			for iy := range blockDim {
				for ix := range 4 {
					buf[iy*blockDim+x*4+ix] = block[iy*4+ix]
				}
			}
		}
	case hfDCT4X8:
		buf[0] = lf[0]

		var coeffs [blockSize]float32

		copy(coeffs[:], buf[:blockSize])

		dcs := [2]float32{coeffs[0] + coeffs[blockDim], coeffs[0] - coeffs[blockDim]}

		for y := range 2 {
			var block [32]float32

			for iy := range 4 {
				for ix := range blockDim {
					block[iy*blockDim+ix] = coeffs[(y+iy*2)*blockDim+ix]
				}
			}

			block[0] = dcs[y]

			idct2d(4, blockDim, block[:], scratch)

			for iy := range 4 {
				for ix := range blockDim {
					buf[(y*4+iy)*blockDim+ix] = block[iy*blockDim+ix]
				}
			}
		}
	case hfAFV0, hfAFV1, hfAFV2, hfAFV3:
		buf[0] = lf[0]

		var coeffs [blockSize]float32

		copy(coeffs[:], buf[:blockSize])
		afvToPixels(int(t-hfAFV0), coeffs[:], buf, scratch)
	default:
		scaledDCT2D(cy, cx, lf, buf, blockDim*max(cx, cy), scratch)
		idct2d(cy*blockDim, cx*blockDim, buf, scratch)
	}
}

func transformFromPixels(t hfTransform, buf, lf, scratch []float32) bool {
	if t != hfDCT {
		return false
	}

	fdct2d(blockDim, blockDim, buf, scratch)
	lf[0] = buf[0]

	return true
}
