package jxl

import "math/bits"

func ceilLog2[T uint32 | uint64](v T) int {
	if v <= 1 {
		return 0
	}

	return bits.Len64(uint64(v) - 1)
}

type hybridUint struct {
	splitToken    uint32
	splitExponent uint32
	msbInToken    uint32
	lsbInToken    uint32
}

func (h *hybridUint) read(r *bitReader, logAlphaSize int) {
	*h = hybridUint{}

	h.splitExponent = uint32(r.bits(ceilLog2(uint32(logAlphaSize) + 1)))
	h.splitToken = 1 << h.splitExponent

	if h.splitExponent != uint32(logAlphaSize) {
		h.msbInToken = uint32(r.bits(ceilLog2(h.splitExponent + 1)))

		if h.msbInToken > h.splitExponent {
			r.setErr(ErrInvalid)

			return
		}

		h.lsbInToken = uint32(r.bits(ceilLog2(h.splitExponent - h.msbInToken + 1)))
	}

	if h.lsbInToken+h.msbInToken > h.splitExponent {
		r.setErr(ErrInvalid)
	}
}

func (h hybridUint) decode(symbol uint32, r *bitReader) uint32 {
	if symbol < h.splitToken {
		return symbol
	}

	inToken := h.lsbInToken + h.msbInToken
	nbits := (h.splitExponent - inToken + ((symbol - h.splitToken) >> inToken)) & 31
	low := symbol & (1<<h.lsbInToken - 1)
	noLow := symbol >> h.lsbInToken
	extra := uint32(r.bitsOpt(int(nbits)))
	hi := (noLow & (1<<h.msbInToken - 1)) | (1 << h.msbInToken)

	return (((hi << nbits) | extra) << h.lsbInToken) | low
}
