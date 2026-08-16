package jxl

import (
	"math"
	"math/bits"
)

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

func (t *modTransform) write(w *bitWriter) {
	if t.id != xformRCT {
		w.setErr(ErrUnsupported)

		return
	}

	w.bits(2, uint64(t.id))
	w.u32(t.beginC, dBits(3), dBitsOff(6, 8), dBitsOff(10, 72), dBitsOff(13, 1096))
	w.u32(t.rctType, dVal(6), dBits(2), dBitsOff(4, 2), dBitsOff(6, 10))
}

func (g *groupHeader) write(w *bitWriter) {
	w.bit(g.useGlobalTree)
	g.wp.write(w)
	w.u32(uint32(len(g.transforms)), dVal(0), dVal(1), dBitsOff(4, 2), dBitsOff(8, 18))

	for i := range g.transforms {
		g.transforms[i].write(w)
	}
}

const rctYCoCg = 6

func (m *modImage) forwardRCT(begin int) bool {
	if begin+3 > len(m.channels) {
		return false
	}

	c0, c1, c2 := m.channels[begin], m.channels[begin+1], m.channels[begin+2]

	if !c0.equivalent(c1) || !c0.equivalent(c2) {
		return false
	}

	for y := range c0.h {
		r, g, b := c0.row(y), c1.row(y), c2.row(y)

		for x := range c0.w {
			co := r[x] - b[x]
			tmp := b[x] + co>>1
			cg := g[x] - tmp

			r[x], g[x], b[x] = tmp+cg>>1, co, cg
		}
	}

	return true
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

type treeSpec struct {
	property    int32
	val         int32
	predictor   uint8
	offset      int32
	mul         uint32
	left, right *treeSpec
}

func serializeTree(root *treeSpec) []treeNode {
	var nodes []treeNode

	queue := []*treeSpec{root}
	leafID := uint32(0)

	for len(queue) > 0 {
		s := queue[0]
		queue = queue[1:]

		if s.property < 0 {
			nodes = append(nodes, treeNode{
				property: -1 - int32(s.predictor), val: s.offset, child: s.mul, id: leafID,
			})
			leafID++

			continue
		}

		nodes = append(nodes, treeNode{
			property: s.property, val: s.val, child: uint32(len(nodes) + len(queue) + 1),
		})

		queue = append(queue, s.left, s.right)
	}

	return nodes
}

func predictorLeaf(p uint8) *treeSpec {
	return &treeSpec{property: -1, predictor: p, mul: 1}
}

const gradientProperty = 14

var gradientThresholds = [2]int32{-2, 1}

func activityLeaf(p uint8) *treeSpec {
	spec := predictorLeaf(p)

	for i := len(gradientThresholds) - 1; i >= 0; i-- {
		spec = &treeSpec{
			property: gradientProperty, val: gradientThresholds[i],
			left: spec, right: predictorLeaf(p),
		}
	}

	return spec
}

const (
	gradientValueProperty = 9
	fixedTreeMinPixels    = 14
)

var fixedTreeCutoffs = [...]int32{
	-500, -392, -255, -191, -127, -95, -63, -47, -31, -23, -15, -11, -7, -4, -3, -1, 0,
	1, 3, 5, 7, 11, 15, 23, 31, 47, 63, 95, 127, 191, 255, 392, 500,
}

func fixedTreeNode(property, mul int32, minGap, begin, end int, p uint8) *treeSpec {
	if begin+minGap >= end {
		return predictorLeaf(p)
	}

	split := (begin + end) / 2

	return &treeSpec{
		property: property,
		val:      fixedTreeCutoffs[split] * mul,
		left:     fixedTreeNode(property, mul, minGap, split+1, end, p),
		right:    fixedTreeNode(property, mul, minGap, begin, split, p),
	}
}

// fixedTreeLeaf mirrors libjxl's static context tree: a balanced search over
// the residual predictor's own error, shallower on small images.
func fixedTreeLeaf(p uint8, pixels int, depth uint32) *treeSpec {
	minGap := 0

	if log := ceilLog2(uint32(max(pixels, 1))); log < fixedTreeMinPixels {
		minGap = 8 * (fixedTreeMinPixels - int(log))
	}

	mul := int32(1)

	if depth > 11 {
		mul = 1 << min(4, depth-11)
	}

	property := int32(gradientValueProperty)
	if p == predWeighted {
		property = wpProperty
	}

	return fixedTreeNode(property, mul, minGap, 0, len(fixedTreeCutoffs), p)
}

func fixedTree(preds []uint8, pixels int, depth uint32) *maTree {
	spec := fixedTreeLeaf(preds[len(preds)-1], pixels, depth)

	for c := len(preds) - 2; c >= 0; c-- {
		spec = &treeSpec{
			property: 0, val: int32(c),
			left:  spec,
			right: fixedTreeLeaf(preds[c], pixels, depth),
		}
	}

	t := &maTree{nodes: serializeTree(spec)}
	t.finish()

	return t
}

func predictorTree(preds []uint8) *maTree {
	spec := activityLeaf(preds[len(preds)-1])

	for c := len(preds) - 2; c >= 0; c-- {
		spec = &treeSpec{property: 0, val: int32(c), left: spec, right: activityLeaf(preds[c])}
	}

	t := &maTree{nodes: serializeTree(spec)}
	t.finish()

	return t
}

func channelTree(channels int, p uint8) *maTree {
	preds := make([]uint8, channels)

	for i := range preds {
		preds[i] = p
	}

	return predictorTree(preds)
}

var (
	fastPredictors = [...]uint8{predGradient, predWeighted, predWest, predNorth, predSelect}
	allPredictors  = [...]uint8{
		predGradient, predWeighted, predWest, predNorth, predSelect, predAvgWestNorth,
		predNorthEast, predNorthWest, predWestWest, predAvgWestNorthWest,
		predAvgNorthNorthWest, predAvgNorthNorthEast, predAvgAll, predZero,
	}
)

func predictorCandidates(effort int) []uint8 {
	switch {
	case effort < predictorSearchEffort:
		return fastPredictors[:1]
	case effort < MaxEffort:
		return fastPredictors[:]
	default:
		return allPredictors[:]
	}
}

// residualCost estimates the bits a channel takes, bucketing residuals the way
// the entropy coder does and adding the bits below each bucket.
func residualCost(counts *[32]uint32, total uint32) float64 {
	if total == 0 {
		return 0
	}

	bits := 0.0

	for b, n := range counts {
		if n == 0 {
			continue
		}

		bits += float64(n) * (log2Int(uint64(total)) - log2Int(uint64(n)))

		if b > 1 {
			bits += float64(n) * float64(b-1)
		}
	}

	return bits
}

func (m *modImage) choosePredictors(wph *weightedHeader, streamID int, effort int) []uint8 {
	out := make([]uint8, len(m.channels))
	candidates := predictorCandidates(effort)

	var scratch modScratch

	for i, c := range m.channels {
		out[i] = candidates[0]

		if c.w == 0 || c.h == 0 {
			continue
		}

		best := math.Inf(1)

		for _, p := range candidates {
			tree := &maTree{nodes: serializeTree(predictorLeaf(p))}
			tree.finish()

			var (
				counts [32]uint32
				total  uint32
			)

			ok := m.walkChannel(tree, wph, i, streamID, &scratch, func(_ []int32, _ uint32, residual int32) bool {
				counts[bits.Len32(packSigned(residual))]++
				total++

				return true
			})

			if !ok {
				continue
			}

			if cost := residualCost(&counts, total); cost < best {
				best, out[i] = cost, p
			}
		}
	}

	return out
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

func (t *maTree) write(w *bitWriter, leafValues []valueCounts) *symbolWriter {
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

	sw := th.write(w, counts)

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

	sw.flush(w)

	leaves, clustered := clusterContexts(leafValues, 256)

	ch, counts := buildHistograms(leaves, clustered)
	t.histograms = *ch

	return t.histograms.write(w, counts)
}

func (m *modImage) walkChannel(tree *maTree, wph *weightedHeader, chan_, streamID int,
	scratch *modScratch, fn func(props []int32, id uint32, residual int32) bool,
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

			if !fn(props, leaf.id, int32(q)) {
				return false
			}

			if tree.usesWP {
				wp.updateErrors(row[x], x, y)
			}
		}
	}

	return true
}

func (m *modImage) walkStream(tree *maTree, wph *weightedHeader, streamID, upto int,
	fn func(props []int32, id uint32, residual int32) bool,
) bool {
	var scratch modScratch

	for i, c := range m.channels[:upto] {
		if c.w == 0 || c.h == 0 {
			continue
		}

		if !m.walkChannel(tree, wph, i, streamID, &scratch, fn) {
			return false
		}
	}

	return true
}

func (m *modImage) countResiduals(hdr *groupHeader, tree *maTree, streamID, upto int,
	values []valueCounts,
) bool {
	return m.walkStream(tree, &hdr.wp, streamID, upto, func(_ []int32, id uint32, residual int32) bool {
		values[id].add(packSigned(residual))

		return true
	})
}

func (m *modImage) encodeSymbols(w *bitWriter, hdr *groupHeader, tree *maTree, streamID, upto int,
	codes, sw *symbolWriter,
) {
	sw.reset(codes)

	m.walkStream(tree, &hdr.wp, streamID, upto, func(_ []int32, id uint32, residual int32) bool {
		sw.writeSigned(&tree.histograms, w, int(id), residual)

		return w.err() == nil
	})

	sw.flush(w)
}

func (m *modImage) encodeStream(w *bitWriter, hdr *groupHeader, tree *maTree, streamID int) {
	hdr.write(w)

	if w.err() != nil {
		return
	}

	m.encodeCoded(w, hdr, tree, streamID, len(m.channels))
}

func (m *modImage) encodeCoded(w *bitWriter, hdr *groupHeader, tree *maTree, streamID, upto int) {
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

	tree.finish()

	values := make([]valueCounts, (len(tree.nodes)+1)/2)

	var collect modScratch

	for i, c := range m.channels[:upto] {
		if c.w == 0 || c.h == 0 {
			continue
		}

		ok := m.walkChannel(tree, &hdr.wp, i, streamID, &collect, func(_ []int32, id uint32, residual int32) bool {
			values[id].add(packSigned(residual))

			return true
		})

		if !ok {
			w.setErr(ErrInvalid)

			return
		}
	}

	sw := tree.write(w, values)

	if w.err() != nil {
		return
	}

	var emit modScratch

	for i, c := range m.channels[:upto] {
		if c.w == 0 || c.h == 0 {
			continue
		}

		m.walkChannel(tree, &hdr.wp, i, streamID, &emit, func(_ []int32, id uint32, residual int32) bool {
			sw.writeSigned(&tree.histograms, w, int(id), residual)

			return w.err() == nil
		})
	}

	sw.flush(w)
}
