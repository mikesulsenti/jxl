package jxl

func (f *frame) encodeLFGlobal(w *bitWriter) {
	if f.hdr.flags&(flagPatches|flagSplines|flagNoise) != 0 {
		w.setErr(ErrUnsupported)

		return
	}

	f.lfQuant.write(w)

	if f.hdr.encoding != encVarDCT {
		w.setErr(ErrUnsupported)

		return
	}

	v := f.vardct

	v.quant.write(w)
	v.bctx.write(w)
	v.cmap.write(w)

	if w.err() != nil {
		return
	}

	w.bit(false)

	if len(f.modular.channels) != 0 {
		w.setErr(ErrUnsupported)
	}
}

func (f *frame) encode(w *bitWriter, m *imageMetadata, imgW, imgH uint32, body [][]byte) {
	if f.hdr.numTOCEntries() != len(body) {
		w.setErr(ErrInvalid)

		return
	}

	f.hdr.write(w, m, imgW, imgH)

	t := toc{entries: make([]uint32, len(body))}

	for i, sec := range body {
		t.entries[i] = uint32(len(sec))
	}

	t.write(w)

	if w.err() != nil {
		return
	}

	for _, sec := range body {
		for _, b := range sec {
			w.bits(8, uint64(b))
		}
	}
}
