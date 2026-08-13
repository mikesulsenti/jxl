package jxl

type patchBlendMode uint8

const (
	pbNone patchBlendMode = iota
	pbReplace
	pbAdd
	pbMul
	pbBlendAbove
	pbBlendBelow
	pbAlphaWeightedAddAbove
	pbAlphaWeightedAddBelow
	numPatchBlendModes
)

func (m patchBlendMode) usesAlpha() bool {
	return m >= pbBlendAbove
}

func (m patchBlendMode) usesClamp() bool {
	return m.usesAlpha() || m == pbMul
}

const (
	pctNumRefPatch = iota
	pctReferenceFrame
	pctPatchSize
	pctReferencePosition
	pctPosition
	pctBlendMode
	pctOffset
	pctCount
	pctAlphaChannel
	pctClamp
	numPatchContexts
)

type patchBlending struct {
	mode  patchBlendMode
	alpha int
	clamp bool
}

type patchRef struct {
	ref          int
	x0, y0, w, h int
}

type patchPos struct {
	x, y   int
	refIdx int
}

type patchDict struct {
	positions []patchPos
	refs      []patchRef
	blendings []patchBlending
	stride    int
}

func (p *patchDict) read(r *bitReader, xsize, ysize, numEC int, refs *[4]*refFrame) {
	*p = patchDict{stride: numEC + 1}

	var h histograms

	h.read(r, numPatchContexts, true)

	if r.err() != nil {
		return
	}

	var sr symbolReader

	sr.init(&h, r, 0)

	numRefPatch := int(sr.readUnsigned(&h, r, pctNumRefPatch))

	maxRefPatches := 1024 + xsize*ysize/4
	maxPatches := maxRefPatches * 4

	if numRefPatch > maxRefPatches {
		r.setErr(ErrInvalid)

		return
	}

	total := 0

	for range numRefPatch {
		ref := int(sr.readUnsigned(&h, r, pctReferenceFrame))
		if ref >= len(refs) {
			r.setErr(ErrInvalid)

			return
		}

		x0 := int(sr.readUnsigned(&h, r, pctReferencePosition))
		y0 := int(sr.readUnsigned(&h, r, pctReferencePosition))
		w := int(sr.readUnsigned(&h, r, pctPatchSize)) + 1
		hh := int(sr.readUnsigned(&h, r, pctPatchSize)) + 1

		if r.err() != nil {
			return
		}

		rf := refs[ref]
		if rf == nil || !rf.beforeCT || x0+w > rf.w || y0+hh > rf.h {
			r.setErr(ErrInvalid)

			return
		}

		count := int(sr.readUnsigned(&h, r, pctCount)) + 1
		total += count

		if count > maxPatches+1 || total > maxPatches {
			r.setErr(ErrInvalid)

			return
		}

		for i := range count {
			pos := patchPos{refIdx: len(p.refs)}

			if i == 0 {
				pos.x = int(sr.readUnsigned(&h, r, pctPosition))
				pos.y = int(sr.readUnsigned(&h, r, pctPosition))
			} else {
				last := p.positions[len(p.positions)-1]

				dx := int(sr.readSigned(&h, r, pctOffset))
				dy := int(sr.readSigned(&h, r, pctOffset))

				if last.x+dx < 0 || last.y+dy < 0 {
					r.setErr(ErrInvalid)

					return
				}

				pos.x, pos.y = last.x+dx, last.y+dy
			}

			if r.err() != nil {
				return
			}

			if pos.x+w > xsize || pos.y+hh > ysize {
				r.setErr(ErrInvalid)

				return
			}

			for range p.stride {
				var b patchBlending

				mode := sr.readUnsigned(&h, r, pctBlendMode)
				if mode >= uint32(numPatchBlendModes) {
					r.setErr(ErrInvalid)

					return
				}

				b.mode = patchBlendMode(mode)

				if b.mode.usesAlpha() && p.stride > 2 {
					b.alpha = int(sr.readUnsigned(&h, r, pctAlphaChannel))

					if b.alpha >= numEC {
						r.setErr(ErrInvalid)

						return
					}
				}

				if b.mode.usesClamp() {
					b.clamp = sr.readUnsigned(&h, r, pctClamp) != 0
				}

				p.blendings = append(p.blendings, b)
			}

			p.positions = append(p.positions, pos)
		}

		p.refs = append(p.refs, patchRef{ref: ref, x0: x0, y0: y0, w: w, h: hh})
	}

	if err := sr.checkFinalState(&h, r); err != nil {
		r.setErr(err)
	}
}

func (d *decoder) applyPatches(f *frame, planes [][]float32, w, h int) {
	p := &f.patches
	if len(p.positions) == 0 {
		return
	}

	numEC := len(d.hdr.meta.extraChannels)
	nc := len(planes) - numEC

	var tmp []float32

	for idx, pos := range p.positions {
		rp := p.refs[pos.refIdx]

		src := d.preCT[rp.ref]
		if src == nil {
			continue
		}

		bl := p.blendings[idx*p.stride:]

		if len(tmp) < numEC*rp.w {
			tmp = make([]float32, numEC*rp.w)
		}

		for y := range rp.h {
			if pos.y+y >= h {
				break
			}

			n := min(rp.w, w-pos.x)
			if n <= 0 {
				break
			}

			d.performBlending(planes, (pos.y+y)*w+pos.x, src.planes, (rp.y0+y)*src.w+rp.x0,
				n, nc, bl[0], bl[1:p.stride], tmp)
		}
	}
}

func (d *decoder) performBlending(bg [][]float32, bi int, fg [][]float32, fi int,
	n, nc int, color patchBlending, ec []patchBlending, tmp []float32,
) {
	m := &d.hdr.meta
	numEC := len(m.extraChannels)

	simple := color.mode == pbNone

	for _, b := range ec {
		if b.mode != pbNone {
			simple = false
		}
	}

	if simple {
		return
	}

	if color.mode == pbReplace {
		simple = true

		for _, b := range ec {
			if b.mode != pbReplace && b.mode != pbNone {
				simple = false
			}
		}

		if simple {
			for c := range nc {
				copy(bg[c][bi:bi+n], fg[c][fi:fi+n])
			}

			for i := range numEC {
				if ec[i].mode == pbReplace {
					copy(bg[nc+i][bi:bi+n], fg[nc+i][fi:fi+n])
				}
			}

			return
		}
	}

	hasAlpha := m.alphaIndex() >= 0

	for i := range numEC {
		copy(tmp[i*n:(i+1)*n], bg[nc+i][bi:bi+n])
	}

	oldAlpha := func(a int) []float32 { return tmp[a*n : (a+1)*n] }

	for i := range numEC {
		a := ec[i].alpha
		clamp := ec[i].clamp
		assoc := m.extraChannels[a].alphaAssociated

		out := bg[nc+i][bi : bi+n]
		in := fg[nc+i][fi : fi+n]

		switch ec[i].mode {
		case pbAdd:
			for x := range n {
				out[x] += in[x]
			}
		case pbBlendAbove:
			fa := fg[nc+a][fi : fi+n]

			switch {
			case i == a:
				for x := range n {
					t := maybeClamp(fa[x], clamp)
					out[x] = 1 - (1-t)*(1-out[x])
				}
			case assoc:
				for x := range n {
					out[x] = in[x] + out[x]*(1-maybeClamp(fa[x], clamp))
				}
			default:
				oa := oldAlpha(a)

				for x := range n {
					t := maybeClamp(fa[x], clamp)
					na := 1 - (1-t)*(1-oa[x])
					rn := float32(0)

					if na > 0 {
						rn = 1 / na
					}

					out[x] = (in[x]*t + out[x]*oa[x]*(1-t)) * rn
				}
			}
		case pbBlendBelow:
			fa := fg[nc+a][fi : fi+n]
			oa := oldAlpha(a)

			switch {
			case i == a:
				for x := range n {
					b := maybeClamp(out[x], clamp)
					out[x] = 1 - (1-b)*(1-in[x])
				}
			case assoc:
				for x := range n {
					out[x] += in[x] * (1 - maybeClamp(oa[x], clamp))
				}
			default:
				for x := range n {
					b := maybeClamp(oa[x], clamp)
					na := 1 - (1-b)*(1-fa[x])
					rn := float32(0)

					if na > 0 {
						rn = 1 / na
					}

					out[x] = (out[x]*b + in[x]*fa[x]*(1-b)) * rn
				}
			}
		case pbAlphaWeightedAddAbove:
			if i == a {
				break
			}

			fa := fg[nc+a][fi : fi+n]

			for x := range n {
				out[x] += in[x] * maybeClamp(fa[x], clamp)
			}
		case pbAlphaWeightedAddBelow:
			if i == a {
				copy(out, in)

				break
			}

			oa := oldAlpha(a)

			for x := range n {
				out[x] = in[x] + out[x]*maybeClamp(oa[x], clamp)
			}
		case pbMul:
			for x := range n {
				out[x] *= maybeClamp(in[x], clamp)
			}
		case pbReplace:
			copy(out, in)
		}
	}

	a := color.alpha
	clamp := color.clamp

	switch color.mode {
	case pbAdd:
		for c := range nc {
			out, in := bg[c][bi:bi+n], fg[c][fi:fi+n]

			for x := range n {
				out[x] += in[x]
			}
		}
	case pbAlphaWeightedAddAbove, pbAlphaWeightedAddBelow:
		below := color.mode == pbAlphaWeightedAddBelow

		for c := range nc {
			out, in := bg[c][bi:bi+n], fg[c][fi:fi+n]

			if !hasAlpha {
				for x := range n {
					out[x] += in[x]
				}

				continue
			}

			w := fg[nc+a][fi : fi+n]
			if below {
				w = oldAlpha(a)
			}

			for x := range n {
				t := maybeClamp(w[x], clamp)

				if below {
					out[x] = in[x] + out[x]*t
				} else {
					out[x] += in[x] * t
				}
			}
		}
	case pbBlendAbove, pbBlendBelow:
		above := color.mode == pbBlendAbove

		if !hasAlpha {
			if above {
				for c := range nc {
					copy(bg[c][bi:bi+n], fg[c][fi:fi+n])
				}
			}

			break
		}

		assoc := m.extraChannels[a].alphaAssociated
		oa := oldAlpha(a)
		fa := fg[nc+a][fi : fi+n]
		outA := bg[nc+a][bi : bi+n]

		for x := range n {
			ta, ba := fa[x], oa[x]
			if !above {
				ta, ba = ba, fa[x]
			}

			ta = maybeClamp(ta, clamp)

			na := 1 - (1-ta)*(1-ba)
			rn := float32(0)

			if na > 0 {
				rn = 1 / na
			}

			for c := range nc {
				tc, bc := fg[c][fi+x], bg[c][bi+x]
				if !above {
					tc, bc = bc, tc
				}

				if assoc {
					bg[c][bi+x] = tc + bc*(1-ta)
				} else {
					bg[c][bi+x] = (tc*ta + bc*ba*(1-ta)) * rn
				}
			}

			outA[x] = na
		}
	case pbMul:
		for c := range nc {
			out, in := bg[c][bi:bi+n], fg[c][fi:fi+n]

			for x := range n {
				out[x] *= maybeClamp(in[x], clamp)
			}
		}
	case pbReplace:
		for c := range nc {
			copy(bg[c][bi:bi+n], fg[c][fi:fi+n])
		}
	}
}
