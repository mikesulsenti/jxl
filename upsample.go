package jxl

func upsampleKernels(weights []float32, n int) [][25]float32 {
	half := n / 2

	var kernel [8][8][5][5]float32

	for i := range 5 * half {
		for j := range 5 * half {
			y, x := min(i, j), max(i, j)
			idx := 5*half*y - y*(y-1)/2 + x - y
			w := weights[idx]

			kernel[j/5][i/5][j%5][i%5] = w
			kernel[2*half-1-j/5][i/5][4-j%5][i%5] = w
			kernel[j/5][2*half-1-i/5][j%5][4-i%5] = w
			kernel[2*half-1-j/5][2*half-1-i/5][4-j%5][4-i%5] = w
		}
	}

	out := make([][25]float32, n*n)

	for oy := range n {
		for ox := range n {
			k := &out[oy*n+ox]

			for i := range 5 {
				for j := range 5 {
					k[i*5+j] = kernel[oy][ox][i][j]
				}
			}
		}
	}

	return out
}

func upsamplePlane(src []float32, sw, sh, n, threads int, kernels [][25]float32) []float32 {
	dw := sw * n
	dst := make([]float32, dw*sh*n)

	overRows(threads, sh, func(sy0, sy1 int) {
		var vals [25]float32

		for y := sy0; y < sy1; y++ {
			for x := range sw {
				mn, mx := src[y*sw+x], src[y*sw+x]

				for iy := range 5 {
					row := mirror(y+iy-2, sh) * sw

					for ix := range 5 {
						v := src[row+mirror(x+ix-2, sw)]
						vals[iy*5+ix] = v
						mn, mx = min(mn, v), max(mx, v)
					}
				}

				for oy := range n {
					out := dst[(y*n+oy)*dw+x*n:]

					for ox := range n {
						k := &kernels[oy*n+ox]

						acc0 := vals[0] * k[0]
						acc1 := vals[1] * k[1]
						acc2 := vals[2] * k[2]

						for i := 3; i < 24; i += 3 {
							acc0 += vals[i] * k[i]
							acc1 += vals[i+1] * k[i+1]
							acc2 += vals[i+2] * k[i+2]
						}

						acc0 += vals[24] * k[24]

						out[ox] = min(max((acc1+acc2)+acc0, mn), mx)
					}
				}
			}
		}
	})

	return dst
}

func (d *decoder) upsamplePlanes(f *frame, planes [][]float32, cw, ch, w, h int) [][]float32 {
	n := int(f.hdr.upsampling)
	t := &d.hdr.transform

	var weights []float32

	switch n {
	case 2:
		weights = t.weights2[:]
	case 4:
		weights = t.weights4[:]
	default:
		weights = t.weights8[:]
	}

	kernels := upsampleKernels(weights, n)

	out := make([][]float32, len(planes))
	dw := cw * n

	for i, p := range planes {
		up := upsamplePlane(p, cw, ch, n, d.threads, kernels)

		out[i] = make([]float32, w*h)
		for y := range h {
			copy(out[i][y*w:(y+1)*w], up[y*dw:])
		}
	}

	return out
}

func chromaUpsampleH(src []float32, w, h int) []float32 {
	dst := make([]float32, 2*w*h)

	for y := range h {
		in := src[y*w : (y+1)*w]
		out := dst[y*2*w:]

		for x := range w {
			prev := in[max(x-1, 0)]
			next := in[min(x+1, w-1)]

			out[2*x] = prev*0.25 + in[x]*0.75
			out[2*x+1] = next*0.25 + in[x]*0.75
		}
	}

	return dst
}

func chromaUpsampleV(src []float32, w, h int) []float32 {
	dst := make([]float32, w*2*h)

	for y := range h {
		prev := src[max(y-1, 0)*w:]
		cur := src[y*w:]
		next := src[min(y+1, h-1)*w:]

		up := dst[2*y*w:]
		down := dst[(2*y+1)*w:]

		for x := range w {
			up[x] = prev[x]*0.25 + cur[x]*0.75
			down[x] = next[x]*0.25 + cur[x]*0.75
		}
	}

	return dst
}
