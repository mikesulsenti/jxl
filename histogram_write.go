package jxl

import (
	"cmp"
	"maps"
	"math"
	"slices"
)

func (l *lz77Params) write(w *bitWriter) {
	w.bit(l.enabled)

	if !l.enabled {
		return
	}

	w.u32(l.minSymbol, dVal(224), dVal(512), dVal(4096), dBitsOff(15, 8))
	w.u32(l.minLength, dVal(3), dVal(4), dBitsOff(2, 5), dBitsOff(8, 9))
}

func moveToFrontEncode(v []uint8) {
	var mtf [256]uint8

	for i := range mtf {
		mtf[i] = uint8(i)
	}

	for i, val := range v {
		index := 0

		for mtf[index] != val {
			index++
		}

		v[i] = uint8(index)

		if index != 0 {
			moveToFront(mtf[:], uint8(index))
		}
	}
}

func writeSimpleContextMap(w *bitWriter, m []uint8, maxVal uint8) {
	bitsPerEntry := 0

	for int(maxVal) >= 1<<bitsPerEntry {
		bitsPerEntry++
	}

	if bitsPerEntry > 3 {
		w.setErr(ErrUnsupported)

		return
	}

	w.bit(true)
	w.bits(2, uint64(bitsPerEntry))

	if bitsPerEntry == 0 {
		return
	}

	for _, v := range m {
		w.bits(bitsPerEntry, uint64(v))
	}
}

func writeCodedContextMap(w *bitWriter, m []uint8, useMTF bool) {
	entries := make([]uint8, len(m))
	copy(entries, m)

	if useMTF {
		moveToFrontEncode(entries)
	}

	values := make([]uint32, len(entries))
	counted := make([]valueCounts, 1)

	for i, v := range entries {
		values[i] = uint32(v)
		counted[0].add(uint32(v))
	}

	h, counts := buildHistograms(singleContext[:], counted)

	w.bit(false)
	w.bit(useMTF)

	sw := symbolWriter{codes: h.write(w, counts)}

	if w.err() != nil {
		return
	}

	for _, v := range values {
		sw.writeUnsigned(h, w, 0, v)
	}
}

func writeContextMap(w *bitWriter, m []uint8) {
	var (
		seen     [256]bool
		distinct int
		maxVal   uint8
	)

	for _, v := range m {
		if !seen[v] {
			seen[v] = true
			distinct++
		}

		maxVal = max(maxVal, v)
	}

	if distinct != int(maxVal)+1 {
		w.setErr(ErrInvalid)

		return
	}

	candidates := make([]*bitWriter, 3)

	for i := range candidates {
		candidates[i] = &bitWriter{}
	}

	writeSimpleContextMap(candidates[0], m, maxVal)
	writeCodedContextMap(candidates[1], m, false)
	writeCodedContextMap(candidates[2], m, true)

	best := -1

	for i, c := range candidates {
		if c.err() != nil {
			continue
		}

		if best < 0 || c.pos() < candidates[best].pos() {
			best = i
		}
	}

	if best < 0 {
		w.setErr(candidates[1].err())

		return
	}

	w.append(candidates[best])
}

func (h *histograms) write(w *bitWriter, counts [][]uint32) [][]prefixCode {
	if h.lz77.enabled || !h.usePrefix || h.logAlphaSize != huffmanMaxBits {
		w.setErr(ErrUnsupported)

		return nil
	}

	if len(counts) > 256 {
		w.setErr(ErrUnsupported)

		return nil
	}

	if len(counts) != len(h.uintConfigs) || len(h.contextMap) == 0 {
		w.setErr(ErrInvalid)

		return nil
	}

	numHistograms := 0

	for _, c := range h.contextMap {
		numHistograms = max(numHistograms, int(c)+1)
	}

	if numHistograms != len(counts) {
		w.setErr(ErrInvalid)

		return nil
	}

	h.lz77.write(w)

	if len(h.contextMap) > 1 {
		writeContextMap(w, h.contextMap)
	}

	w.bit(true)

	for i := range h.uintConfigs {
		h.uintConfigs[i].write(w, h.logAlphaSize)
	}

	return h.huff.write(w, counts)
}

func clusterContexts(values []valueCounts, limit int) ([]uint8, []valueCounts) {
	type ranked struct {
		index, count int
	}

	var used []ranked

	for i := range values {
		if values[i].total > 0 {
			used = append(used, ranked{i, int(values[i].total)})
		}
	}

	slices.SortFunc(used, func(a, b ranked) int {
		return cmp.Or(cmp.Compare(b.count, a.count), cmp.Compare(a.index, b.index))
	})

	m := make([]uint8, len(values))

	clusters := min(len(used), limit)
	if clusters == 0 {
		return m, make([]valueCounts, 1)
	}

	merged := make([]valueCounts, 0, clusters)

	var hist []histEntry

	for _, c := range used {
		src := &values[c.index]
		hist = src.entries(hist[:0])

		if len(merged) < clusters {
			merged = append(merged, src.clone())
			m[c.index] = uint8(len(merged) - 1)

			continue
		}

		best, bestCost := 0, math.Inf(1)

		for i := range merged {
			if cost := mergeCost(&merged[i], hist, src.total); cost < bestCost {
				best, bestCost = i, cost
			}
		}

		merged[best].merge(src)
		m[c.index] = uint8(best)
	}

	return m, merged
}

type histEntry struct {
	value, count uint32
}

const denseValueCount = 16

type valueCounts struct {
	dense [denseValueCount]uint32
	rest  map[uint32]uint32
	total uint32
}

func (c *valueCounts) add(v uint32) {
	c.total++

	if v < denseValueCount {
		c.dense[v]++

		return
	}

	if c.rest == nil {
		c.rest = make(map[uint32]uint32, 8)
	}

	c.rest[v]++
}

func (c *valueCounts) count(v uint32) uint32 {
	if v < denseValueCount {
		return c.dense[v]
	}

	return c.rest[v]
}

func (c *valueCounts) merge(o *valueCounts) {
	c.total += o.total

	for v, n := range o.dense {
		c.dense[v] += n
	}

	if len(o.rest) == 0 {
		return
	}

	if c.rest == nil {
		c.rest = make(map[uint32]uint32, len(o.rest))
	}

	for v, n := range o.rest {
		c.rest[v] += n
	}
}

func (c *valueCounts) clone() valueCounts {
	out := valueCounts{dense: c.dense, total: c.total}

	if len(c.rest) != 0 {
		out.rest = maps.Clone(c.rest)
	}

	return out
}

func (c *valueCounts) entries(out []histEntry) []histEntry {
	for v, n := range c.dense {
		if n != 0 {
			out = append(out, histEntry{uint32(v), n})
		}
	}

	tail := len(out)

	for v, n := range c.rest {
		out = append(out, histEntry{v, n})
	}

	slices.SortFunc(out[tail:], func(a, b histEntry) int {
		return cmp.Compare(a.value, b.value)
	})

	return out
}

const log2TableSize = 1 << 12

var log2Table = func() (t [log2TableSize]float64) {
	for i := 1; i < len(t); i++ {
		t[i] = math.Log2(float64(i))
	}

	return t
}()

func log2Int(n uint64) float64 {
	if n < log2TableSize {
		return log2Table[n]
	}

	return math.Log2(float64(n))
}

func xLogX(n uint64) float64 {
	if n == 0 {
		return 0
	}

	return float64(n) * log2Int(n)
}

func mergeCost(a *valueCounts, b []histEntry, bt uint32) float64 {
	at := uint64(a.total)

	cost := xLogX(at+uint64(bt)) - xLogX(at) - xLogX(uint64(bt))

	for _, e := range b {
		if av := uint64(a.count(e.value)); av != 0 {
			cost -= xLogX(av+uint64(e.count)) - xLogX(av) - xLogX(uint64(e.count))
		}
	}

	return cost
}

func buildHistograms(contextMap []uint8, values []valueCounts) (*histograms, [][]uint32) {
	h := &histograms{
		contextMap:   contextMap,
		logAlphaSize: huffmanMaxBits,
		usePrefix:    true,
		uintConfigs:  make([]hybridUint, len(values)),
	}

	counts := make([][]uint32, len(values))

	var (
		scratch uintCostScratch
		hist    []histEntry
	)

	for i := range values {
		hist = values[i].entries(hist[:0])

		cfg := chooseUintConfig(hist, huffmanMaxBits, 1<<huffmanMaxBits-1, &scratch)
		h.uintConfigs[i] = cfg

		maxToken := uint32(0)

		for _, e := range hist {
			token, _, _ := cfg.encode(e.value)
			maxToken = max(maxToken, token)
		}

		c := make([]uint32, maxToken+1)

		for _, e := range hist {
			token, _, _ := cfg.encode(e.value)
			c[token] += e.count
		}

		counts[i] = c
	}

	return h, counts
}

type symbolWriter struct {
	codes [][]prefixCode
}

func (s *symbolWriter) writeUnsigned(h *histograms, w *bitWriter, context int, v uint32) {
	s.writeClustered(h, w, int(h.contextMap[context]), v)
}

func (s *symbolWriter) writeSigned(h *histograms, w *bitWriter, context int, v int32) {
	s.writeUnsigned(h, w, context, packSigned(v))
}

func (s *symbolWriter) writeClustered(h *histograms, w *bitWriter, cluster int, v uint32) {
	token, nbits, extra := h.uintConfigs[cluster].encode(v)

	if int(token) >= len(s.codes[cluster]) {
		w.setErr(ErrInvalid)

		return
	}

	w.symbol(s.codes[cluster][token])

	if nbits > 0 {
		w.bits(int(nbits), uint64(extra))
	}
}
