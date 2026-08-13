package jxl

import (
	"cmp"
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

	for i, v := range entries {
		values[i] = uint32(v)
	}

	h, counts := buildHistograms(singleContext[:], [][]uint32{values})

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

func clusterContexts(values [][]uint32, limit int) ([]uint8, [][]uint32) {
	type ranked struct {
		index, count int
	}

	var used []ranked

	for i, v := range values {
		if len(v) > 0 {
			used = append(used, ranked{i, len(v)})
		}
	}

	slices.SortFunc(used, func(a, b ranked) int {
		return cmp.Or(cmp.Compare(b.count, a.count), cmp.Compare(a.index, b.index))
	})

	m := make([]uint8, len(values))

	clusters := min(len(used), limit)
	if clusters == 0 {
		return m, [][]uint32{nil}
	}

	merged := make([][]uint32, 0, clusters)
	buckets := make([]map[uint32]uint64, 0, clusters)
	totals := make([]uint64, 0, clusters)

	for _, c := range used {
		vals := values[c.index]
		hist := sparseHistogram(vals)

		if len(buckets) < clusters {
			counts := make(map[uint32]uint64, len(hist))

			for _, e := range hist {
				counts[e.value] = e.count
			}

			buckets = append(buckets, counts)
			totals = append(totals, uint64(len(vals)))
			merged = append(merged, append([]uint32(nil), vals...))
			m[c.index] = uint8(len(buckets) - 1)

			continue
		}

		best, bestCost := 0, math.Inf(1)

		for i, b := range buckets {
			if cost := mergeCost(b, totals[i], hist, uint64(len(vals))); cost < bestCost {
				best, bestCost = i, cost
			}
		}

		for _, e := range hist {
			buckets[best][e.value] += e.count
		}

		totals[best] += uint64(len(vals))
		merged[best] = append(merged[best], vals...)
		m[c.index] = uint8(best)
	}

	return m, merged
}

type histEntry struct {
	value uint32
	count uint64
}

func sparseHistogram(values []uint32) []histEntry {
	sorted := make([]uint32, len(values))
	copy(sorted, values)
	slices.Sort(sorted)

	var out []histEntry

	for i := 0; i < len(sorted); {
		j := i

		for j < len(sorted) && sorted[j] == sorted[i] {
			j++
		}

		out = append(out, histEntry{sorted[i], uint64(j - i)})
		i = j
	}

	return out
}

func xLogX(n uint64) float64 {
	if n == 0 {
		return 0
	}

	return float64(n) * math.Log2(float64(n))
}

func mergeCost(a map[uint32]uint64, at uint64, b []histEntry, bt uint64) float64 {
	cost := xLogX(at+bt) - xLogX(at) - xLogX(bt)

	for _, e := range b {
		if av, ok := a[e.value]; ok {
			cost -= xLogX(av+e.count) - xLogX(av) - xLogX(e.count)
		}
	}

	return cost
}

func buildHistograms(contextMap []uint8, values [][]uint32) (*histograms, [][]uint32) {
	h := &histograms{
		contextMap:   contextMap,
		logAlphaSize: huffmanMaxBits,
		usePrefix:    true,
		uintConfigs:  make([]hybridUint, len(values)),
	}

	counts := make([][]uint32, len(values))

	for i, vals := range values {
		distinct, weights := valueHistogram(vals)

		cfg := chooseUintConfig(distinct, weights, huffmanMaxBits, 1<<huffmanMaxBits-1)
		h.uintConfigs[i] = cfg

		maxToken := uint32(0)

		for _, v := range distinct {
			token, _, _ := cfg.encode(v)
			maxToken = max(maxToken, token)
		}

		c := make([]uint32, maxToken+1)

		for j, v := range distinct {
			token, _, _ := cfg.encode(v)
			c[token] += weights[j]
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
