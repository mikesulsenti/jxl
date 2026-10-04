package jxl

import "math"

// RGB primary matrices are derived from CIE xy chromaticities. Standard
// coordinates come from IEC 61966-2-1, ITU-R BT.2020 and SMPTE RP 431-2;
// the conversion procedure is also described by CSS Color 4 section 19.
// XYB's opsin inverse yields linear sRGB, which must be converted to the
// declared primaries/white point before applying that space's transfer curve.
type gamutMatrix [9]float64

func (m gamutMatrix) vector(v [3]float64) (out [3]float64) {
	for r := range 3 {
		out[r] = m[r*3]*v[0] + m[r*3+1]*v[1] + m[r*3+2]*v[2]
	}
	return
}
func (m gamutMatrix) mul(n gamutMatrix) (out gamutMatrix) {
	for r := range 3 {
		for c := range 3 {
			for k := range 3 {
				out[r*3+c] += m[r*3+k] * n[k*3+c]
			}
		}
	}
	return
}
func (m gamutMatrix) inverse() (out gamutMatrix, ok bool) {
	cof := gamutMatrix{m[4]*m[8] - m[5]*m[7], m[2]*m[7] - m[1]*m[8], m[1]*m[5] - m[2]*m[4], m[5]*m[6] - m[3]*m[8], m[0]*m[8] - m[2]*m[6], m[2]*m[3] - m[0]*m[5], m[3]*m[7] - m[4]*m[6], m[1]*m[6] - m[0]*m[7], m[0]*m[4] - m[1]*m[3]}
	det := m[0]*cof[0] + m[1]*cof[3] + m[2]*cof[6]
	if math.IsNaN(det) || math.IsInf(det, 0) || math.Abs(det) < 1e-12 {
		return out, false
	}
	for i, v := range cof {
		out[i] = v / det
	}
	return out, true
}

func rgbXYZ(prim [3][2]float64, white [2]float64) (out gamutMatrix, ok bool) {
	if white[1] <= 0 {
		return out, false
	}
	var p gamutMatrix
	for c, xy := range prim {
		p[c] = xy[0]
		p[3+c] = xy[1]
		p[6+c] = 1 - xy[0] - xy[1]
	}
	inv, ok := p.inverse()
	if !ok {
		return out, false
	}
	s := inv.vector([3]float64{white[0] / white[1], 1, (1 - white[0] - white[1]) / white[1]})
	for r := range 3 {
		for c := range 3 {
			out[r*3+c] = p[r*3+c] * s[c]
		}
	}
	return out, true
}

func (c *colorEncoding) srgbGamut() (out gamutMatrix, identity, ok bool) {
	out = gamutMatrix{1, 0, 0, 0, 1, 0, 0, 0, 1}
	if c.wantICC || c.space == csGray || c.prim == prSRGB && c.white == wpD65 {
		return out, true, true
	}
	d65 := [2]float64{0.3127, 0.3290}
	white := d65
	switch c.white {
	case wpD65:
	case wpE:
		white = [2]float64{1.0 / 3, 1.0 / 3}
	case wpDCI:
		white = [2]float64{0.314, 0.351}
	case wpCustom:
		white = [2]float64{float64(c.whiteXY.x) * 1e-6, float64(c.whiteXY.y) * 1e-6}
	default:
		return out, false, false
	}
	var prim [3][2]float64
	switch c.prim {
	case prSRGB:
		prim = [3][2]float64{{0.64, 0.33}, {0.30, 0.60}, {0.15, 0.06}}
	case prBT2100:
		prim = [3][2]float64{{0.708, 0.292}, {0.170, 0.797}, {0.131, 0.046}}
	case prP3:
		prim = [3][2]float64{{0.680, 0.320}, {0.265, 0.690}, {0.150, 0.060}}
	case prCustom:
		for i, p := range c.customPrim {
			prim[i] = [2]float64{float64(p.x) * 1e-6, float64(p.y) * 1e-6}
		}
	default:
		return out, false, false
	}
	srgb, _ := rgbXYZ([3][2]float64{{0.64, 0.33}, {0.30, 0.60}, {0.15, 0.06}}, d65)
	target, ok := rgbXYZ(prim, white)
	if !ok {
		return out, false, false
	}
	inv, ok := target.inverse()
	if !ok {
		return out, false, false
	}
	adapt := gamutMatrix{1, 0, 0, 0, 1, 0, 0, 0, 1}
	if white != d65 {
		bradford := gamutMatrix{0.8951, 0.2664, -0.1614, -0.7502, 1.7135, 0.0367, 0.0389, -0.0685, 1.0296}
		binv, _ := bradford.inverse()
		src := bradford.vector([3]float64{d65[0] / d65[1], 1, (1 - d65[0] - d65[1]) / d65[1]})
		dst := bradford.vector([3]float64{white[0] / white[1], 1, (1 - white[0] - white[1]) / white[1]})
		var scale gamutMatrix
		for i := range 3 {
			if src[i] == 0 {
				return out, false, false
			}
			scale[4*i] = dst[i] / src[i]
		}
		adapt = binv.mul(scale).mul(bradford)
	}
	return inv.mul(adapt).mul(srgb), false, true
}

func (m gamutMatrix) apply(r, g, b float32) (float32, float32, float32) {
	v := m.vector([3]float64{float64(r), float64(g), float64(b)})
	return float32(v[0]), float32(v[1]), float32(v[2])
}
