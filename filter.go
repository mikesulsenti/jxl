package jxl

import "math"

func mirror1(i, n int) int {
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}

	return i
}

func gaborish(plane, scratch []float32, w, h, threads int, weight1, weight2 float32) {
	total := 1 + weight1*4 + weight2*4
	w0, w1, w2 := 1/total, weight1/total, weight2/total

	overRows(threads, h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			top := mirror1(y-1, h) * w
			mid := y * w
			bot := mirror1(y+1, h) * w

			trow := plane[top : top+w]
			mrow := plane[mid : mid+w]
			brow := plane[bot : bot+w]
			orow := scratch[mid : mid+w]

			for x := range w {
				l := mirror1(x-1, w)
				r := mirror1(x+1, w)

				orow[x] = mrow[x]*w0 +
					(trow[x]+mrow[l]+brow[x]+mrow[r])*w1 +
					(trow[l]+trow[r]+brow[l]+brow[r])*w2
			}
		}
	})

	copy(plane, scratch[:w*h])
}

func mirror(v, s int) int {
	for {
		switch {
		case v < 0:
			v = -v - 1
		case v >= s:
			v = s*2 - v - 1
		default:
			return v
		}
	}
}

const (
	epfMinSigma    = -3.90524291751269967465540850526868
	epfInvSigmaNum = -1.1715728752538099024
)

const epfRadius = 3

type epfPlanes struct {
	planes  [][]float32
	scales  []float32
	sigma   []float32
	sw      int
	w, h    int
	threads int

	xoff []int
	yoff []int
	out  [][]float32
}

func (e *epfPlanes) init() {
	e.xoff = make([]int, e.w+2*epfRadius)
	for i := range e.xoff {
		e.xoff[i] = mirror(i-epfRadius, e.w)
	}

	e.yoff = make([]int, e.h+2*epfRadius)
	for i := range e.yoff {
		e.yoff[i] = mirror(i-epfRadius, e.h) * e.w
	}

	e.out = make([][]float32, len(e.planes))
	for i := range e.out {
		e.out[i] = make([]float32, e.w*e.h)
	}
}

func (e *epfPlanes) sigmaAt(x, y int) float32 {
	return e.sigma[(y>>3)*e.sw+(x>>3)]
}

func epfSadMul(x, y int, sm, bsm float32) float32 {
	if y%blockDim == 0 || y%blockDim == blockDim-1 ||
		x%blockDim == 0 || x%blockDim == blockDim-1 {
		return bsm
	}

	return sm
}

func (e *epfPlanes) pass1(sigmaScale, borderSadMul float32) {
	sm := sigmaScale * 1.65
	bsm := sm * borderSadMul

	out := e.out

	overRows(e.threads, e.h, func(y0, y1 int) {
		var sads [4]float32

		var rows, xs [7]int

		for y := y0; y < y1; y++ {
			for i := range rows {
				rows[i] = e.yoff[y+i]
			}

			for x := range e.w {
				for i := range xs {
					xs[i] = e.xoff[x+i]
				}

				sigma := e.sigmaAt(x, y)

				if epfMinSigma > sigma {
					for c := range e.planes {
						out[c][y*e.w+x] = e.planes[c][rows[3]+xs[3]]
					}

					continue
				}

				sads = [4]float32{}

				for c := range e.planes {
					pl := e.planes[c]
					s := e.scales[c]

					p20 := pl[rows[1]+xs[3]]
					p11 := pl[rows[2]+xs[2]]
					p21 := pl[rows[2]+xs[3]]
					p31 := pl[rows[2]+xs[4]]
					p02 := pl[rows[3]+xs[1]]
					p12 := pl[rows[3]+xs[2]]
					p22 := pl[rows[3]+xs[3]]
					p32 := pl[rows[3]+xs[4]]
					p42 := pl[rows[3]+xs[5]]
					p13 := pl[rows[4]+xs[2]]
					p23 := pl[rows[4]+xs[3]]
					p33 := pl[rows[4]+xs[4]]
					p24 := pl[rows[5]+xs[3]]

					sads[0] += s * (absF(p20-p21) + absF(p11-p12) + absF(p22-p21) +
						absF(p31-p32) + absF(p22-p23))
					sads[1] += s * (absF(p11-p21) + absF(p02-p12) + absF(p22-p12) +
						absF(p22-p32) + absF(p13-p23))
					sads[2] += s * (absF(p31-p21) + absF(p22-p12) + absF(p22-p32) +
						absF(p42-p32) + absF(p33-p23))
					sads[3] += s * (absF(p22-p21) + absF(p13-p12) + absF(p22-p23) +
						absF(p33-p32) + absF(p24-p23))
				}

				invSigma := sigma * epfSadMul(x, y, sm, bsm)
				w := float32(1)

				for i := range sads {
					sads[i] = max(sads[i]*invSigma+1, 0)
					w += sads[i]
				}

				for c := range e.planes {
					pl := e.planes[c]

					v := pl[rows[3]+xs[3]] +
						sads[3]*pl[rows[4]+xs[3]] +
						sads[2]*pl[rows[3]+xs[4]] +
						sads[1]*pl[rows[3]+xs[2]] +
						sads[0]*pl[rows[2]+xs[3]]

					out[c][y*e.w+x] = v / w
				}
			}
		}
	})

	for c := range e.planes {
		copy(e.planes[c], out[c])
	}
}

func (e *epfPlanes) pass2(sigmaScale, borderSadMul float32) {
	sm := sigmaScale * 1.65
	bsm := sm * borderSadMul

	out := e.out

	offsets := [4][2]int{{0, -1}, {-1, 0}, {1, 0}, {0, 1}}

	overRows(e.threads, e.h, func(y0, y1 int) {
		acc := make([]float32, len(e.planes))

		var rows, xs [7]int

		for y := y0; y < y1; y++ {
			for i := range rows {
				rows[i] = e.yoff[y+i]
			}

			for x := range e.w {
				for i := range xs {
					xs[i] = e.xoff[x+i]
				}

				sigma := e.sigmaAt(x, y)

				if epfMinSigma > sigma {
					for c := range e.planes {
						out[c][y*e.w+x] = e.planes[c][rows[3]+xs[3]]
					}

					continue
				}

				invSigma := sigma * epfSadMul(x, y, sm, bsm)
				w := float32(1)

				for c := range e.planes {
					acc[c] = e.planes[c][rows[3]+xs[3]]
				}

				for _, o := range offsets {
					var sad float32

					for c := range e.planes {
						pl := e.planes[c]
						sad += absF(pl[rows[o[1]+3]+xs[o[0]+3]]-pl[rows[3]+xs[3]]) * e.scales[c]
					}

					weight := max(sad*invSigma+1, 0)
					w += weight

					for c := range e.planes {
						acc[c] += weight * e.planes[c][rows[o[1]+3]+xs[o[0]+3]]
					}
				}

				for c := range e.planes {
					out[c][y*e.w+x] = acc[c] / w
				}
			}
		}
	})

	for c := range e.planes {
		copy(e.planes[c], out[c])
	}
}

func absF(v float32) float32 {
	return math.Float32frombits(math.Float32bits(v) &^ (1 << 31))
}
