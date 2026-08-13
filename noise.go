package jxl

import "math"

type noiseParams struct {
	lut [8]float32
}

func (n *noiseParams) read(r *bitReader) {
	for i := range n.lut {
		n.lut[i] = float32(r.bits(10)) / (1 << 10)
	}
}

func (n *noiseParams) zero() bool {
	for _, v := range n.lut {
		if v != 0 {
			return false
		}
	}

	return true
}

func (n *noiseParams) strength(vx float32) float32 {
	const scale = float32(len(n.lut) - 2)

	sv := max(0, vx*scale)

	floor := float32(math.Floor(float64(sv)))
	frac := sv - floor

	if sv >= scale+1 {
		floor, frac = scale, 1
	}

	lo, hi := n.lut[int(floor)], n.lut[int(floor)+1]

	return min(max((hi-lo)*frac+lo, 0), 1)
}

const xorshiftLanes = 8

type xorshift128 struct {
	s0, s1 [xorshiftLanes]uint64
}

func splitMix64(z uint64) uint64 {
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB

	return z ^ (z >> 31)
}

func newXorshift128(s1, s2, s3, s4 uint32) *xorshift128 {
	x := &xorshift128{}

	x.s0[0] = splitMix64((uint64(s1)<<32 + uint64(s2)) + 0x9E3779B97F4A7C15)
	x.s1[0] = splitMix64((uint64(s3)<<32 + uint64(s4)) + 0x9E3779B97F4A7C15)

	for i := 1; i < xorshiftLanes; i++ {
		x.s0[i] = splitMix64(x.s0[i-1])
		x.s1[i] = splitMix64(x.s1[i-1])
	}

	return x
}

func (x *xorshift128) fill(out *[xorshiftLanes]uint64) {
	for i := range xorshiftLanes {
		s1 := x.s0[i]
		s0 := x.s1[i]

		x.s0[i] = s0
		out[i] = s1 + s0

		s1 ^= s1 << 23
		x.s1[i] = s1 ^ s0 ^ (s1 >> 18) ^ (s0 >> 5)
	}
}

const floatsPerBatch = xorshiftLanes * 2

func (d *decoder) noisePlanes(f *frame, w, h int) [3][]float32 {
	var out [3][]float32

	for c := range out {
		out[c] = make([]float32, w*h)
	}

	dim := f.hdr.groupDim()
	ups := int(f.hdr.upsampling)
	gx, gy := f.hdr.sizeGroups()

	var batch [xorshiftLanes]uint64

	for g := range gx * gy {
		bx := (g % gx) * ups * dim
		by := (g / gx) * ups * dim

		for iy := range ups {
			for ix := range ups {
				x0, y0 := bx+ix*dim, by+iy*dim
				if x0 >= w || y0 >= h {
					continue
				}

				nx, ny := min(dim, w-x0), min(dim, h-y0)

				rng := newXorshift128(uint32(d.visible), uint32(d.nonvisible),
					uint32(x0), uint32(y0))

				for c := range out {
					for y := range ny {
						row := out[c][(y0+y)*w+x0:]

						for b := 0; b < nx; b += floatsPerBatch {
							rng.fill(&batch)

							for i := range min(floatsPerBatch, nx-b) {
								bits := uint32(batch[i/2])
								if i%2 != 0 {
									bits = uint32(batch[i/2] >> 32)
								}

								row[b+i] = math.Float32frombits(bits>>9 | 0x3F800000)
							}
						}
					}
				}
			}
		}
	}

	for c := range out {
		out[c] = convolveNoise(out[c], w, h)
	}

	return out
}

func convolveNoise(src []float32, w, h int) []float32 {
	dst := make([]float32, w*h)

	for y := range h {
		for x := range w {
			var others float32

			for iy := range 5 {
				row := mirror(y+iy-2, h) * w

				for ix := range 5 {
					if iy == 2 && ix == 2 {
						continue
					}

					others += src[row+mirror(x+ix-2, w)]
				}
			}

			dst[y*w+x] = others*0.16 + src[y*w+x]*-3.84
		}
	}

	return dst
}

func (d *decoder) addNoise(f *frame, planes [][]float32, w, h int) {
	if f.noise.zero() {
		return
	}

	rnd := d.noisePlanes(f, w, h)

	cmap := colorCorrelation{factor: defaultColorFactor, baseB: 1}
	if f.vardct != nil {
		cmap = f.vardct.cmap
	}

	ytox := cmap.yToX(cmap.ytoxLF)
	ytob := cmap.yToB(cmap.ytobLF)

	const (
		normConst = 0.22
		rgCorr    = 0.9921875
		rgnCorr   = 0.0078125
	)

	for i := range w * h {
		vx, vy := planes[0][i], planes[1][i]

		sg := f.noise.strength((vy - vx) * 0.5)
		sr := f.noise.strength((vy + vx) * 0.5)

		red := rnd[0][i] * normConst
		green := rnd[1][i] * normConst
		corr := rnd[2][i] * normConst

		redNoise := sr * (rgnCorr*red + rgCorr*corr)
		greenNoise := sg * (rgnCorr*green + rgCorr*corr)
		rg := redNoise + greenNoise

		planes[0][i] += ytox*rg + redNoise - greenNoise
		planes[1][i] += rg
		planes[2][i] += ytob * rg
	}
}
