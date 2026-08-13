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
