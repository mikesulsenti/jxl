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

func (l *lfQuantFactors) write(w *bitWriter) {
	if l.factors == lfQuantDefault {
		w.bit(true)

		return
	}

	w.bit(false)

	for _, v := range l.factors {
		if v < 1e-8 {
			w.setErr(ErrInvalid)

			return
		}

		w.f16(v * 128)
	}
}

func (q *quantizerParams) write(w *bitWriter) {
	switch {
	case q.globalScale >= 1 && q.globalScale <= 2048:
		w.bits(2, 0)
		w.bits(11, uint64(q.globalScale-1))
	case q.globalScale <= 4096:
		w.bits(2, 1)
		w.bits(11, uint64(q.globalScale-2049))
	case q.globalScale <= 8192:
		w.bits(2, 2)
		w.bits(12, uint64(q.globalScale-4097))
	case q.globalScale <= 8192+1<<16:
		w.bits(2, 3)
		w.bits(16, uint64(q.globalScale-8193))
	default:
		w.setErr(ErrInvalid)

		return
	}

	switch {
	case q.quantLF == 16:
		w.bits(2, 0)
	case q.quantLF >= 1 && q.quantLF <= 32:
		w.bits(2, 1)
		w.bits(5, uint64(q.quantLF-1))
	case q.quantLF <= 256:
		w.bits(2, 2)
		w.bits(8, uint64(q.quantLF-1))
	case q.quantLF <= 1<<16:
		w.bits(2, 3)
		w.bits(16, uint64(q.quantLF-1))
	default:
		w.setErr(ErrInvalid)
	}
}

func defaultColorCorrelation() colorCorrelation {
	return colorCorrelation{factor: defaultColorFactor, baseB: 1}
}

func (c *colorCorrelation) write(w *bitWriter) {
	if *c == defaultColorCorrelation() {
		w.bit(true)

		return
	}

	w.bit(false)

	switch {
	case c.factor == defaultColorFactor:
		w.bits(2, 0)
	case c.factor == 256:
		w.bits(2, 1)
	case c.factor >= 2 && c.factor <= 257:
		w.bits(2, 2)
		w.bits(8, uint64(c.factor-2))
	case c.factor >= 258 && c.factor <= 257+1<<16:
		w.bits(2, 3)
		w.bits(16, uint64(c.factor-258))
	default:
		w.setErr(ErrInvalid)

		return
	}

	if c.baseX > 4 || c.baseB > 4 || c.ytoxLF < -128 || c.ytoxLF > 127 ||
		c.ytobLF < -128 || c.ytobLF > 127 {
		w.setErr(ErrInvalid)

		return
	}

	w.f16(c.baseX)
	w.f16(c.baseB)
	w.bits(8, uint64(c.ytoxLF+128))
	w.bits(8, uint64(c.ytobLF+128))
}

func writeBlockContextValue(w *bitWriter, u uint32) {
	switch {
	case u < 16:
		w.bits(2, 0)
		w.bits(4, uint64(u))
	case u < 272:
		w.bits(2, 1)
		w.bits(8, uint64(u-16))
	case u < 65808:
		w.bits(2, 2)
		w.bits(16, uint64(u-272))
	default:
		w.bits(2, 3)
		w.bits(32, uint64(u-65808))
	}
}

func writeQuantFactorThreshold(w *bitWriter, u uint32) {
	switch {
	case u < 4:
		w.bits(2, 0)
		w.bits(2, uint64(u))
	case u < 12:
		w.bits(2, 1)
		w.bits(3, uint64(u-4))
	case u < 44:
		w.bits(2, 2)
		w.bits(5, uint64(u-12))
	case u < 300:
		w.bits(2, 3)
		w.bits(8, uint64(u-44))
	default:
		w.setErr(ErrInvalid)
	}
}

func (b *blockContextMap) isDefault() bool {
	if b.numLFContexts != 1 || b.numContexts != 15 || len(b.qfThresholds) != 0 {
		return false
	}

	for c := range 3 {
		if len(b.lfThresholds[c]) != 0 {
			return false
		}
	}

	return len(b.contextMap) == len(defaultBlockContextMap)
}

func (b *blockContextMap) write(w *bitWriter) {
	if b.isDefault() {
		w.bit(true)

		return
	}

	w.bit(false)

	lf := 1

	for c := range 3 {
		t := b.lfThresholds[c]

		if len(t) > 15 {
			w.setErr(ErrInvalid)

			return
		}

		w.bits(4, uint64(len(t)))

		for _, v := range t {
			writeBlockContextValue(w, packSigned(v))
		}

		lf *= len(t) + 1
	}

	if len(b.qfThresholds) > 15 || lf*(len(b.qfThresholds)+1) > 64 {
		w.setErr(ErrInvalid)

		return
	}

	w.bits(4, uint64(len(b.qfThresholds)))

	for _, v := range b.qfThresholds {
		if v == 0 {
			w.setErr(ErrInvalid)

			return
		}

		writeQuantFactorThreshold(w, v-1)
	}

	if len(b.contextMap) != 3*numOrders*lf*(len(b.qfThresholds)+1) {
		w.setErr(ErrInvalid)

		return
	}

	writeContextMap(w, b.contextMap)
}

func (d *dequantMatrices) write(w *bitWriter) {
	for i := range numQuantTables {
		if &d.tables[i][0] != &libraryTables[i][0] {
			w.setErr(ErrUnsupported)

			return
		}
	}

	w.bit(true)
}
