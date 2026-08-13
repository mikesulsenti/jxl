package jxl

func unpackSigned(v uint32) int32 {
	return int32(v>>1) ^ (int32(^v&1) - 1)
}

func (r *bitReader) enum() uint32 {
	return r.u32(dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(6, 18))
}

func (r *bitReader) str() string {
	n := int(r.u32(dVal(0), dBits(4), dBitsOff(5, 16), dBitsOff(10, 48)))
	if n > r.left()/8 {
		r.setErr(ErrInvalid)

		return ""
	}

	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.bits(8))
	}

	return string(b)
}

func (r *bitReader) extensions() {
	selector := r.u64()
	if selector == 0 {
		return
	}

	var total uint64

	for i := range 64 {
		if selector&(1<<i) == 0 {
			continue
		}

		size := r.u64()
		if total+size < total {
			r.setErr(ErrInvalid)

			return
		}

		total += size
	}

	if total > uint64(r.left()) {
		r.setErr(ErrInvalid)

		return
	}

	r.skip(int(total))
}

const (
	aspectUnknown = iota
	aspect1x1
	aspect12x10
	aspect4x3
	aspect3x2
	aspect16x9
	aspect5x4
	aspect2x1
)

func deriveXSize(ysize uint32, ratio uint32, coded uint64) (uint32, bool) {
	var x uint64

	switch ratio {
	case aspectUnknown:
		x = coded
	case aspect1x1:
		x = uint64(ysize)
	case aspect12x10:
		x = uint64(ysize) * 12 / 10
	case aspect4x3:
		x = uint64(ysize) * 4 / 3
	case aspect3x2:
		x = uint64(ysize) * 3 / 2
	case aspect16x9:
		x = uint64(ysize) * 16 / 9
	case aspect5x4:
		x = uint64(ysize) * 5 / 4
	case aspect2x1:
		x = uint64(ysize) * 2
	default:
		return 0, false
	}

	if x == 0 || x > 1<<32-1 {
		return 0, false
	}

	return uint32(x), true
}

type sizeHeader struct {
	xsize uint32
	ysize uint32
}

func (s *sizeHeader) read(r *bitReader) {
	var (
		ysize uint32
		coded uint64
	)

	small := r.bit()
	if small {
		ysize = (uint32(r.bits(5)) + 1) * 8
	} else {
		ysize = r.u32(dBits(9), dBits(13), dBits(18), dBits(30)) + 1
	}

	ratio := r.bits(3)

	if ratio == aspectUnknown {
		if small {
			coded = uint64(r.bits(5)+1) * 8
		} else {
			coded = uint64(r.u32(dBits(9), dBits(13), dBits(18), dBits(30))) + 1
		}
	}

	xsize, ok := deriveXSize(ysize, uint32(ratio), coded)
	if !ok {
		r.setErr(ErrInvalid)

		return
	}

	s.xsize, s.ysize = xsize, ysize
}

type previewHeader struct {
	xsize uint32
	ysize uint32
}

func (p *previewHeader) read(r *bitReader) {
	var (
		ysize uint32
		coded uint64
	)

	div8 := r.bit()
	if div8 {
		ysize = r.u32(dVal(16), dVal(32), dBitsOff(5, 1), dBitsOff(9, 33)) * 8
	} else {
		ysize = r.u32(dBits(6), dBitsOff(8, 64), dBitsOff(10, 320), dBitsOff(12, 1344)) + 1
	}

	ratio := r.bits(3)

	if ratio == aspectUnknown {
		if div8 {
			coded = uint64(r.u32(dVal(16), dVal(32), dBitsOff(5, 1), dBitsOff(9, 33))) * 8
		} else {
			coded = uint64(r.u32(dBits(6), dBitsOff(8, 64), dBitsOff(10, 320), dBitsOff(12, 1344))) + 1
		}
	}

	xsize, ok := deriveXSize(ysize, uint32(ratio), coded)
	if !ok {
		r.setErr(ErrInvalid)

		return
	}

	p.xsize, p.ysize = xsize, ysize
}

type fileHeader struct {
	size      sizeHeader
	meta      imageMetadata
	transform customTransformData
}

func (f *fileHeader) read(r *bitReader) {
	if r.bits(8) != 0xff || r.bits(8) != 0x0a {
		r.setErr(ErrInvalid)

		return
	}

	f.size.read(r)
	f.meta.read(r)
	f.transform.read(r, f.meta.xybEncoded)
}

func (m *imageMetadata) colorChannels() int {
	if m.color.space == csGray {
		return 1
	}

	return 3
}

func (m *imageMetadata) alphaIndex() int {
	for i, ec := range m.extraChannels {
		if ec.ecType == ecAlpha {
			return i
		}
	}

	return -1
}
