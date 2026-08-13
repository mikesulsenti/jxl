package jxl

import "math"

type xybForward struct {
	inv            [9]float32
	biasCbrt       [3]float32
	scaledBias     [3]float32
	intensityScale float32
}

func invert3x3(m [9]float32) ([9]float32, bool) {
	var out [9]float32

	a := [9]float64{}

	for i, v := range m {
		a[i] = float64(v)
	}

	det := a[0]*(a[4]*a[8]-a[5]*a[7]) - a[1]*(a[3]*a[8]-a[5]*a[6]) +
		a[2]*(a[3]*a[7]-a[4]*a[6])

	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return out, false
	}

	c := [9]float64{
		a[4]*a[8] - a[5]*a[7], a[2]*a[7] - a[1]*a[8], a[1]*a[5] - a[2]*a[4],
		a[5]*a[6] - a[3]*a[8], a[0]*a[8] - a[2]*a[6], a[2]*a[3] - a[0]*a[5],
		a[3]*a[7] - a[4]*a[6], a[1]*a[6] - a[0]*a[7], a[0]*a[4] - a[1]*a[3],
	}

	for i, v := range c {
		out[i] = float32(v / det)
	}

	return out, true
}

func newXYBForward(p *xybParams) (xybForward, bool) {
	inv, ok := invert3x3(p.mat)
	if !ok {
		return xybForward{}, false
	}

	return xybForward{
		inv:            inv,
		biasCbrt:       p.biasCbrt,
		scaledBias:     p.scaledBias,
		intensityScale: p.intensityScale,
	}, true
}

func (f *xybForward) apply(r, g, b float32) (float32, float32, float32) {
	l := f.inv[0]*r + f.inv[1]*g + f.inv[2]*b
	m := f.inv[3]*r + f.inv[4]*g + f.inv[5]*b
	s := f.inv[6]*r + f.inv[7]*g + f.inv[8]*b

	l = float32(math.Cbrt(float64((l-f.scaledBias[0])/f.intensityScale))) + f.biasCbrt[0]
	m = float32(math.Cbrt(float64((m-f.scaledBias[1])/f.intensityScale))) + f.biasCbrt[1]
	s = float32(math.Cbrt(float64((s-f.scaledBias[2])/f.intensityScale))) + f.biasCbrt[2]

	return (l - m) / 2, (l + m) / 2, s
}
