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

	if len(f.modular.channels) == 0 {
		return
	}

	hdr := groupHeader{wp: defaultWeightedHeader()}
	hdr.write(w)

	if w.err() != nil {
		return
	}

	if f.beginC == 0 {
		return
	}

	f.modular.encodeCoded(w, &hdr, channelTree(f.beginC, predGradient), 0, f.beginC)
}

func (f *frame) setAlpha(a *alphaSource, w, h int) {
	c := newModChannel(w, h, 0, 0)

	for y := range h {
		a.rows(y, c.row(y))
	}

	f.modular.channels = []*modChannel{c}
	f.beginC = f.modular.firstGroupedChannel(f.hdr.groupDim())
}

func (f *frame) encodeModularGroup(w *bitWriter, rc rect, minShift, maxShift, streamID int) {
	var sub modImage

	for c := f.beginC; c < len(f.modular.channels); c++ {
		fc := f.modular.channels[c]

		if shift := min(fc.hshift, fc.vshift); shift > maxShift || shift < minShift {
			continue
		}

		cr := rect{rc.x0 >> fc.hshift, rc.y0 >> fc.vshift,
			rc.w >> fc.hshift, rc.h >> fc.vshift}

		cr.w = min(cr.w, fc.w-cr.x0)
		cr.h = min(cr.h, fc.h-cr.y0)

		if cr.w <= 0 || cr.h <= 0 {
			continue
		}

		dst := newModChannel(cr.w, cr.h, fc.hshift, fc.vshift)

		for y := range cr.h {
			copy(dst.row(y), fc.row(cr.y0 + y)[cr.x0:cr.x0+cr.w])
		}

		sub.channels = append(sub.channels, dst)
	}

	if len(sub.channels) == 0 {
		return
	}

	hdr := groupHeader{wp: defaultWeightedHeader()}

	sub.encodeStream(w, &hdr, channelTree(len(sub.channels), predGradient), streamID)
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

	total := 0

	for _, sec := range body {
		total += len(sec)
	}

	w.reserve(total)

	for _, sec := range body {
		w.writeBytes(sec)
	}
}
