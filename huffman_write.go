package jxl

type prefixCode struct {
	code uint16
	bits uint8
}

func writeSimplePrefix(w *bitWriter, alSize int, symbols []uint16, special bool) {
	if len(symbols) < 1 || len(symbols) > 4 || alSize < 2 {
		w.setErr(ErrInvalid)

		return
	}

	maxBits := ceilLog2(uint32(alSize))

	for i, s := range symbols {
		if int(s) >= alSize {
			w.setErr(ErrInvalid)

			return
		}

		for _, o := range symbols[:i] {
			if o == s {
				w.setErr(ErrInvalid)

				return
			}
		}
	}

	w.bits(2, 1)
	w.bits(2, uint64(len(symbols)-1))

	for _, s := range symbols {
		w.bits(maxBits, uint64(s))
	}

	if len(symbols) == 4 {
		w.bit(special)
	}
}

func prefixCodes(t *huffTable, alSize int) ([]prefixCode, bool) {
	out := make([]prefixCode, alSize)
	seen := make([]bool, alSize)

	for i, e := range t.entries {
		if e.bits > huffmanTableBits {
			return nil, false
		}

		s := int(e.value)

		if s >= alSize || seen[s] {
			continue
		}

		seen[s] = true
		out[s] = prefixCode{code: uint16(i), bits: e.bits}
	}

	return out, true
}

func (w *bitWriter) symbol(p prefixCode) { w.bits(int(p.bits), uint64(p.code)) }
