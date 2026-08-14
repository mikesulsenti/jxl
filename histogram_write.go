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

	sw := h.write(w, counts)

	if w.err() != nil {
		return
	}

	for _, v := range values {
		sw.writeUnsigned(h, w, 0, v)
	}

	sw.flush(w)
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

func (h *histograms) write(w *bitWriter, counts [][]uint32) *symbolWriter {
	if h.lz77.enabled {
		w.setErr(ErrUnsupported)

		return nil
	}

	if h.usePrefix && h.logAlphaSize != huffmanMaxBits {
		w.setErr(ErrUnsupported)

		return nil
	}

	if !h.usePrefix && (h.logAlphaSize < ansMinLogAlpha || h.logAlphaSize > ansMaxLogAlpha) {
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

	w.bit(h.usePrefix)

	if !h.usePrefix {
		w.bits(2, uint64(h.logAlphaSize-ansMinLogAlpha))
	}

	for i := range h.uintConfigs {
		h.uintConfigs[i].write(w, h.logAlphaSize)
	}

	if h.usePrefix {
		return &symbolWriter{codes: h.huff.write(w, counts)}
	}

	tables := make([]*ansTable, len(counts))

	for i, cnt := range counts {
		t, ok := buildANSTable(cnt, h.logAlphaSize)
		if !ok {
			w.setErr(ErrInvalid)

			return nil
		}

		writeANSHistogram(w, t.dist, t.omit, t.shift)

		tables[i] = t
	}

	return &symbolWriter{tables: tables}
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

		best, bestCost := 0, math.Inf(1)

		for i := range merged {
			if cost := mergeCost(&merged[i], hist, src.total); cost < bestCost {
				best, bestCost = i, cost
			}
		}

		if len(merged) < clusters && bestCost > tableCost(len(hist)) {
			merged = append(merged, src.clone())
			m[c.index] = uint8(len(merged) - 1)

			continue
		}

		merged[best].merge(src)
		m[c.index] = uint8(best)
	}

	return m, merged
}

type histEntry struct {
	value, count uint32
}

const uintConfigBits = 15

func tableCost(distinct int) float64 {
	return tableBitsPerSymbol*float64(distinct) + uintConfigBits
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
	prefix, prefixCounts := codeHistograms(contextMap, values, huffmanMaxBits, true)
	ans, ansCounts := codeHistograms(contextMap, values, ansMaxLogAlpha, false)

	if ans == nil || prefixBits(prefixCounts) <= ansBits(ansCounts) {
		return prefix, prefixCounts
	}

	return ans, ansCounts
}

func prefixBits(counts [][]uint32) float64 {
	total := 0.0

	for _, cnt := range counts {
		n, used := uint64(0), 0

		for _, c := range cnt {
			if c != 0 {
				n += uint64(c)
				used++
			}
		}

		if n == 0 {
			continue
		}

		total += tableCost(used)

		for _, c := range cnt {
			if c != 0 {
				total += float64(c) * max(log2Int(n)-log2Int(uint64(c)), 1)
			}
		}
	}

	return total
}

func ansBits(counts [][]uint32) float64 {
	total := 0.0

	for _, cnt := range counts {
		n, used := uint64(0), 0

		for _, c := range cnt {
			if c != 0 {
				n += uint64(c)
				used++
			}
		}

		if n == 0 {
			continue
		}

		total += tableCost(used)

		for _, c := range cnt {
			if c != 0 {
				total += float64(c) * (log2Int(n) - log2Int(uint64(c)))
			}
		}
	}

	return total
}

func codeHistograms(contextMap []uint8, values []valueCounts, logAlphaSize int,
	usePrefix bool,
) (*histograms, [][]uint32) {
	h := &histograms{
		contextMap:   contextMap,
		logAlphaSize: logAlphaSize,
		usePrefix:    usePrefix,
		uintConfigs:  make([]hybridUint, len(values)),
	}

	counts := make([][]uint32, len(values))

	var (
		scratch uintCostScratch
		hist    []histEntry
	)

	for i := range values {
		hist = values[i].entries(hist[:0])

		cfg := chooseUintConfig(hist, logAlphaSize, 1<<logAlphaSize-1, &scratch)
		h.uintConfigs[i] = cfg

		maxToken := uint32(0)

		for _, e := range hist {
			token, _, _ := cfg.encode(e.value)

			if int(token) >= 1<<logAlphaSize {
				return nil, nil
			}

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

type ansToken struct {
	cluster uint16
	token   uint16
	nbits   uint8
	extra   uint32
}

type bitGroup struct {
	bits  uint64
	nbits int
}

type symbolWriter struct {
	codes  [][]prefixCode
	tables []*ansTable
	tokens []ansToken
	groups []bitGroup
}

func (s *symbolWriter) reset(from *symbolWriter) {
	s.codes, s.tables = from.codes, from.tables
	s.tokens, s.groups = s.tokens[:0], s.groups[:0]
}

func (s *symbolWriter) writeUnsigned(h *histograms, w *bitWriter, context int, v uint32) {
	s.writeClustered(h, w, int(h.contextMap[context]), v)
}

func (s *symbolWriter) writeSigned(h *histograms, w *bitWriter, context int, v int32) {
	s.writeUnsigned(h, w, context, packSigned(v))
}

func (s *symbolWriter) writeClustered(h *histograms, w *bitWriter, cluster int, v uint32) {
	token, nbits, extra := h.uintConfigs[cluster].encode(v)

	if s.tables != nil {
		if int(token) >= len(s.tables[cluster].info) ||
			s.tables[cluster].info[token].freq == 0 {
			w.setErr(ErrInvalid)

			return
		}

		s.tokens = append(s.tokens, ansToken{
			cluster: uint16(cluster), token: uint16(token),
			nbits: uint8(nbits), extra: extra,
		})

		return
	}

	if int(token) >= len(s.codes[cluster]) {
		w.setErr(ErrInvalid)

		return
	}

	w.symbol(s.codes[cluster][token])

	if nbits > 0 {
		w.bits(int(nbits), uint64(extra))
	}
}

func (s *symbolWriter) flush(w *bitWriter) {
	if s.tables == nil {
		return
	}

	groups, acc := s.groups[:0], bitGroup{}

	add := func(v uint64, n int) {
		if n == 0 {
			return
		}

		if acc.nbits+n > maxBitsPerRead {
			groups = append(groups, acc)
			acc = bitGroup{}
		}

		acc.bits = acc.bits<<n | v
		acc.nbits += n
	}

	state := uint32(ansChecksum)

	for i := len(s.tokens) - 1; i >= 0; i-- {
		t := s.tokens[i]

		add(uint64(t.extra), int(t.nbits))

		next, word, n := s.tables[t.cluster].put(state, uint32(t.token))
		state = next

		add(uint64(word), n)
	}

	w.bits(32, uint64(state))
	w.bits(acc.nbits, acc.bits)

	for i := len(groups) - 1; i >= 0; i-- {
		w.bits(groups[i].nbits, groups[i].bits)
	}

	s.tokens, s.groups = s.tokens[:0], groups[:0]
}
