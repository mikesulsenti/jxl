package jxl

import "math"

func quantizeBias(c int, target float32, biases *[4]float32) int32 {
	if target != target {
		return 0
	}

	target = min(max(target, -1<<30), 1<<30)

	near := int32(math.Round(float64(target)))
	best := near
	bestErr := float32(math.Inf(1))

	for cand := near - 1; cand <= near+1; cand++ {
		e := adjustQuantBias(c, cand, biases) - target

		if e < 0 {
			e = -e
		}

		if e < bestErr {
			best, bestErr = cand, e
		}
	}

	return best
}

func (v *varDCT) quantizeBlock(s *dctScratch, t hfTransform, invGlobalScale float32, quant uint32,
	xcc, bcc float32, numCoeffs, offset int,
) {
	biases := &v.biases

	scaledY := invGlobalScale / float32(quant)
	scaledX := scaledY * v.xdm
	scaledB := scaledY * v.bdm

	coeffs, in := &s.coeffs, &s.block

	mx := v.dq.matrix(t, 0)
	my := v.dq.matrix(t, 1)
	mb := v.dq.matrix(t, 2)

	for k := range numCoeffs {
		qy := quantizeBias(1, in[1][k]/(my[k]*scaledY), biases)
		dy := adjustQuantBias(1, qy, biases) * my[k] * scaledY

		coeffs[0][offset+k] = quantizeBias(0, (in[0][k]-xcc*dy)/(mx[k]*scaledX), biases)
		coeffs[1][offset+k] = qy
		coeffs[2][offset+k] = quantizeBias(2, (in[2][k]-bcc*dy)/(mb[k]*scaledB), biases)
	}
}
