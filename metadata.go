package jxl

type bitDepth struct {
	floatSample   bool
	bitsPerSample uint32
	expBits       uint32
}

func defaultBitDepth() bitDepth { return bitDepth{bitsPerSample: 8} }

func (b *bitDepth) read(r *bitReader) {
	*b = defaultBitDepth()

	b.floatSample = r.bit()

	if b.floatSample {
		b.bitsPerSample = r.u32(dVal(32), dVal(16), dVal(24), dBitsOff(6, 1))
		b.expBits = uint32(r.bits(4)) + 1

		mantissa := int32(b.bitsPerSample) - int32(b.expBits) - 1
		if b.expBits < 2 || b.expBits > 8 || mantissa < 2 || mantissa > 23 {
			r.setErr(ErrInvalid)
		}

		return
	}

	b.bitsPerSample = r.u32(dVal(8), dVal(10), dVal(12), dBitsOff(6, 1))

	if b.bitsPerSample > 31 {
		r.setErr(ErrInvalid)
	}
}

type animation struct {
	tpsNumerator   uint32
	tpsDenominator uint32
	numLoops       uint32
	haveTimecodes  bool
}

func (a *animation) read(r *bitReader) {
	a.tpsNumerator = r.u32(dVal(100), dVal(1000), dBitsOff(10, 1), dBitsOff(30, 1))
	a.tpsDenominator = r.u32(dVal(1), dVal(1001), dBitsOff(8, 1), dBitsOff(10, 1))
	a.numLoops = r.u32(dVal(0), dBits(3), dBits(16), dBits(32))
	a.haveTimecodes = r.bit()
}

type toneMapping struct {
	intensityTarget      float32
	minNits              float32
	relativeToMaxDisplay bool
	linearBelow          float32
}

func defaultToneMapping() toneMapping { return toneMapping{intensityTarget: 255} }

func (t *toneMapping) read(r *bitReader) {
	*t = defaultToneMapping()

	if r.bit() {
		return
	}

	t.intensityTarget = r.f16()
	t.minNits = r.f16()
	t.relativeToMaxDisplay = r.bit()
	t.linearBelow = r.f16()

	if t.intensityTarget <= 0 || t.minNits < 0 || t.minNits > t.intensityTarget ||
		t.linearBelow < 0 || (t.relativeToMaxDisplay && t.linearBelow > 1) {
		r.setErr(ErrInvalid)
	}
}

type extraChannelType uint32

const (
	ecAlpha extraChannelType = iota
	ecDepth
	ecSpotColor
	ecSelectionMask
	ecBlack
	ecCFA
	ecThermal
	ecReserved0
	ecReserved1
	ecReserved2
	ecReserved3
	ecReserved4
	ecReserved5
	ecReserved6
	ecReserved7
	ecUnknown
	ecOptional
)

type extraChannelInfo struct {
	ecType          extraChannelType
	depth           bitDepth
	dimShift        uint32
	name            string
	alphaAssociated bool
	spotColor       [4]float32
	cfaChannel      uint32
}

func defaultExtraChannelInfo() extraChannelInfo {
	return extraChannelInfo{ecType: ecAlpha, depth: defaultBitDepth(), cfaChannel: 1}
}

func (e *extraChannelInfo) read(r *bitReader) {
	*e = defaultExtraChannelInfo()

	if r.bit() {
		return
	}

	e.ecType = extraChannelType(r.enum())
	if e.ecType > ecOptional {
		r.setErr(ErrInvalid)

		return
	}

	e.depth.read(r)
	e.dimShift = r.u32(dVal(0), dVal(3), dVal(4), dBitsOff(3, 1))
	e.name = r.str()

	switch e.ecType {
	case ecAlpha:
		e.alphaAssociated = r.bit()
	case ecSpotColor:
		for i := range e.spotColor {
			e.spotColor[i] = r.f16()
		}
	case ecCFA:
		e.cfaChannel = r.u32(dVal(1), dBits(2), dBitsOff(4, 3), dBitsOff(8, 19))
	}

	if e.dimShift > 3 {
		r.setErr(ErrInvalid)
	}
}

type orientation uint32

const (
	orIdentity orientation = 1 + iota
	orFlipHorizontal
	orRotate180
	orFlipVertical
	orTranspose
	orRotate90CW
	orAntiTranspose
	orRotate90CCW
)

func (o orientation) transposing() bool {
	switch o {
	case orTranspose, orRotate90CW, orAntiTranspose, orRotate90CCW:
		return true
	}

	return false
}

type imageMetadata struct {
	orient            orientation
	haveIntrinsicSize bool
	intrinsicSize     sizeHeader
	havePreview       bool
	preview           previewHeader
	haveAnimation     bool
	anim              animation
	depth             bitDepth
	modular16Bit      bool
	extraChannels     []extraChannelInfo
	xybEncoded        bool
	color             colorEncoding
	tone              toneMapping
}

func defaultImageMetadata() imageMetadata {
	return imageMetadata{
		orient:       orIdentity,
		depth:        defaultBitDepth(),
		modular16Bit: true,
		xybEncoded:   true,
		color:        defaultColorEncoding(),
		tone:         defaultToneMapping(),
	}
}

func (m *imageMetadata) read(r *bitReader) {
	*m = defaultImageMetadata()

	if r.bit() {
		return
	}

	extraFields := r.bit()

	if extraFields {
		m.orient = orientation(r.bits(3) + 1)

		if m.haveIntrinsicSize = r.bit(); m.haveIntrinsicSize {
			m.intrinsicSize.read(r)
		}
		if m.havePreview = r.bit(); m.havePreview {
			m.preview.read(r)
		}
		if m.haveAnimation = r.bit(); m.haveAnimation {
			m.anim.read(r)
		}
	}

	m.depth.read(r)
	m.modular16Bit = r.bit()

	n := r.u32(dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(12, 1))
	if n > 256 || int(n) > r.left() {
		r.setErr(ErrInvalid)

		return
	}

	m.extraChannels = make([]extraChannelInfo, n)
	for i := range m.extraChannels {
		m.extraChannels[i].read(r)
	}

	m.xybEncoded = r.bit()
	m.color.read(r)

	if extraFields {
		m.tone.read(r)
	}

	r.extensions()
	m.color.check(r)
}

func (o orientation) displayPixel(x, y, w, h int) (int, int) {
	switch o {
	case orFlipHorizontal:
		return w - 1 - x, y
	case orRotate180:
		return w - 1 - x, h - 1 - y
	case orFlipVertical:
		return x, h - 1 - y
	case orTranspose:
		return y, x
	case orRotate90CW:
		return h - 1 - y, x
	case orAntiTranspose:
		return h - 1 - y, w - 1 - x
	case orRotate90CCW:
		return y, w - 1 - x
	}

	return x, y
}
