package jxl

func (p *passes) write(w *bitWriter) {
	w.u32(p.numPasses, dVal(1), dVal(2), dVal(3), dBitsOff(3, 4))

	if p.numPasses == 1 {
		return
	}

	w.u32(p.numDS, dVal(0), dVal(1), dVal(2), dBitsOff(1, 3))

	for _, s := range p.shift {
		w.bits(2, uint64(s))
	}

	for _, d := range p.downsample {
		w.u32(d, dVal(1), dVal(2), dVal(4), dVal(8))
	}

	for _, l := range p.lastPass {
		w.u32(l, dVal(0), dVal(1), dVal(2), dBits(3))
	}
}

func (b *blendingInfo) write(w *bitWriter, numExtra uint32, fullFrame bool) {
	w.u32(uint32(b.mode), dVal(0), dVal(1), dVal(2), dBitsOff(2, 3))

	usesAlpha := b.mode == bmBlend || b.mode == bmAlphaWeightedAdd

	if numExtra > 0 && usesAlpha {
		w.u32(b.alphaChannel, dVal(0), dVal(1), dVal(2), dBitsOff(3, 3))
	}

	if (numExtra > 0 && usesAlpha) || b.mode == bmMul {
		w.bit(b.clamp)
	}

	if !(fullFrame && b.mode == bmReplace) {
		w.u32(b.source, dVal(0), dVal(1), dVal(2), dVal(3))
	}
}

func (f *restorationFilter) write(w *bitWriter, enc frameEncoding) {
	d := defaultRestorationFilter()

	if *f == d {
		w.bit(true)

		return
	}

	w.bit(false)
	w.bit(f.gab)

	if f.gab {
		custom := f.gabXWeight1 != d.gabXWeight1 || f.gabXWeight2 != d.gabXWeight2 ||
			f.gabYWeight1 != d.gabYWeight1 || f.gabYWeight2 != d.gabYWeight2 ||
			f.gabBWeight1 != d.gabBWeight1 || f.gabBWeight2 != d.gabBWeight2

		w.bit(custom)

		if custom {
			w.f16(f.gabXWeight1)
			w.f16(f.gabXWeight2)
			w.f16(f.gabYWeight1)
			w.f16(f.gabYWeight2)
			w.f16(f.gabBWeight1)
			w.f16(f.gabBWeight2)
		}
	}

	w.bits(2, uint64(f.epfIters))

	if f.epfIters > 0 && enc == encVarDCT {
		custom := f.epfSharpLUT != d.epfSharpLUT

		w.bit(custom)

		if custom {
			for _, v := range f.epfSharpLUT {
				w.f16(v)
			}
		}
	}

	if f.epfIters > 0 {
		custom := f.epfChannelScale != d.epfChannelScale ||
			f.epfPass1Zeroflush != d.epfPass1Zeroflush ||
			f.epfPass2Zeroflush != d.epfPass2Zeroflush

		w.bit(custom)

		if custom {
			for _, v := range f.epfChannelScale {
				w.f16(v)
			}

			w.f16(f.epfPass1Zeroflush)
			w.f16(f.epfPass2Zeroflush)
		}
	}

	if f.epfIters > 0 {
		custom := f.epfPass0SigmaScale != d.epfPass0SigmaScale ||
			f.epfPass2SigmaScale != d.epfPass2SigmaScale ||
			f.epfBorderSadMul != d.epfBorderSadMul ||
			(enc == encVarDCT && f.epfQuantMul != d.epfQuantMul)

		w.bit(custom)

		if custom {
			if enc == encVarDCT {
				w.f16(f.epfQuantMul)
			}

			w.f16(f.epfPass0SigmaScale)
			w.f16(f.epfPass2SigmaScale)
			w.f16(f.epfBorderSadMul)
		}
	}

	if f.epfIters > 0 && enc == encModular {
		w.f16(f.epfSigmaForModular)
	}

	w.extensions()
}

func (h *frameHeader) isDefault() bool {
	if h.ftype != ftRegular || h.encoding != encVarDCT || h.flags != 0 || h.doYCbCr ||
		h.jpegUpsampling != [3]uint32{} || h.upsampling != 1 || h.groupSizeShift != 1 ||
		h.xQMScale != 3 || h.bQMScale != 2 || h.pass.numPasses != 1 || h.lfLevel != 0 ||
		h.haveCrop || h.duration != 0 || h.timecode != 0 || !h.isLast ||
		h.saveAsReference != 0 || h.saveBeforeCT || h.name != "" {
		return false
	}

	if h.blending != (blendingInfo{}) {
		return false
	}

	for _, b := range h.ecBlending {
		if b != (blendingInfo{}) {
			return false
		}
	}

	for _, u := range h.ecUpsampling {
		if u != 1 {
			return false
		}
	}

	return h.filter == defaultRestorationFilter()
}

func (h *frameHeader) write(w *bitWriter, m *imageMetadata, imgW, imgH uint32) {
	h.deriveFullFrame(imgW, imgH)
	h.deriveRefs()
	h.finish(imgW, imgH)

	w.zeroPad()

	if h.isDefault() {
		w.bit(true)

		return
	}

	w.bit(false)

	numExtra := uint32(len(m.extraChannels))

	w.bits(2, uint64(h.ftype))
	w.bits(1, uint64(h.encoding))
	w.u64(h.flags)

	if !m.xybEncoded {
		w.bit(h.doYCbCr)
	}

	if h.doYCbCr && h.flags&flagUseLFFrame == 0 {
		for _, u := range h.jpegUpsampling {
			w.bits(2, uint64(u))
		}
	}

	if h.flags&flagUseLFFrame == 0 {
		w.u32(h.upsampling, dVal(1), dVal(2), dVal(4), dVal(8))

		for i, u := range h.ecUpsampling {
			if h.upsampling > 1 {
				u >>= m.extraChannels[i].dimShift
			}

			w.u32(u, dVal(1), dVal(2), dVal(4), dVal(8))
		}
	}

	if h.encoding == encModular {
		w.bits(2, uint64(h.groupSizeShift))
	}

	if h.encoding == encVarDCT && m.xybEncoded {
		w.bits(3, uint64(h.xQMScale))
		w.bits(3, uint64(h.bQMScale))
	}

	if h.ftype != ftReferenceOnly {
		h.pass.write(w)
	}

	if h.ftype == ftLF {
		w.u32(h.lfLevel, dVal(1), dVal(2), dVal(3), dVal(4))
	}

	if h.ftype != ftLF {
		w.bit(h.haveCrop)
	}

	crop := d4(dBits(8), dBitsOff(11, 256), dBitsOff(14, 2304), dBitsOff(30, 18688))

	if h.haveCrop && h.ftype != ftReferenceOnly {
		w.u32(packSigned(h.x0), crop[0], crop[1], crop[2], crop[3])
		w.u32(packSigned(h.y0), crop[0], crop[1], crop[2], crop[3])
	}

	if h.haveCrop {
		w.u32(h.frameWidth, crop[0], crop[1], crop[2], crop[3])
		w.u32(h.frameHeight, crop[0], crop[1], crop[2], crop[3])
	}

	normal := h.ftype == ftRegular || h.ftype == ftSkipProgressive

	if normal {
		h.blending.write(w, numExtra, h.fullFrame)

		for i := range h.ecBlending {
			h.ecBlending[i].write(w, numExtra, h.fullFrame)
		}
	}

	if normal && m.haveAnimation {
		w.u32(h.duration, dVal(0), dVal(1), dBits(8), dBits(32))

		if m.anim.haveTimecodes {
			w.bits(32, uint64(h.timecode))
		}
	}

	if normal {
		w.bit(h.isLast)
	}

	if h.ftype != ftLF && !h.isLast {
		w.bits(2, uint64(h.saveAsReference))
	}

	saveBeforeCTDefFalse := h.canBeReferenced && h.blending.mode == bmReplace &&
		h.fullFrame && normal

	if h.ftype == ftReferenceOnly || saveBeforeCTDefFalse {
		w.bit(h.saveBeforeCT)
	}

	w.str(h.name)
	h.filter.write(w, h.encoding)
	w.extensions()
}
