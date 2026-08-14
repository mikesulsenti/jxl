package jxl

type toc struct {
	permuted    bool
	permutation []uint32
	entries     []uint32
	offsets     []uint32
}

func (t *toc) read(r *bitReader, numEntries int) {
	*t = toc{}

	t.permuted = r.bit()

	if numEntries < 0 || numEntries > r.left() {
		r.setErr(ErrInvalid)

		return
	}

	if t.permuted {
		t.permutation = readTOCPermutation(r, numEntries)

		if r.err() != nil {
			return
		}
	}

	r.zeroPad()

	t.entries = make([]uint32, numEntries)
	for i := range t.entries {
		t.entries[i] = r.u32(dBits(10), dBitsOff(14, 1024), dBitsOff(22, 17408), dBitsOff(30, 4211712))
	}

	r.zeroPad()

	t.offsets = make([]uint32, numEntries+1)
	for i, e := range t.entries {
		t.offsets[i+1] = t.offsets[i] + e

		if t.offsets[i+1] < t.offsets[i] {
			r.setErr(ErrInvalid)

			return
		}
	}
}

func (t *toc) write(w *bitWriter) {
	if t.permuted {
		w.setErr(ErrUnsupported)

		return
	}

	w.bit(false)
	w.zeroPad()

	for _, e := range t.entries {
		w.u32(e, dBits(10), dBitsOff(14, 1024), dBitsOff(22, 17408), dBitsOff(30, 4211712))
	}

	w.zeroPad()
}
