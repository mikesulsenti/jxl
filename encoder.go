package jxl

func encodeFilter() restorationFilter {
	f := defaultRestorationFilter()
	f.gab = false
	f.epfIters = 0

	return f
}

func newEncodeFrame(m *imageMetadata, t *customTransformData, w, h int, quant int32) (*frame, error) {
	f := &frame{}

	f.hdr.width, f.hdr.height = uint32(w), uint32(h)
	f.hdr.encoding = encVarDCT
	f.hdr.upsampling = 1
	f.hdr.groupSizeShift = 1
	f.hdr.pass.numPasses = 1
	f.hdr.xQMScale, f.hdr.bQMScale = 3, 2
	f.hdr.isLast = true
	f.hdr.flags = flagSkipAdaptiveLFSmoothing
	f.hdr.filter = encodeFilter()
	f.hdr.ecUpsampling = make([]uint32, len(m.extraChannels))

	for i := range f.hdr.ecUpsampling {
		f.hdr.ecUpsampling[i] = 1
	}

	if f.hdr.numTOCEntries() != 1 {
		return nil, ErrUnsupported
	}

	f.lfQuant.factors = lfQuantDefault
	f.vardct = newVarDCT(f, t.opsin.quant, nil)

	v := f.vardct

	v.dq.tables = libraryTables
	v.bctx = blockContextMap{contextMap: defaultBlockContextMap, numLFContexts: 1, numContexts: 15}
	v.cmap = defaultColorCorrelation()
	v.quant = quantizerParams{globalScale: 4096, quantLF: 16}
	v.numHistograms = 1
	v.xdm = powf(1/1.25, float32(f.hdr.xQMScale)-2)
	v.bdm = powf(1/1.25, float32(f.hdr.bQMScale)-2)

	v.passes = make([]varDCTPass, 1)

	for o := range numOrders {
		for c := range 3 {
			v.passes[0].orders[3*o+c] = naturalOrders[o]
		}
	}

	for i := range v.transform {
		v.transform[i] = 128 | uint8(hfDCT)
		v.rawQuant[i] = quant
	}

	return f, nil
}

func (f *frame) fillXYB(fwd *xybForward, rgb [3][]float32, w, h int) {
	v := f.vardct
	stride := v.bw * blockDim

	for y := range v.bh * blockDim {
		sy := min(y, h-1)

		for x := range stride {
			sx := min(x, w-1)
			i := sy*w + sx

			cx, cy, cb := fwd.apply(rgb[0][i], rgb[1][i], rgb[2][i])

			v.pixels[0][y*stride+x] = cx
			v.pixels[1][y*stride+x] = cy
			v.pixels[2][y*stride+x] = cb
		}
	}
}

func (f *frame) transformBlocks(s *dctScratch) bool {
	v := f.vardct
	stride := v.bw * blockDim

	for by := range v.bh {
		for bx := range v.bw {
			b := by*v.bw + bx

			for c := range 3 {
				var buf, lf [blockSize]float32

				for y := range blockDim {
					copy(buf[y*blockDim:],
						v.pixels[c][(by*blockDim+y)*stride+bx*blockDim:][:blockDim])
				}

				if !transformFromPixels(hfDCT, buf[:], lf[:], s.scratch) {
					return false
				}

				copy(s.block[c], buf[:])
				v.lf[c][b] = lf[0]
			}

			v.quantizeBlock(s, hfDCT, v.quant.invGlobalScale(), uint32(v.rawQuant[b]),
				v.cmap.yToX(0), v.cmap.yToB(0), blockSize, b*blockSize)

			for c := range 3 {
				s.coeffs[c][b*blockSize] = 0
			}
		}
	}

	return true
}

func (f *frame) encodeBody(s *dctScratch) ([]byte, error) {
	hist, counts, err := f.buildACHistograms(s, []int{0})
	if err != nil {
		return nil, err
	}

	var w bitWriter

	hdr := groupHeader{wp: defaultWeightedHeader()}
	tree := &maTree{nodes: []treeNode{{property: -1 - predGradient, child: 1}}}

	f.encodeLFGlobal(&w)
	f.encodeVarDCTLF(&w, 0, &hdr, tree)
	f.encodeHFMetadata(&w, 0, &hdr, tree)

	codes := f.encodeHFGlobal(&w, hist, counts)

	if err := w.err(); err != nil {
		return nil, err
	}

	hist.resize(f.acContexts() + zeroDensityContextLimit - zeroDensityContextCount)

	sw := symbolWriter{codes: codes}

	if err := f.encodeVarDCTGroup(&w, 0, s, hist, &sw, 0); err != nil {
		return nil, err
	}

	if err := w.err(); err != nil {
		return nil, err
	}

	return w.bytes(), nil
}

func encodeVarDCTImage(rgb [3][]float32, w, h int, quant int32) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, ErrInvalid
	}

	for c := range 3 {
		if len(rgb[c]) < w*h {
			return nil, ErrInvalid
		}
	}

	var head fileHeader

	head.size = sizeHeader{xsize: uint32(w), ysize: uint32(h)}
	head.meta = defaultImageMetadata()
	head.transform = defaultCustomTransformData()

	f, err := newEncodeFrame(&head.meta, &head.transform, w, h, quant)
	if err != nil {
		return nil, err
	}

	params := newXYBParams(&head.transform.opsin, head.meta.tone.intensityTarget)

	fwd, ok := newXYBForward(&params)
	if !ok {
		return nil, ErrInvalid
	}

	f.fillXYB(&fwd, rgb, w, h)

	var scratch dctScratch

	scratch.alloc(f)

	if !f.transformBlocks(&scratch) {
		return nil, ErrUnsupported
	}

	body, err := f.encodeBody(&scratch)
	if err != nil {
		return nil, err
	}

	var out bitWriter

	head.write(&out)
	f.encode(&out, &head.meta, uint32(w), uint32(h), body)

	if err := out.err(); err != nil {
		return nil, err
	}

	return out.bytes(), nil
}
