package jxl

import "math"

func roundHalfAway(v float32) int32 {
	t := int32(v)
	d := v - float32(t)

	if d >= 0.5 {
		return t + 1
	}

	if d <= -0.5 {
		return t - 1
	}

	return t
}

func quantizeBias(c int, target float32, biases *[4]float32) int32 {
	if target != target {
		return 0
	}

	target = min(max(target, -1<<30), 1<<30)

	near := roundHalfAway(target)

	best := near - 1
	bestErr := absf(adjustQuantBias(c, best, biases) - target)

	if e := absf(adjustQuantBias(c, near, biases) - target); e < bestErr {
		best, bestErr = near, e
	}

	if e := absf(adjustQuantBias(c, near+1, biases) - target); e < bestErr {
		best = near + 1
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

	k := 0

	if n := numCoeffs &^ 7; n > 0 {
		a := quantArgs{
			in:     [3]*float32{&in[0][0], &in[1][0], &in[2][0]},
			coeffs: [3]*int32{&coeffs[0][offset], &coeffs[1][offset], &coeffs[2][offset]},
			mat:    [3]*float32{&mx[0], &my[0], &mb[0]},
			n:      n,
			p: [9]float32{biases[0], biases[1], biases[2], biases[3],
				scaledX, scaledY, scaledB, xcc, bcc},
		}

		if quantize(&a) {
			k = n
		}
	}

	for ; k < numCoeffs; k++ {
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

const maxRawQuant = 256

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

func (f *frame) encodeVarDCTLF(w *bitWriter, g int, hdr *groupHeader, tree *maTree) {
	v := f.vardct
	rc := f.lfGroupBlockRect(g)

	if !f.hdr.is444() {
		w.setErr(ErrUnsupported)

		return
	}

	w.bits(2, 0)

	invQuantLF := float32(globalScaleDenom) /
		(float32(v.quant.globalScale) * float32(v.quant.quantLF))

	var fac [3]float32

	for c := range 3 {
		fac[c] = f.lfQuant.factors[c] * invQuantLF
	}

	cflX := v.cmap.yToX(v.cmap.ytoxLF)
	cflB := v.cmap.yToB(v.cmap.ytobLF)

	img := &modImage{bitDepth: f.modular.bitDepth}

	for range 3 {
		img.channels = append(img.channels, newModChannel(rc.w, rc.h, 0, 0))
	}

	for y := range rc.h {
		qy, qx, qb := img.channels[0].row(y), img.channels[1].row(y), img.channels[2].row(y)
		off := (rc.y0+y)*v.bw + rc.x0

		for x := range rc.w {
			qy[x] = int32(math.Round(float64(v.lf[1][off+x] / fac[1])))
			ry := float32(qy[x]) * fac[1]

			qx[x] = int32(math.Round(float64((v.lf[0][off+x] - ry*cflX) / fac[0])))
			qb[x] = int32(math.Round(float64((v.lf[2][off+x] - ry*cflB) / fac[2])))
		}
	}

	img.encodeStream(w, hdr, tree, 1+g)

	if w.err() != nil {
		return
	}

	for y := range rc.h {
		qy, qx, qb := img.channels[0].row(y), img.channels[1].row(y), img.channels[2].row(y)
		off := (rc.y0+y)*v.bw + rc.x0

		for x := range rc.w {
			inX := float32(qx[x]) * fac[0]
			inY := float32(qy[x]) * fac[1]
			inB := float32(qb[x]) * fac[2]

			v.lf[1][off+x] = inY
			v.lf[0][off+x] = inY*cflX + inX
			v.lf[2][off+x] = inY*cflB + inB

			if v.bctx.numLFContexts <= 1 {
				continue
			}

			bucket := countAbove(v.bctx.lfThresholds[0], qx[x])
			bucket *= len(v.bctx.lfThresholds[2]) + 1
			bucket += countAbove(v.bctx.lfThresholds[2], qb[x])
			bucket *= len(v.bctx.lfThresholds[1]) + 1
			bucket += countAbove(v.bctx.lfThresholds[1], qy[x])

			v.quantLF[off+x] = uint8(bucket)
		}
	}
}

func (f *frame) encodeHFMetadata(w *bitWriter, g int, hdr *groupHeader, tree *maTree) {
	v := f.vardct
	rc := f.lfGroupBlockRect(g)
	cw, ch := divCeil(rc.w, 8), divCeil(rc.h, 8)

	var types, quants []int32

	for y := range rc.h {
		for x := range rc.w {
			pos := (rc.y0+y)*v.bw + rc.x0 + x

			if v.transform[pos] < 128 {
				continue
			}

			if v.rawQuant[pos] < 1 || v.rawQuant[pos] > maxRawQuant {
				w.setErr(ErrInvalid)

				return
			}

			types = append(types, int32(v.transform[pos]&127))
			quants = append(quants, v.rawQuant[pos]-1)
		}
	}

	if len(types) == 0 || len(types) > rc.w*rc.h {
		w.setErr(ErrInvalid)

		return
	}

	w.bits(ceilLog2(uint32(rc.w*rc.h)), uint64(len(types)-1))

	img := &modImage{bitDepth: f.modular.bitDepth}
	img.channels = []*modChannel{
		newModChannel(cw, ch, 3, 3),
		newModChannel(cw, ch, 3, 3),
		newModChannel(len(types), 2, 0, 0),
		newModChannel(rc.w, rc.h, 0, 0),
	}

	cx0, cy0 := rc.x0>>3, rc.y0>>3

	for y := range ch {
		rx, rb := img.channels[0].row(y), img.channels[1].row(y)
		off := (cy0+y)*v.cw + cx0

		for x := range cw {
			rx[x] = int32(v.ytox[off+x])
			rb[x] = int32(v.ytob[off+x])
		}
	}

	copy(img.channels[2].row(0), types)
	copy(img.channels[2].row(1), quants)

	for y := range rc.h {
		row := img.channels[3].row(y)
		off := (rc.y0+y)*v.bw + rc.x0

		for x := range rc.w {
			row[x] = int32(v.epf[off+x])
		}
	}

	img.encodeStream(w, hdr, tree, 1+2*f.hdr.numLFGroups()+g)
}

func (f *frame) encodeHFGlobal(w *bitWriter, hist *histograms, counts [][]uint32) [][]prefixCode {
	v := f.vardct

	if f.hdr.pass.numPasses != 1 {
		w.setErr(ErrUnsupported)

		return nil
	}

	v.dq.write(w)

	if w.err() != nil {
		return nil
	}

	w.bits(ceilLog2(uint32(f.hdr.numGroups())), uint64(v.numHistograms-1))
	w.bits(2, 2)

	return hist.write(w, counts)
}

func (f *frame) acContexts() int {
	return int(f.vardct.numHistograms) * f.vardct.bctx.numACContexts()
}

func (f *frame) acValues() [][]uint32 {
	return make([][]uint32, f.acContexts()+zeroDensityContextLimit-zeroDensityContextCount)
}

func (f *frame) collectACGroup(s *dctScratch, g int, values [][]uint32) error {
	v := f.vardct

	return v.walkACGroup(f.blockGroupRect(g), s, 0, 0, 0, &v.passes[0].orders,
		func(ctx int, packed uint32) {
			values[ctx] = append(values[ctx], packed)
		})
}

func (f *frame) finishACHistograms(values [][]uint32) (*histograms, [][]uint32) {
	numContexts := f.acContexts()

	m, clustered := clusterContexts(values[:numContexts], 256)

	for _, vs := range values[numContexts:] {
		clustered[0] = append(clustered[0], vs...)
	}

	return buildHistograms(m, clustered)
}

func (f *frame) buildACHistograms(s *dctScratch, groups []int) (*histograms, [][]uint32, error) {
	values := f.acValues()

	for _, g := range groups {
		if err := f.collectACGroup(s, g, values); err != nil {
			return nil, nil, err
		}
	}

	hist, counts := f.finishACHistograms(values)

	return hist, counts, nil
}

func (f *frame) encodeVarDCTGroup(w *bitWriter, g int, s *dctScratch, hist *histograms,
	sw *symbolWriter, histIdx int,
) error {
	v := f.vardct

	w.bits(ceilLog2(v.numHistograms), uint64(histIdx))

	return v.walkACGroup(f.blockGroupRect(g), s, 0, 0, histIdx, &v.passes[0].orders,
		func(ctx int, packed uint32) {
			sw.writeUnsigned(hist, w, ctx, packed)
		})
}
