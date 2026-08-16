package jxl

import (
	"math"
	"slices"
)

const (
	aqSGmul     = 226.77216153508914
	aqSGmul2    = 1.0 / 73.377132366608819
	aqSGVOffset = 7.7825991679894591
	aqInvLog2e  = 0.6931471805599453
	aqSGRetMul  = aqSGmul2 * 18.6580932135 * aqInvLog2e
	aqEpsilon   = 1e-2

	aqGammaOffset = 0.019
	aqDiffLimit   = 0.2

	aqACQuant     = 0.765
	aqDampenStart = 2
	aqDampenEnd   = 14

	aqDCQuant    = 1.095924047623553
	aqDCQuantPow = 0.83
	aqDCMul      = 0.3

	quantFieldTarget = 5
	globalScaleNum   = 4096
)

func gammaRatioParts(v float32) (float32, float32) {
	v = max(v, 0)
	v2 := v * v

	num := float32(aqSGRetMul*3*aqSGmul)*v2 + aqEpsilon
	den := float32(aqInvLog2e*aqSGmul)*v*v2 + float32(aqSGVOffset*aqInvLog2e+aqEpsilon)

	return num, den
}

func gammaRatio(v float32) float32 {
	num, den := gammaRatioParts(v)

	return den / num
}

func gammaRatioInverse(v float32) float32 {
	num, den := gammaRatioParts(v)

	return num / den
}

func maskingSqrt(v float32) float32 {
	const (
		logOffset = 27.505837037000106
		mul       = 211.66567973503678
	)

	return 0.25 * sqrtf(v*sqrtf(mul*1e8)+logOffset)
}

func computeMask(out float32) float32 {
	const (
		base    = -0.7647
		mul4    = 9.4708735624378946
		mul2    = 17.35036561631863
		offset2 = 302.59587815579727
		mul3    = 6.7943250517376494
		offset3 = 3.7179635626140772
		offset4 = 0.25 * offset3
		mul0    = 0.80061762862741759
	)

	v1 := max(out*mul0, 1e-3)
	v2 := 1 / (v1 + offset2)
	v3 := 1 / (v1*v1 + offset3)
	v4 := 1 / (v1*v1 + offset4)

	return base + mul4*v4 + mul2*v2 + mul3*v3
}

func storeMin4(v float32, m *[4]float32) {
	switch {
	case v >= m[3]:
	case v < m[0]:
		m[3], m[2], m[1], m[0] = m[2], m[1], m[0], v
	case v < m[1]:
		m[3], m[2], m[1] = m[2], m[1], v
	case v < m[2]:
		m[3], m[2] = m[2], v
	default:
		m[3] = v
	}
}

func sort4(m *[4]float32) {
	for i := 1; i < 4; i++ {
		for j := i; j > 0 && m[j] < m[j-1]; j-- {
			m[j], m[j-1] = m[j-1], m[j]
		}
	}
}

func (f *frame) diffPrecompute(pre []float32, pw, ph int) {
	v := f.vardct
	w, h := v.bw*blockDim, v.bh*blockDim
	acc := make([]float32, w)

	for y := range h {
		up := v.pixels[1][max(y-1, 0)*w:]
		down := v.pixels[1][min(y+1, h-1)*w:]
		row := v.pixels[1][y*w:]

		for x := range w {
			base := 0.25 * (down[x] + up[x] + row[max(x-1, 0)] + row[min(x+1, w-1)])
			d := gammaRatio(row[x]+aqGammaOffset) * (row[x] - base)
			d = min(d*d, aqDiffLimit)

			if y%4 == 0 {
				acc[x] = maskingSqrt(d)
			} else {
				acc[x] += maskingSqrt(d)
			}
		}

		if y%4 != 3 {
			continue
		}

		out := pre[(y/4)*pw:]

		for qx := range pw {
			out[qx] = (acc[4*qx] + acc[4*qx+1] + acc[4*qx+2] + acc[4*qx+3]) * 0.25
		}
	}
}

func (f *frame) fuzzyErosion(distance float32, pre []float32, pw, ph int, field []float32) {
	v := f.vardct

	base := [4]float32{0.125, 0.1, 0.09, 0.06}
	add := [4]float32{0, -0.1, -0.09, -0.06}

	ramp := float32(0)
	if distance < 2 {
		ramp = (2 - distance) * 0.5
	}

	var mul [4]float32

	sum := float32(0)

	for i := range mul {
		mul[i] = base[i] + ramp*add[i]
		sum += mul[i]
	}

	norm := float32(0.29959705784054957) / sum

	for i := range mul {
		mul[i] *= norm
	}

	for fy := range ph {
		up := pre[max(fy-1, 0)*pw:]
		down := pre[min(fy+1, ph-1)*pw:]
		row := pre[fy*pw:]

		for fx := range pw {
			xm1, xp1 := max(fx-1, 0), min(fx+1, pw-1)

			m := [4]float32{row[fx], row[xm1], row[xp1], up[xm1]}
			sort4(&m)

			storeMin4(up[fx], &m)
			storeMin4(up[xp1], &m)
			storeMin4(down[xm1], &m)
			storeMin4(down[fx], &m)
			storeMin4(down[xp1], &m)

			val := mul[0]*m[0] + mul[1]*m[1] + mul[2]*m[2] + mul[3]*m[3]
			pos := (fy/2)*v.bw + fx/2

			if fx%2 == 0 && fy%2 == 0 {
				field[pos] = val
			} else {
				field[pos] += val
			}
		}
	}
}

func (f *frame) gammaModulation(x, y int, out float32) float32 {
	v := f.vardct
	stride := v.bw * blockDim

	const bias = 0.16

	overall := float32(0)

	for dy := range blockDim {
		rowX := v.pixels[0][(y+dy)*stride+x:]
		rowY := v.pixels[1][(y+dy)*stride+x:]

		for dx := range blockDim {
			inY, inX := rowY[dx]+bias, rowX[dx]

			overall += gammaRatioInverse(inY - inX)
			overall += gammaRatioInverse(inY + inX)
		}
	}

	overall *= 0.5 / blockSize

	return 0.1005613337192697*float32(math.Log2(float64(overall))) + out
}

func (f *frame) hfModulation(x, y int, out float32) float32 {
	v := f.vardct
	stride := v.bw * blockDim

	const valmin = 0.0206

	sum := float32(0)

	for dy := range blockDim {
		row := v.pixels[1][(y+dy)*stride+x:]
		next := row

		if dy != blockDim-1 {
			next = v.pixels[1][(y+dy+1)*stride+x:]
		}

		for dx := range blockDim {
			if dx != blockDim-1 {
				sum += min(valmin, absf(row[dx]-row[dx+1]))
			}

			sum += min(valmin, absf(row[dx]-next[dx]))
		}
	}

	return -0.38*sum + 0.42 + out
}

func (f *frame) blueModulation(x, y int, out float32) float32 {
	v := f.vardct
	stride := v.bw * blockDim

	const (
		limit    = 0.010474084867598155
		offset   = 0.0031994768654636393
		maxLimit = 15.463398341612438
	)

	sum := float32(0)

	for dy := range blockDim {
		rowX := v.pixels[0][(y+dy)*stride+x:]
		rowY := v.pixels[1][(y+dy)*stride+x:]
		rowB := v.pixels[2][(y+dy)*stride+x:]

		for dx := range blockDim {
			effective := rowY[dx] + offset + absf(rowX[dx])

			if rowB[dx] > effective {
				sum += min(rowB[dx]-effective, limit)
			}
		}
	}

	if sum >= 32*limit {
		sum = 64*limit - sum
	}

	sum = min(sum, maxLimit*limit)

	return sum*0.90590804735610064 + out
}

func quantFieldScale(distance float32) (mul, add float32) {
	scale := aqACQuant / distance
	dampen := float32(1)

	if distance >= aqDampenStart {
		dampen = max(1-(distance-aqDampenStart)/(aqDampenEnd-aqDampenStart), 0)
	}

	return scale * dampen, (1 - dampen) * 0.48 * scale
}

func (f *frame) flatQuantField(distance float32) []float32 {
	v := f.vardct
	mul, add := quantFieldScale(distance)

	field := make([]float32, v.bw*v.bh)

	for i := range field {
		field[i] = mul + add
	}

	return field
}

func (f *frame) adaptiveQuantField(distance, strength float32) []float32 {
	v := f.vardct
	pw, ph := v.bw*2, v.bh*2

	pre := make([]float32, pw*ph)
	field := make([]float32, v.bw*v.bh)

	f.diffPrecompute(pre, pw, ph)
	f.fuzzyErosion(distance, pre, pw, ph, field)

	mul, add := quantFieldScale(distance)

	for by := range v.bh {
		for bx := range v.bw {
			x, y := bx*blockDim, by*blockDim
			pos := by*v.bw + bx

			maskVal := f.gammaModulation(x, y, computeMask(field[pos]))

			out := min(f.hfModulation(x, y, maskVal), f.blueModulation(x, y, maskVal))

			field[pos] = expf(out*strength)*mul + add
		}
	}

	return field
}

func (f *frame) adjustQuantField() {
	v := f.vardct

	for by := range v.bh {
		for bx := range v.bw {
			pos := by*v.bw + bx

			if v.transform[pos] < 128 {
				continue
			}

			cx, cy := hfTransform(v.transform[pos] & 127).blocks()

			if cx == 1 && cy == 1 {
				continue
			}

			best := v.rawQuant[pos]

			for iy := range cy {
				for ix := range cx {
					best = max(best, v.rawQuant[(by+iy)*v.bw+bx+ix])
				}
			}

			for iy := range cy {
				for ix := range cx {
					v.rawQuant[(by+iy)*v.bw+bx+ix] = best
				}
			}
		}
	}
}

func initialQuantDC(distance float32) float32 {
	dc := max(0.5*distance, min(distance,
		aqDCMul*powf(distance/aqDCMul, aqDCQuantPow)))

	return min(aqDCQuant/dc, 50)
}

func medianOf(v []float32) float32 {
	s := make([]float32, len(v))
	copy(s, v)
	slices.Sort(s)

	return s[len(s)/2]
}

func (f *frame) setQuantField(distance, strength float32) {
	v := f.vardct

	field := f.flatQuantField(distance)
	if strength != 0 {
		field = f.adaptiveQuantField(distance, strength)
	}

	median := medianOf(field)

	dev := make([]float32, len(field))

	for i, s := range field {
		dev[i] = absf(s - median)
	}

	scale := float32(globalScaleDenom) * (median - medianOf(dev)) / quantFieldTarget
	quantDC := initialQuantDC(distance)

	gs := min(max(int32(scale), 1), 1<<15)

	if dc := int32(quantDC * globalScaleNum * 1.6); gs > dc {
		gs = max(dc, 1)
	}

	v.quant.globalScale = uint32(gs)

	inv := float32(globalScaleDenom) / float32(gs)

	v.quant.quantLF = uint32(min(quantDC*inv+0.5, 1<<16))

	for i, s := range field {
		v.rawQuant[i] = min(max(int32(s*inv+0.5), 1), maxRawQuant)
	}
}
