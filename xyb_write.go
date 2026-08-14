package jxl

import "math"

type xybForward struct {
	inv            [9]float32
	biasCbrt       [3]float32
	scaledBias     [3]float32
	intensityScale float32
}

func cbrt32(x float32) float32 {
	if x == 0 || x != x || x > math.MaxFloat32 || x < -math.MaxFloat32 {
		return x
	}

	neg := x < 0
	if neg {
		x = -x
	}

	b := math.Float32bits(x)
	scale := 1.0

	if b < 1<<23 {
		x *= 1 << 30
		b = math.Float32bits(x)
		scale = 1.0 / 1024
	}

	r := float64(math.Float32frombits(0x548C2B4B - b/3))
	d := float64(x)

	const third = 1.0 / 3

	for range 4 {
		r = r * (4 - d*r*r*r) * third
	}

	c := d * r * r
	c = (2*c + d/(c*c)) * third

	y := float32(c * scale)

	if neg {
		return -y
	}

	return y
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

func cbrtPlane(p []float32) {
	for i := cbrtBlock(p); i < len(p); i++ {
		p[i] = cbrt32(p[i])
	}
}

func (f *xybForward) apply(r, g, b float32) (float32, float32, float32) {
	l := f.inv[0]*r + f.inv[1]*g + f.inv[2]*b
	m := f.inv[3]*r + f.inv[4]*g + f.inv[5]*b
	s := f.inv[6]*r + f.inv[7]*g + f.inv[8]*b

	l = cbrt32((l-f.scaledBias[0])/f.intensityScale) + f.biasCbrt[0]
	m = cbrt32((m-f.scaledBias[1])/f.intensityScale) + f.biasCbrt[1]
	s = cbrt32((s-f.scaledBias[2])/f.intensityScale) + f.biasCbrt[2]

	return (l - m) / 2, (l + m) / 2, s
}

func srgbToLinear(v float32) float32 {
	sign := float32(1)

	if v < 0 {
		sign, v = -1, -v
	}

	if v <= 0.04045 {
		return sign * v / 12.92
	}

	return sign * float32(math.Pow((float64(v)+0.055)/1.055, 2.4))
}
