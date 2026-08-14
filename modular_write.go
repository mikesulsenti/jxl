package jxl

import "math/bits"

func (h *weightedHeader) write(w *bitWriter) {
	if *h == defaultWeightedHeader() {
		w.bit(true)

		return
	}

	w.bit(false)

	for _, v := range [7]uint32{h.p1c, h.p2c, h.p3ca, h.p3cb, h.p3cc, h.p3cd, h.p3ce} {
		w.bits(5, uint64(v))
	}

	for _, v := range [4]uint32{h.w0, h.w1, h.w2, h.w3} {
		w.bits(4, uint64(v))
	}
}

func (g *groupHeader) write(w *bitWriter) {
	if len(g.transforms) != 0 {
		w.setErr(ErrUnsupported)

		return
	}

	w.bit(g.useGlobalTree)
	g.wp.write(w)
	w.u32(0, dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(8, 18))
}

func splitMultiplier(m uint32) (mulLog, mulBits uint32, ok bool) {
	if m == 0 {
		return 0, 0, false
	}

	mulLog = uint32(bits.TrailingZeros32(m))
	if mulLog >= 31 {
		return 0, 0, false
	}

	return mulLog, m>>mulLog - 1, true
}

func (t *maTree) finish() {
	maxProperty := int32(0)
	t.usesWP = false

	for i := range t.nodes {
		n := &t.nodes[i]

		if n.isLeaf() {
			t.usesWP = t.usesWP || n.predictor() == predWeighted

			continue
		}

		maxProperty = max(maxProperty, n.property)
		t.usesWP = t.usesWP || n.property == wpProperty
	}

	t.numProperties = int(maxProperty) + 1
}

func (t *maTree) write(w *bitWriter, leafValues []valueCounts) [][]prefixCode {
	if len(t.nodes) == 0 || len(t.nodes)%2 == 0 {
		w.setErr(ErrInvalid)

		return nil
	}

	if len(leafValues) != (len(t.nodes)+1)/2 {
		w.setErr(ErrInvalid)

		return nil
	}

	values := make([]valueCounts, numTreeContexts)

	for i := range t.nodes {
		n := &t.nodes[i]

		if !n.isLeaf() {
			values[ctxProperty].add(uint32(n.property) + 1)
			values[ctxSplitVal].add(packSigned(n.val))

			continue
		}

		mulLog, mulBits, ok := splitMultiplier(n.child)
		if !ok {
			w.setErr(ErrInvalid)

			return nil
		}

		values[ctxProperty].add(0)
		values[ctxPredictor].add(uint32(n.predictor()))
		values[ctxOffset].add(packSigned(n.val))
		values[ctxMultiplierLog].add(mulLog)
		values[ctxMultiplierBits].add(mulBits)
	}

	contexts := make([]uint8, numTreeContexts)

	for i := range contexts {
		contexts[i] = uint8(i)
	}

	th, counts := buildHistograms(contexts, values)

	sw := symbolWriter{codes: th.write(w, counts)}

	if w.err() != nil {
		return nil
	}

	for i := range t.nodes {
		n := &t.nodes[i]

		if !n.isLeaf() {
			sw.writeUnsigned(th, w, ctxProperty, uint32(n.property)+1)
			sw.writeSigned(th, w, ctxSplitVal, n.val)

			continue
		}

		mulLog, mulBits, _ := splitMultiplier(n.child)

		sw.writeUnsigned(th, w, ctxProperty, 0)
		sw.writeUnsigned(th, w, ctxPredictor, uint32(n.predictor()))
		sw.writeSigned(th, w, ctxOffset, n.val)
		sw.writeUnsigned(th, w, ctxMultiplierLog, mulLog)
		sw.writeUnsigned(th, w, ctxMultiplierBits, mulBits)
	}

	leaves := make([]uint8, len(leafValues))

	for i := range leaves {
		leaves[i] = uint8(i)
	}

	ch, counts := buildHistograms(leaves, leafValues)
	t.histograms = *ch

	return t.histograms.write(w, counts)
}

func (m *modImage) walkChannel(tree *maTree, wph *weightedHeader, chan_, streamID int,
	scratch *modScratch, fn func(id uint32, residual int32) bool,
) bool {
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
			diff := int64(row[x]) - guess
			mul := int64(leaf.child)

			if mul <= 0 || diff%mul != 0 {
				return false
			}

			q := diff / mul
			if int64(int32(q)) != q {
				return false
			}

			if !fn(leaf.id, int32(q)) {
				return false
			}

			if tree.usesWP {
				wp.updateErrors(row[x], x, y)
			}
		}
	}

	return true
}

func (m *modImage) encodeStream(w *bitWriter, hdr *groupHeader, tree *maTree, streamID int) {
	hdr.write(w)

	if w.err() != nil {
		return
	}

	m.encodeCoded(w, hdr, tree, streamID)
}

func (m *modImage) encodeCoded(w *bitWriter, hdr *groupHeader, tree *maTree, streamID int) {
	empty := true

	for _, c := range m.channels {
		if c.w != 0 && c.h != 0 {
			empty = false

			break
		}
	}

	if empty {
		return
	}

	tree.finish()

	values := make([]valueCounts, (len(tree.nodes)+1)/2)

	var collect modScratch

	for i, c := range m.channels {
		if c.w == 0 || c.h == 0 {
			continue
		}

		ok := m.walkChannel(tree, &hdr.wp, i, streamID, &collect, func(id uint32, residual int32) bool {
			values[id].add(packSigned(residual))

			return true
		})

		if !ok {
			w.setErr(ErrInvalid)

			return
		}
	}

	sw := symbolWriter{codes: tree.write(w, values)}

	if w.err() != nil {
		return
	}

	var emit modScratch

	for i, c := range m.channels {
		if c.w == 0 || c.h == 0 {
			continue
		}

		m.walkChannel(tree, &hdr.wp, i, streamID, &emit, func(id uint32, residual int32) bool {
			sw.writeSigned(&tree.histograms, w, int(id), residual)

			return w.err() == nil
		})
	}
}
