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

func (v *varDCT) walkACGroup(rc rect, s *dctScratch, pass int, shift uint32, histIdx int,
	orders *[numOrders * 3][]uint32, fn func(ctx int, packed uint32),
) error {
	for c := range 3 {
		clear(s.nonzeros[pass][c])
	}

	ctxOffset := histIdx * v.bctx.numACContexts()
	offset := 0

	var sbx, sby [3]int

	for by := range rc.h {
		for c := range 3 {
			sby[c] = by >> v.vs[c]
		}

		for bx := range rc.w {
			for c := range 3 {
				sbx[c] = bx >> v.hs[c]
			}

			pos := (rc.y0+by)*v.bw + rc.x0 + bx

			raw := v.transform[pos]
			if raw < 128 {
				continue
			}

			t := hfTransform(raw & 127)
			cx, cy := t.blocks()
			shapeID := int(hfShapeID[t])

			numBlocks := cx * cy
			numCoeffs := numBlocks * blockSize
			logBlocks := ceilLog2(uint32(numBlocks))

			rawQuant := uint32(v.rawQuant[pos])
			quantLF := int(v.quantLF[pos])

			for _, c := range [3]int{1, 0, 2} {
				if sbx[c]<<v.hs[c] != bx || sby[c]<<v.vs[c] != by {
					continue
				}

				nz := s.nonzeros[pass][c]
				stride := rc.w >> v.hs[c]
				predicted := predictNonzeros(nz, stride, sbx[c], sby[c])
				bc := v.bctx.blockContext(quantLF, rawQuant, shapeID, c)

				order := orders[shapeID*3+c]
				cur := s.coeffs[c][offset : offset+numCoeffs]

				nonzeros := 0

				for k := numBlocks; k < numCoeffs; k++ {
					if cur[order[k]] != 0 {
						nonzeros++
					}
				}

				if nonzeros+numBlocks > numCoeffs {
					return ErrInvalid
				}

				fn(v.bctx.nonzeroContext(predicted, bc)+ctxOffset, uint32(nonzeros))

				count := uint32(shiftRightCeil(nonzeros, logBlocks))

				for iy := range cy {
					row := nz[(sby[c]+iy)*stride:]

					for ix := range cx {
						row[sbx[c]+ix] = count
					}
				}

				histOffset := v.bctx.zeroDensityOffset(bc) + ctxOffset

				prev := 1
				if nonzeros > numCoeffs/16 {
					prev = 0
				}

				for k, left := numBlocks, nonzeros; k < numCoeffs && left > 0; k++ {
					coeff := cur[order[k]]

					if coeff>>shift<<shift != coeff {
						return ErrInvalid
					}

					fn(histOffset+zeroDensityContext(left, k, logBlocks, prev), packSigned(coeff>>shift))

					prev = 0
					if coeff != 0 {
						prev = 1
					}

					left -= prev
				}
			}

			offset += numCoeffs
		}
	}

	return nil
}
