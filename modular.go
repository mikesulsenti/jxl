package jxl

type modChannel struct {
	w, h           int
	hshift, vshift int
	data           []int32
}

func newModChannel(w, h, hshift, vshift int) *modChannel {
	return &modChannel{w: w, h: h, hshift: hshift, vshift: vshift, data: make([]int32, w*h)}
}

func (c *modChannel) row(y int) []int32 { return c.data[y*c.w : (y+1)*c.w] }

func (c *modChannel) equivalent(o *modChannel) bool {
	return c.w == o.w && c.h == o.h && c.hshift == o.hshift && c.vshift == o.vshift
}

type modScratch struct {
	props []int32
	refs  []int32
	wp    wpState
}

type modImage struct {
	channels []*modChannel
	nbMeta   int
	bitDepth uint32
	threads  int
	arena    *groupScratch
}

func (m *modImage) newChannel(w, h, hshift, vshift int) *modChannel {
	if m.arena != nil {
		return m.arena.channel(w, h, hshift, vshift)
	}

	return newModChannel(w, h, hshift, vshift)
}

const (
	xformRCT = iota
	xformPalette
	xformSqueeze
	xformInvalid
)

type squeezeParams struct {
	horizontal bool
	inPlace    bool
	beginC     uint32
	numC       uint32
}

type modTransform struct {
	id          uint32
	beginC      uint32
	rctType     uint32
	numChannels uint32
	numColors   uint32
	numDeltas   uint32
	predictorID uint32
	squeezes    []squeezeParams
}

func (t *modTransform) read(r *bitReader) {
	*t = modTransform{rctType: 6, numChannels: 3, numColors: 256}

	t.id = uint32(r.bits(2))
	if t.id == xformInvalid {
		r.setErr(ErrInvalid)

		return
	}

	beginDist := d4(dBits(3), dBitsOff(6, 8), dBitsOff(10, 72), dBitsOff(13, 1096))

	if t.id == xformRCT || t.id == xformPalette {
		t.beginC = r.u32(beginDist[0], beginDist[1], beginDist[2], beginDist[3])
	}

	switch t.id {
	case xformRCT:
		t.rctType = r.u32(dVal(6), dBits(2), dBitsOff(4, 2), dBitsOff(6, 10))

		if t.rctType >= 42 {
			r.setErr(ErrInvalid)
		}
	case xformPalette:
		t.numChannels = r.u32(dVal(1), dVal(3), dVal(4), dBitsOff(13, 1))
		t.numColors = r.u32(dBits(8), dBitsOff(10, 256), dBitsOff(12, 1280), dBitsOff(16, 5376))
		t.numDeltas = r.u32(dVal(0), dBitsOff(8, 1), dBitsOff(10, 257), dBitsOff(16, 1281))
		t.predictorID = uint32(r.bits(4))

		if t.predictorID >= numPredictors {
			r.setErr(ErrInvalid)
		}
	case xformSqueeze:
		n := r.u32(dVal(0), dBitsOff(4, 1), dBitsOff(6, 9), dBitsOff(8, 41))

		if int(n) > r.left() {
			r.setErr(ErrInvalid)

			return
		}

		t.squeezes = make([]squeezeParams, n)
		for i := range t.squeezes {
			s := &t.squeezes[i]
			s.horizontal = r.bit()
			s.inPlace = r.bit()
			s.beginC = r.u32(beginDist[0], beginDist[1], beginDist[2], beginDist[3])
			s.numC = r.u32(dVal(1), dVal(2), dVal(3), dBitsOff(4, 4))
		}
	}
}

type groupHeader struct {
	useGlobalTree bool
	wp            weightedHeader
	transforms    []modTransform
}

func (g *groupHeader) read(r *bitReader) {
	*g = groupHeader{}

	g.useGlobalTree = r.bit()
	g.wp.read(r)

	n := r.u32(dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(8, 18))
	if int(n) > r.left() {
		r.setErr(ErrInvalid)

		return
	}

	g.transforms = make([]modTransform, n)
	for i := range g.transforms {
		g.transforms[i].read(r)

		if r.err() != nil {
			return
		}
	}
}

func divCeil(a, b int) int { return (a + b - 1) / b }

const maxFirstPreviewSize = 8

func defaultSqueezeParams(m *modImage) []squeezeParams {
	var out []squeezeParams

	nb := len(m.channels) - m.nbMeta
	if nb < 1 {
		return nil
	}

	w := m.channels[m.nbMeta].w
	h := m.channels[m.nbMeta].h
	wide := w > h

	if nb > 2 && m.channels[m.nbMeta+1].w == w && m.channels[m.nbMeta+1].h == h {
		out = append(out,
			squeezeParams{horizontal: true, beginC: uint32(m.nbMeta) + 1, numC: 2},
			squeezeParams{horizontal: false, beginC: uint32(m.nbMeta) + 1, numC: 2})
	}

	p := squeezeParams{beginC: uint32(m.nbMeta), numC: uint32(nb), inPlace: true}

	if !wide && h > maxFirstPreviewSize {
		p.horizontal = false
		out = append(out, p)
		h = divCeil(h, 2)
	}

	for w > maxFirstPreviewSize || h > maxFirstPreviewSize {
		if w > maxFirstPreviewSize {
			p.horizontal = true
			out = append(out, p)
			w = divCeil(w, 2)
		}
		if h > maxFirstPreviewSize {
			p.horizontal = false
			out = append(out, p)
			h = divCeil(h, 2)
		}
	}

	return out
}

func (m *modImage) metaApply(r *bitReader, transforms []modTransform) {
	for i := range transforms {
		t := &transforms[i]

		switch t.id {
		case xformRCT:
			if !m.checkEqual(r, int(t.beginC), 3) {
				return
			}
		case xformPalette:
			m.metaPalette(r, t)
		case xformSqueeze:
			if len(t.squeezes) == 0 {
				t.squeezes = defaultSqueezeParams(m)
			}

			m.metaSqueeze(r, t.squeezes)
		}

		if r.err() != nil {
			return
		}
	}
}

func (m *modImage) checkEqual(r *bitReader, first, num int) bool {
	if first < 0 || first+num > len(m.channels) {
		r.setErr(ErrInvalid)

		return false
	}

	for i := 1; i < num; i++ {
		if !m.channels[first].equivalent(m.channels[first+i]) {
			r.setErr(ErrInvalid)

			return false
		}
	}

	return true
}

func (m *modImage) metaPalette(r *bitReader, t *modTransform) {
	begin := int(t.beginC)
	num := int(t.numChannels)
	end := begin + num - 1

	if num == 0 || end >= len(m.channels) {
		r.setErr(ErrInvalid)

		return
	}
	if !m.checkEqual(r, begin, num) {
		return
	}

	if begin >= m.nbMeta {
		m.nbMeta++
	} else {
		if end >= m.nbMeta {
			r.setErr(ErrInvalid)

			return
		}

		m.nbMeta += 2 - num
	}

	m.channels = append(m.channels[:begin+1], m.channels[end+1:]...)

	palette := m.newChannel(int(t.numColors+t.numDeltas), num, -1, -1)
	m.channels = append([]*modChannel{palette}, m.channels...)
}

func (m *modImage) metaSqueeze(r *bitReader, params []squeezeParams) {
	for _, p := range params {
		begin := int(p.beginC)
		end := begin + int(p.numC) - 1

		if p.numC == 0 || end >= len(m.channels) {
			r.setErr(ErrInvalid)

			return
		}

		if begin < m.nbMeta {
			if end >= m.nbMeta || !p.inPlace {
				r.setErr(ErrInvalid)

				return
			}

			m.nbMeta += int(p.numC)
		}

		offset := len(m.channels)
		if p.inPlace {
			offset = end + 1
		}

		for c := begin; c <= end; c++ {
			ch := m.channels[c]

			if ch.hshift > 30 || ch.vshift > 30 || ch.w == 0 || ch.h == 0 {
				r.setErr(ErrInvalid)

				return
			}

			var w, h int

			if p.horizontal {
				w = ch.w - divCeil(ch.w, 2)
				h = ch.h
				ch.w = divCeil(ch.w, 2)
				ch.hshift++
			} else {
				w = ch.w
				h = ch.h - divCeil(ch.h, 2)
				ch.h = divCeil(ch.h, 2)
				ch.vshift++
			}

			ch.data = make([]int32, ch.w*ch.h)

			res := m.newChannel(w, h, ch.hshift, ch.vshift)
			at := offset + (c - begin)

			m.channels = append(m.channels, nil)
			copy(m.channels[at+1:], m.channels[at:])
			m.channels[at] = res
		}
	}
}

func (m *modImage) inverse(r *bitReader, transforms []modTransform, wp *weightedHeader) {
	for i := len(transforms) - 1; i >= 0; i-- {
		t := &transforms[i]

		switch t.id {
		case xformRCT:
			m.invRCT(r, int(t.beginC), t.rctType)
		case xformSqueeze:
			m.invSqueeze(r, t.squeezes)
		case xformPalette:
			m.invPalette(r, t, wp)
		}

		if r.err() != nil {
			return
		}
	}
}

func (m *modImage) invRCT(r *bitReader, begin int, rctType uint32) {
	if !m.checkEqual(r, begin, 3) {
		return
	}
	if rctType == 0 {
		return
	}

	perm := int(rctType / 7)
	custom := rctType % 7

	i0 := begin + perm%3
	i1 := begin + (perm+1+perm/3)%3
	i2 := begin + (perm+2-perm/3)%3

	if custom == 0 {
		c0, c1, c2 := m.channels[begin], m.channels[begin+1], m.channels[begin+2]
		m.channels[i0], m.channels[i1], m.channels[i2] = c0, c1, c2

		return
	}

	second, third := custom>>1, custom&1

	in0, in1, in2 := m.channels[begin], m.channels[begin+1], m.channels[begin+2]
	out0, out1, out2 := m.channels[i0], m.channels[i1], m.channels[i2]

	w, h := in0.w, in0.h
	tmp0 := make([]int32, w)
	tmp1 := make([]int32, w)
	tmp2 := make([]int32, w)

	for y := range h {
		r0, r1, r2 := in0.row(y), in1.row(y), in2.row(y)

		for x := range w {
			if custom == 6 {
				yy, co, cg := r0[x], r1[x], r2[x]
				t := yy - cg>>1
				g := cg + t
				b := t - co>>1
				tmp0[x], tmp1[x], tmp2[x] = b+co, g, b

				continue
			}

			first, sec, thd := r0[x], r1[x], r2[x]

			if third != 0 {
				thd += first
			}

			switch second {
			case 1:
				sec += first
			case 2:
				sec += (first + thd) >> 1
			}

			tmp0[x], tmp1[x], tmp2[x] = first, sec, thd
		}

		copy(out0.row(y), tmp0)
		copy(out1.row(y), tmp1)
		copy(out2.row(y), tmp2)
	}
}

func smoothTendency(b, a, n int32) int32 {
	var diff int32

	switch {
	case b >= a && a >= n:
		diff = (4*b - 3*n - a + 6) / 12

		if diff-(diff&1) > 2*(b-a) {
			diff = 2*(b-a) + 1
		}
		if diff+(diff&1) > 2*(a-n) {
			diff = 2 * (a - n)
		}
	case b <= a && a <= n:
		diff = (4*b - 3*n - a - 6) / 12

		if diff+(diff&1) < 2*(b-a) {
			diff = 2*(b-a) - 1
		}
		if diff-(diff&1) < 2*(a-n) {
			diff = 2 * (a - n)
		}
	}

	return diff
}

func (m *modImage) invSqueeze(r *bitReader, params []squeezeParams) {
	for i := len(params) - 1; i >= 0; i-- {
		p := params[i]
		begin := int(p.beginC)
		end := begin + int(p.numC) - 1

		if p.numC == 0 || end >= len(m.channels) {
			r.setErr(ErrInvalid)

			return
		}

		offset := len(m.channels) + begin - end - 1
		if p.inPlace {
			offset = end + 1
		}
		if offset < 0 || offset+int(p.numC) > len(m.channels) {
			r.setErr(ErrInvalid)

			return
		}

		if begin < m.nbMeta {
			m.nbMeta -= int(p.numC)
		}

		for c := begin; c <= end; c++ {
			rc := offset + c - begin

			if m.channels[c].w < m.channels[rc].w || m.channels[c].h < m.channels[rc].h {
				r.setErr(ErrInvalid)

				return
			}

			if p.horizontal {
				m.channels[c] = m.invHSqueeze(m.channels[c], m.channels[rc])
			} else {
				m.channels[c] = m.invVSqueeze(m.channels[c], m.channels[rc])
			}
		}

		m.channels = append(m.channels[:offset], m.channels[offset+int(p.numC):]...)
	}
}

func (m *modImage) invHSqueeze(in, res *modChannel) *modChannel {
	if res.w == 0 {
		in.hshift--

		return in
	}

	out := newModChannel(in.w+res.w, in.h, in.hshift-1, in.vshift)

	if res.h == 0 {
		return out
	}

	overRows(m.threads, in.h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			pres, pavg, pout := res.row(y), in.row(y), out.row(y)

			for x := range res.w {
				avg := pavg[x]

				nextAvg := avg
				if x+1 < in.w {
					nextAvg = pavg[x+1]
				}

				left := avg
				if x > 0 {
					left = pout[(x<<1)-1]
				}

				diff := pres[x] + smoothTendency(left, avg, nextAvg)
				a := avg + diff/2
				pout[x<<1] = a
				pout[(x<<1)+1] = a - diff
			}

			if out.w&1 != 0 {
				pout[out.w-1] = pavg[in.w-1]
			}
		}
	})

	return out
}

func (m *modImage) invVSqueeze(in, res *modChannel) *modChannel {
	if res.h == 0 {
		in.vshift--

		return in
	}

	out := newModChannel(in.w, in.h+res.h, in.hshift, in.vshift-1)

	if res.w == 0 {
		return out
	}

	overRows(m.threads, in.w, func(x0, x1 int) {
		for y := range res.h {
			pres, pavg, pout, pnout := res.row(y), in.row(y), out.row(y<<1), out.row((y<<1)+1)

			pnavg := pavg
			if y+1 < in.h {
				pnavg = in.row(y + 1)
			}

			ppout := pavg
			if y > 0 {
				ppout = out.row((y << 1) - 1)
			}

			for x := x0; x < x1; x++ {
				diff := pres[x] + smoothTendency(ppout[x], pavg[x], pnavg[x])
				v := pavg[x] + diff/2
				pout[x] = v
				pnout[x] = v - diff
			}
		}
	})

	if out.h&1 != 0 {
		copy(out.row((in.h-1)<<1), in.row(in.h-1))
	}

	return out
}

func (m *modImage) decodeStream(r *bitReader, hdr *groupHeader, globalTree *maTree, streamID int) {
	local := hdr

	if local == nil {
		local = &groupHeader{}
		local.read(r)

		if r.err() != nil {
			return
		}

		m.metaApply(r, local.transforms)

		if r.err() != nil {
			return
		}
	}

	m.decodeCoded(r, local, globalTree, streamID, len(m.channels))

	if r.err() != nil {
		return
	}

	if hdr == nil {
		m.inverse(r, local.transforms, &local.wp)
	}
}

func (m *modImage) decodeCoded(r *bitReader, hdr *groupHeader, globalTree *maTree, streamID, upto int) {
	empty := true
	for _, c := range m.channels[:upto] {
		if c.w != 0 && c.h != 0 {
			empty = false

			break
		}
	}

	if empty {
		return
	}

	tree := globalTree

	if !hdr.useGlobalTree {
		samples := 0
		for _, c := range m.channels[:upto] {
			samples += c.w * c.h
		}

		tree = &maTree{}
		tree.read(r, min(1024+samples, 1<<20))

		if r.err() != nil {
			return
		}
	}

	if tree == nil || len(tree.nodes) == 0 {
		r.setErr(ErrInvalid)

		return
	}

	imageWidth := 0
	for _, c := range m.channels[:upto] {
		imageWidth = max(imageWidth, c.w)
	}

	var sr symbolReader

	sr.init(&tree.histograms, r, imageWidth)

	var scratch modScratch

	for i, c := range m.channels[:upto] {
		if c.w == 0 || c.h == 0 {
			continue
		}

		m.decodeChannel(r, &sr, tree, &hdr.wp, i, streamID, &scratch)

		if r.err() != nil {
			return
		}
	}

	if err := sr.checkFinalState(&tree.histograms, r); err != nil {
		r.setErr(err)
	}
}

func predictionAt(c *modChannel, x, y int) predData {
	row := c.row(y)
	rowTop, rowTopTop := row, row

	if y > 0 {
		rowTop = c.row(y - 1)
	}
	if y > 1 {
		rowTopTop = c.row(y - 2)
	}

	var d predData

	switch {
	case x > 0:
		d.left = row[x-1]
	case y > 0:
		d.left = rowTop[0]
	}

	d.top = d.left
	if y > 0 {
		d.top = rowTop[x]
	}

	d.topleft = d.left
	if x > 0 && y > 0 {
		d.topleft = rowTop[x-1]
	}

	d.topright = d.top
	if x+1 < c.w && y > 0 {
		d.topright = rowTop[x+1]
	}

	d.leftleft = d.left
	if x > 1 {
		d.leftleft = row[x-2]
	}

	d.toptop = d.top
	if y > 1 {
		d.toptop = rowTopTop[x]
	}

	d.toprightright = d.topright
	if x+2 < c.w && y > 0 {
		d.toprightright = rowTop[x+2]
	}

	return d
}

func (s *modScratch) grow(p *[]int32, n int) []int32 {
	if cap(*p) < n {
		*p = make([]int32, n)
	}

	*p = (*p)[:n]

	return *p
}

func (m *modImage) precomputeReferences(chan_ int, y int, refs []int32, numRefs int) {
	if numRefs == 0 {
		return
	}

	for i := range refs {
		refs[i] = 0
	}

	cur := m.channels[chan_]
	offset := 0

	for i := range chan_ {
		if offset >= numRefs {
			break
		}

		j := chan_ - i - 1
		o := m.channels[j]

		if !o.equivalent(cur) {
			continue
		}

		rowCur := o.row(y)
		rowPrev := rowCur

		if y > 0 {
			rowPrev = o.row(y - 1)
		}

		for x := range cur.w {
			base := x*numRefs + offset
			v := rowCur[x]
			refs[base] = absInt32(v)
			refs[base+1] = v

			var vleft int32
			if x > 0 {
				vleft = rowCur[x-1]
			}

			vtop := vleft
			if y > 0 {
				vtop = rowPrev[x]
			}

			vtopleft := vleft
			if x > 0 && y > 0 {
				vtopleft = rowPrev[x-1]
			}

			pred := clampedGradient(int64(vleft), int64(vtop), int64(vtopleft))
			refs[base+2] = int32(absInt64(int64(v) - pred))
			refs[base+3] = int32(int64(v) - pred)
		}

		offset += 4
	}
}

func absInt32(v int32) int32 {
	m := v >> 31

	return (v ^ m) - m
}

func (m *modImage) decodeChannel(r *bitReader, sr *symbolReader, tree *maTree,
	wph *weightedHeader, chan_ int, streamID int, scratch *modScratch,
) {
	c := m.channels[chan_]

	numRefs := 0
	if tree.numProperties > numNonRefProperties {
		numRefs = divCeil(tree.numProperties-numNonRefProperties, propertiesPerPrevChan) *
			propertiesPerPrevChan
	}

	props := scratch.grow(&scratch.props, max(256, numNonRefProperties+numRefs))
	refs := scratch.grow(&scratch.refs, c.w*numRefs)
	wp := &scratch.wp

	wp.reset(wph, c.w)

	base := props[:numNonRefProperties:numNonRefProperties]

	base[0] = int32(chan_)
	base[1] = int32(streamID)

	for y := range c.h {
		m.precomputeReferences(chan_, y, refs, numRefs)
		base[9] = 0

		row := c.row(y)

		rowTop, rowTopTop := row, row
		if y > 0 {
			rowTop = c.row(y - 1)
		}
		if y > 1 {
			rowTopTop = c.row(y - 2)
		}

		for x := range c.w {
			var d predData

			if y > 1 && x > 1 && x+2 < c.w {
				d = predData{
					left:          row[x-1],
					top:           rowTop[x],
					topleft:       rowTop[x-1],
					topright:      rowTop[x+1],
					leftleft:      row[x-2],
					toptop:        rowTopTop[x],
					toprightright: rowTop[x+2],
				}
			} else {
				d = predictionAt(c, x, y)
			}

			base[2] = int32(y)
			base[3] = int32(x)
			base[4] = absInt32(d.top)
			base[5] = absInt32(d.left)
			base[6] = d.top
			base[7] = d.left
			base[8] = d.left - base[9]
			base[9] = d.left + d.top - d.topleft
			base[10] = d.left - d.topleft
			base[11] = d.topleft - d.top
			base[12] = d.top - d.topright
			base[13] = d.top - d.toptop
			base[14] = d.left - d.leftleft

			var wpPred int64

			if tree.usesWP {
				var wpProp int32

				wpPred, wpProp = wp.predictAndProperty(x, y, &d)
				base[wpProperty] = wpProp
			}

			if numRefs != 0 {
				copy(props[numNonRefProperties:numNonRefProperties+numRefs], refs[x*numRefs:])
			}

			leaf := tree.walk(props)
			guess := predictOne(leaf.predictor(), d, wpPred) + int64(leaf.val)
			dec := sr.readSigned(&tree.histograms, r, int(leaf.id))
			val := int32(guess + int64(leaf.child)*int64(dec))

			if tree.usesWP {
				wp.updateErrors(val, x, y)
			}

			row[x] = val
		}

		if r.err() != nil {
			return
		}
	}
}

const (
	paletteLargeCube   = 5
	paletteSmallCube   = 4
	paletteSmallBits   = 2
	paletteLargeOffset = paletteSmallCube * paletteSmallCube * paletteSmallCube
	paletteRGBChannels = 3
)

func paletteScale(value uint64, bitDepth uint32) int32 {
	return int32((value * (uint64(1)<<bitDepth - 1)) >> 2)
}

func paletteValue(pal *modChannel, index int32, c int, paletteSize int, bitDepth uint32) int32 {
	switch {
	case index < 0:
		if c >= paletteRGBChannels {
			return 0
		}

		i := -(index + 1)
		i %= 1 + 2*(int32(len(deltaPalette))-1)

		result := int32(deltaPalette[(i+1)>>1][c])
		if i&1 == 0 {
			result = -result
		}
		if bitDepth > 8 {
			result *= 1 << (bitDepth - 8)
		}

		return result

	case int(index) >= paletteSize && int(index) < paletteSize+paletteLargeOffset:
		if c >= paletteRGBChannels {
			return 0
		}

		i := (int(index) - paletteSize) >> (c * paletteSmallBits)

		return paletteScale(uint64(i%paletteSmallCube), bitDepth) +
			1<<max(0, int(bitDepth)-3)

	case int(index) >= paletteSize+paletteLargeOffset:
		if c >= paletteRGBChannels {
			return 0
		}

		i := int(index) - paletteSize - paletteLargeOffset

		switch c {
		case 1:
			i /= paletteLargeCube
		case 2:
			i /= paletteLargeCube * paletteLargeCube
		}

		return paletteScale(uint64(i%paletteLargeCube), bitDepth)
	}

	return pal.row(c)[index]
}

func (m *modImage) invPalette(r *bitReader, t *modTransform, wph *weightedHeader) {
	if m.nbMeta < 1 {
		r.setErr(ErrInvalid)

		return
	}

	pal := m.channels[0]
	nb := pal.h
	c0 := int(t.beginC) + 1

	if nb < 1 || c0 >= len(m.channels) {
		r.setErr(ErrInvalid)

		return
	}

	idx := m.channels[c0]
	w, h := idx.w, idx.h

	extra := make([]*modChannel, nb-1)
	for i := range extra {
		extra[i] = m.newChannel(w, h, idx.hshift, idx.vshift)
	}

	m.channels = append(m.channels[:c0+1], append(extra, m.channels[c0+1:]...)...)
	m.nbMeta--
	m.channels = m.channels[1:]
	c0--

	bitDepth := min(m.bitDepth, 24)
	paletteSize := pal.w

	if w == 0 {
		return
	}

	if t.numDeltas == 0 && t.predictorID == predZero {
		rows := make([][]int32, nb)

		for y := range h {
			prow := idx.row(y)

			for c := range nb {
				rows[c] = m.channels[c0+c].row(y)
			}

			for x := range w {
				i := prow[x]
				if nb == 1 {
					i = min(max(i, 0), int32(paletteSize)-1)
				}

				for c := range nb {
					rows[c][x] = paletteValue(pal, i, c, paletteSize, bitDepth)
				}
			}
		}

		return
	}

	indices := make([]int32, len(idx.data))
	copy(indices, idx.data)

	for c := range nb {
		out := m.channels[c0+c]

		var wp *wpState
		if t.predictorID == predWeighted {
			wp = &wpState{}
			wp.reset(wph, out.w)
		}

		for y := range out.h {
			orow := out.row(y)

			for x := range out.w {
				i := indices[y*w+x]
				entry := paletteValue(pal, i, c, paletteSize, bitDepth)

				d := predictionAt(out, x, y)

				var wpPred int64
				if wp != nil {
					wpPred, _ = wp.predictAndProperty(x, y, &d)
				}

				val := int64(entry)
				if i < int32(t.numDeltas) {
					val += predictOne(uint8(t.predictorID), d, wpPred)
				}

				orow[x] = int32(val)

				if wp != nil {
					wp.updateErrors(orow[x], x, y)
				}
			}
		}
	}
}
