package jxl

func (w *bitWriter) enum(v uint32) {
	w.u32(v, dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(6, 18))
}

func (w *bitWriter) str(s string) {
	w.u32(uint32(len(s)), dVal(0), dBits(4), dBitsOff(5, 16), dBitsOff(10, 48))

	for i := range len(s) {
		w.bits(8, uint64(s[i]))
	}
}

func (w *bitWriter) extensions() { w.u64(0) }

func (b *bitDepth) write(w *bitWriter) {
	w.bit(b.floatSample)

	if b.floatSample {
		w.u32(b.bitsPerSample, dVal(32), dVal(16), dVal(24), dBitsOff(6, 1))
		w.bits(4, uint64(b.expBits-1))

		return
	}

	w.u32(b.bitsPerSample, dVal(8), dVal(10), dVal(12), dBitsOff(6, 1))
}

func (a *animation) write(w *bitWriter) {
	w.u32(a.tpsNumerator, dVal(100), dVal(1000), dBitsOff(10, 1), dBitsOff(30, 1))
	w.u32(a.tpsDenominator, dVal(1), dVal(1001), dBitsOff(8, 1), dBitsOff(10, 1))
	w.u32(a.numLoops, dVal(0), dBits(3), dBits(16), dBits(32))
	w.bit(a.haveTimecodes)
}

func (t *toneMapping) write(w *bitWriter) {
	if *t == defaultToneMapping() {
		w.bit(true)

		return
	}

	w.bit(false)
	w.f16(t.intensityTarget)
	w.f16(t.minNits)
	w.bit(t.relativeToMaxDisplay)
	w.f16(t.linearBelow)
}

func (e *extraChannelInfo) write(w *bitWriter) {
	if *e == defaultExtraChannelInfo() {
		w.bit(true)

		return
	}

	w.bit(false)
	w.enum(uint32(e.ecType))
	e.depth.write(w)
	w.u32(e.dimShift, dVal(0), dVal(3), dVal(4), dBitsOff(3, 1))
	w.str(e.name)

	switch e.ecType {
	case ecAlpha:
		w.bit(e.alphaAssociated)
	case ecSpotColor:
		for _, v := range e.spotColor {
			w.f16(v)
		}
	case ecCFA:
		w.u32(e.cfaChannel, dVal(1), dBits(2), dBitsOff(4, 3), dBitsOff(8, 19))
	}
}

func (m *imageMetadata) isDefault() bool {
	if len(m.extraChannels) != 0 {
		return false
	}

	d := defaultImageMetadata()

	return m.orient == d.orient &&
		m.haveIntrinsicSize == d.haveIntrinsicSize && m.intrinsicSize == d.intrinsicSize &&
		m.havePreview == d.havePreview && m.preview == d.preview &&
		m.haveAnimation == d.haveAnimation && m.anim == d.anim &&
		m.depth == d.depth && m.modular16Bit == d.modular16Bit &&
		m.xybEncoded == d.xybEncoded && m.color == d.color && m.tone == d.tone
}

func (m *imageMetadata) write(w *bitWriter) {
	if m.isDefault() {
		w.bit(true)

		return
	}

	extraFields := m.orient != orIdentity || m.haveIntrinsicSize ||
		m.havePreview || m.haveAnimation || m.tone != defaultToneMapping()

	w.bit(false)
	w.bit(extraFields)

	if extraFields {
		w.bits(3, uint64(m.orient-1))

		w.bit(m.haveIntrinsicSize)
		if m.haveIntrinsicSize {
			m.intrinsicSize.write(w)
		}

		w.bit(m.havePreview)
		if m.havePreview {
			m.preview.write(w)
		}

		w.bit(m.haveAnimation)
		if m.haveAnimation {
			m.anim.write(w)
		}
	}

	m.depth.write(w)
	w.bit(m.modular16Bit)

	w.u32(uint32(len(m.extraChannels)), dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(12, 1))

	for i := range m.extraChannels {
		m.extraChannels[i].write(w)
	}

	w.bit(m.xybEncoded)
	m.color.write(w)

	if extraFields {
		m.tone.write(w)
	}

	w.extensions()
}
