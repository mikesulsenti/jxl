package jxl

func (e *epfPlanes) pass0(sigmaScale, borderSadMul float32) {
	sm := sigmaScale * 1.65
	bsm := sm * borderSadMul

	out := e.out

	overRows(e.threads, e.h, func(y0, y1 int) {
		var sads [12]float32

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

				sads = [12]float32{}

				for c := range e.planes {
					pl := e.planes[c]
					s := e.scales[c]

					p30 := pl[rows[0]+xs[3]]
					p21 := pl[rows[1]+xs[2]]
					p31 := pl[rows[1]+xs[3]]
					p41 := pl[rows[1]+xs[4]]
					p12 := pl[rows[2]+xs[1]]
					p22 := pl[rows[2]+xs[2]]
					p32 := pl[rows[2]+xs[3]]
					p42 := pl[rows[2]+xs[4]]
					p52 := pl[rows[2]+xs[5]]
					p03 := pl[rows[3]+xs[0]]
					p13 := pl[rows[3]+xs[1]]
					p23 := pl[rows[3]+xs[2]]
					p33 := pl[rows[3]+xs[3]]
					p43 := pl[rows[3]+xs[4]]
					p53 := pl[rows[3]+xs[5]]
					p63 := pl[rows[3]+xs[6]]
					p14 := pl[rows[4]+xs[1]]
					p24 := pl[rows[4]+xs[2]]
					p34 := pl[rows[4]+xs[3]]
					p44 := pl[rows[4]+xs[4]]
					p54 := pl[rows[4]+xs[5]]
					p25 := pl[rows[5]+xs[2]]
					p35 := pl[rows[5]+xs[3]]
					p45 := pl[rows[5]+xs[4]]
					p36 := pl[rows[6]+xs[3]]

					sads[0] += s * (absF(p32-p30) + absF(p23-p21) + absF(p33-p31) +
						absF(p43-p41) + absF(p32-p34))
					sads[1] += s * (absF(p32-p21) + absF(p23-p12) + absF(p33-p22) +
						absF(p32-p43) + absF(p23-p34))
					sads[2] += s * (absF(p32-p31) + absF(p23-p22) + absF(p32-p33) +
						absF(p43-p42) + absF(p33-p34))
					sads[3] += s * (absF(p32-p41) + absF(p32-p23) + absF(p33-p42) +
						absF(p43-p52) + absF(p43-p34))
					sads[4] += s * (absF(p32-p12) + absF(p23-p03) + absF(p33-p13) +
						absF(p23-p43) + absF(p34-p14))
					sads[5] += s * (absF(p32-p22) + absF(p23-p13) + absF(p23-p33) +
						absF(p33-p43) + absF(p34-p24))
					sads[6] += s * (absF(p32-p42) + absF(p23-p33) + absF(p33-p43) +
						absF(p43-p53) + absF(p34-p44))
					sads[7] += s * (absF(p32-p52) + absF(p23-p43) + absF(p33-p53) +
						absF(p43-p63) + absF(p34-p54))
					sads[8] += s * (absF(p32-p23) + absF(p23-p14) + absF(p33-p24) +
						absF(p43-p34) + absF(p34-p25))
					sads[9] += s * (absF(p32-p33) + absF(p23-p24) + absF(p33-p34) +
						absF(p43-p44) + absF(p34-p35))
					sads[10] += s * (absF(p32-p43) + absF(p23-p34) + absF(p33-p44) +
						absF(p43-p54) + absF(p34-p45))
					sads[11] += s * (absF(p32-p34) + absF(p23-p25) + absF(p33-p35) +
						absF(p43-p45) + absF(p34-p36))
				}

				invSigma := sigma * epfSadMul(x, y, sm, bsm)
				w := float32(1)

				for i := range sads {
					sads[i] = max(sads[i]*invSigma+1, 0)
					w += sads[i]
				}

				for c := range e.planes {
					pl := e.planes[c]

					v := pl[rows[3]+xs[3]]

					v += sads[11] * pl[rows[5]+xs[3]]
					v += sads[10] * pl[rows[4]+xs[4]]
					v += sads[9] * pl[rows[4]+xs[3]]
					v += sads[8] * pl[rows[4]+xs[2]]
					v += sads[7] * pl[rows[3]+xs[5]]
					v += sads[6] * pl[rows[3]+xs[4]]
					v += sads[5] * pl[rows[3]+xs[2]]
					v += sads[4] * pl[rows[3]+xs[1]]
					v += sads[3] * pl[rows[2]+xs[4]]
					v += sads[2] * pl[rows[2]+xs[3]]
					v += sads[1] * pl[rows[2]+xs[2]]
					v += sads[0] * pl[rows[1]+xs[3]]

					out[c][y*e.w+x] = v / w
				}
			}
		}
	})

	for c := range e.planes {
		copy(e.planes[c], out[c])
	}
}
