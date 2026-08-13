package jxl

func aspectFor(xsize, ysize uint32) (uint32, bool) {
	for ratio := uint32(aspect1x1); ratio <= aspect2x1; ratio++ {
		if x, ok := deriveXSize(ysize, ratio, 0); ok && x == xsize {
			return ratio, true
		}
	}

	return aspectUnknown, false
}

func (s *sizeHeader) write(w *bitWriter) {
	ratio, keyed := aspectFor(s.xsize, s.ysize)

	small := s.ysize%8 == 0 && s.ysize/8-1 < 32
	if !keyed {
		small = small && s.xsize%8 == 0 && s.xsize/8-1 < 32
	}

	w.bit(small)

	if small {
		w.bits(5, uint64(s.ysize/8-1))
	} else {
		w.u32(s.ysize-1, dBits(9), dBits(13), dBits(18), dBits(30))
	}

	w.bits(3, uint64(ratio))

	if keyed {
		return
	}

	if small {
		w.bits(5, uint64(s.xsize/8-1))
	} else {
		w.u32(s.xsize-1, dBits(9), dBits(13), dBits(18), dBits(30))
	}
}

func (p *previewHeader) write(w *bitWriter) {
	ratio, keyed := aspectFor(p.xsize, p.ysize)

	div8 := p.ysize%8 == 0
	if !keyed {
		div8 = div8 && p.xsize%8 == 0
	}

	w.bit(div8)

	if div8 {
		w.u32(p.ysize/8, dVal(16), dVal(32), dBitsOff(5, 1), dBitsOff(9, 33))
	} else {
		w.u32(p.ysize-1, dBits(6), dBitsOff(8, 64), dBitsOff(10, 320), dBitsOff(12, 1344))
	}

	w.bits(3, uint64(ratio))

	if keyed {
		return
	}

	if div8 {
		w.u32(p.xsize/8, dVal(16), dVal(32), dBitsOff(5, 1), dBitsOff(9, 33))
	} else {
		w.u32(p.xsize-1, dBits(6), dBitsOff(8, 64), dBitsOff(10, 320), dBitsOff(12, 1344))
	}
}

func (o *opsinInverseMatrix) write(w *bitWriter) {
	if *o == defaultOpsinInverseMatrix() {
		w.bit(true)

		return
	}

	w.bit(false)

	for _, v := range o.inverse {
		w.f16(v)
	}

	for _, v := range o.biases {
		w.f16(v)
	}

	for _, v := range o.quant {
		w.f16(v)
	}
}

func (c *customTransformData) write(w *bitWriter, xybEncoded bool) {
	if *c == defaultCustomTransformData() {
		w.bit(true)

		return
	}

	w.bit(false)

	if xybEncoded {
		c.opsin.write(w)
	}

	var mask uint64

	if c.weights2 != defaultKern2 {
		mask |= 1
	}

	if c.weights4 != defaultKern4 {
		mask |= 2
	}

	if c.weights8 != defaultKern8 {
		mask |= 4
	}

	w.bits(3, mask)

	if mask&1 != 0 {
		for _, v := range c.weights2 {
			w.f16(v)
		}
	}

	if mask&2 != 0 {
		for _, v := range c.weights4 {
			w.f16(v)
		}
	}

	if mask&4 != 0 {
		for _, v := range c.weights8 {
			w.f16(v)
		}
	}
}

func (f *fileHeader) write(w *bitWriter) {
	w.bits(8, 0xff)
	w.bits(8, 0x0a)

	f.size.write(w)
	f.meta.write(w)
	f.transform.write(w, f.meta.xybEncoded)
}
