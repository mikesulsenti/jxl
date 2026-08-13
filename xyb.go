package jxl

import "math"

type xybParams struct {
	mat            [9]float32
	biasCbrt       [3]float32
	scaledBias     [3]float32
	intensityScale float32
}

func newXYBParams(o *opsinInverseMatrix, intensityTarget float32) xybParams {
	p := xybParams{mat: o.inverse, intensityScale: 255 / intensityTarget}

	for i := range 3 {
		p.biasCbrt[i] = float32(math.Cbrt(float64(o.biases[i])))
		p.scaledBias[i] = o.biases[i] * p.intensityScale
	}

	return p
}

func (p *xybParams) apply(x, y, b float32) (float32, float32, float32) {
	l := y + x - p.biasCbrt[0]
	m := y - x - p.biasCbrt[1]
	s := b - p.biasCbrt[2]

	l = l*l*(l*p.intensityScale) + p.scaledBias[0]
	m = m*m*(m*p.intensityScale) + p.scaledBias[1]
	s = s*s*(s*p.intensityScale) + p.scaledBias[2]

	return p.mat[0]*l + p.mat[1]*m + p.mat[2]*s,
		p.mat[3]*l + p.mat[4]*m + p.mat[5]*s,
		p.mat[6]*l + p.mat[7]*m + p.mat[8]*s
}

var (
	srgbP = [5]float32{
		-5.135152395e-4, 5.287254571e-3, 3.903842876e-1, 1.474205315, 7.352629620e-1,
	}
	srgbQ = [5]float32{
		1.004519624e-2, 3.036675394e-1, 1.340816930, 9.258482155e-1, 2.424867759e-2,
	}
)

func (p *xybParams) srgbValues() [29]float32 {
	var v [29]float32

	for i := range 3 {
		v[i] = p.biasCbrt[i]
		v[4+i] = p.scaledBias[i]
	}

	v[3] = p.intensityScale

	for i := range 9 {
		v[7+i] = p.mat[i]
	}

	for i := range 5 {
		v[16+i] = srgbP[i]
		v[21+i] = srgbQ[i]
	}

	v[26] = 12.92
	v[27] = 0.0031308
	v[28] = math.Float32frombits(0x7fffffff)

	return v
}

func (p *xybParams) srgbConsts() *[29][8]float32 {
	k := new([29][8]float32)

	for i, v := range p.srgbValues() {
		for j := range k[i] {
			k[i][j] = v
		}
	}

	return k
}

func (p *xybParams) srgbConsts512() *[29][16]float32 {
	k := new([29][16]float32)

	for i, v := range p.srgbValues() {
		for j := range k[i] {
			k[i][j] = v
		}
	}

	return k
}

func evalRationalPoly(x float32, p, q *[5]float32) float32 {
	yp := p[4]
	yq := q[4]

	for i := 3; i >= 0; i-- {
		yp = yp*x + p[i]
		yq = yq*x + q[i]
	}

	return yp / yq
}

func linearToSRGB(v float32) float32 {
	sign := float32(1)
	if v < 0 {
		sign, v = -1, -v
	}

	if v < 0.0031308 {
		return sign * v * 12.92
	}

	return sign * evalRationalPoly(float32(math.Sqrt(float64(v))), &srgbP, &srgbQ)
}

func linearToGamma(v, gamma float32) float32 {
	sign := float32(1)
	if v < 0 {
		sign, v = -1, -v
	}

	return sign * float32(math.Pow(float64(v), float64(gamma)))
}

func (c *colorEncoding) fromLinear(v float32) float32 {
	if c.tf.haveGamma {
		return linearToGamma(v, c.tf.gammaValue())
	}

	switch c.tf.tf {
	case tfLinear:
		return v
	case tfSRGB:
		return linearToSRGB(v)
	case tfBT709:
		if v <= 0.018 {
			return v * 4.5
		}

		return 1.099*float32(math.Pow(float64(v), 0.45)) - 0.099
	case tfDCI:
		return linearToGamma(v, 1.0/2.6)
	}

	return v
}
