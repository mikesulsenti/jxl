package jxl

import "math/bits"

const (
	ansMinReps      = 5
	ansMaxLogAlpha  = 8
	ansMinLogAlpha  = 5
	ansStateMin     = 1 << 16
	ansRenormShift  = 32 - logSumProbs
	ansBitWidthCode = logSumProbs + 2
)

var (
	bitWidthLengths = [ansBitWidthCode]uint8{5, 4, 4, 4, 4, 4, 3, 3, 3, 3, 3, 6, 7, 7}
	bitWidthSymbols = [ansBitWidthCode]uint8{17, 11, 15, 3, 9, 7, 4, 2, 5, 6, 0, 33, 1, 65}
)

func ansWriteU8(w *bitWriter, n uint32) {
	if n == 0 {
		w.bit(false)

		return
	}

	w.bit(true)

	log := bits.Len32(n) - 1
	w.bits(3, uint64(log))
	w.bits(log, uint64(n)-1<<log)
}

func popCountPrecision(zeros, shift int) int {
	return min(max(shift-((logSumProbs-zeros)>>1), 0), zeros)
}

func ansGridStep(count, shift int) int {
	zeros := bits.Len32(uint32(count)) - 1

	return zeros - popCountPrecision(zeros, shift)
}

func ansRoundToGrid(count, shift int) int {
	if count <= 1 {
		return 1
	}

	drop := ansGridStep(count, shift)
	if drop == 0 {
		return count
	}

	zeros := bits.Len32(uint32(count)) - 1
	base, step := 1<<zeros, 1<<drop

	return base + (count-base+step/2)/step*step
}

func ansLowerOnGrid(count, shift int) int {
	if count <= 1 {
		return 1
	}

	zeros := bits.Len32(uint32(count)) - 1
	base, step := 1<<zeros, 1<<ansGridStep(count, shift)

	if count-step >= base {
		return count - step
	}

	if count > base {
		return base
	}

	if zeros == 0 {
		return 1
	}

	return base - 1<<(zeros-1-popCountPrecision(zeros-1, shift))
}

func normalizeCounts(counts []uint32, shift int) ([]uint16, int, bool) {
	alphabet := 0
	total := uint64(0)

	for i, c := range counts {
		if c != 0 {
			alphabet = i + 1
			total += uint64(c)
		}
	}

	if alphabet == 0 || total == 0 {
		return []uint16{sumProbs}, 0, true
	}

	dist := make([]uint16, alphabet)

	omit, best := 0, uint32(0)

	for i, c := range counts[:alphabet] {
		if c > best {
			omit, best = i, c
		}
	}

	sum := 0

	for i, c := range counts[:alphabet] {
		if c == 0 || i == omit {
			continue
		}

		v := int((uint64(c)*sumProbs + total/2) / total)
		v = ansRoundToGrid(max(v, 1), shift)
		dist[i] = uint16(v)
		sum += v
	}

	for sum >= sumProbs {
		largest := -1

		for i, d := range dist {
			if i == omit || d <= 1 {
				continue
			}

			if largest < 0 || d > dist[largest] {
				largest = i
			}
		}

		if largest < 0 {
			return nil, 0, false
		}

		v := ansLowerOnGrid(int(dist[largest]), shift)
		if v >= int(dist[largest]) {
			return nil, 0, false
		}

		sum -= int(dist[largest]) - v
		dist[largest] = uint16(v)
	}

	for i := range omit {
		for dist[i] >= sumProbs/2 {
			v := ansLowerOnGrid(int(dist[i]), shift)
			if v >= int(dist[i]) {
				return nil, 0, false
			}

			sum -= int(dist[i]) - v
			dist[i] = uint16(v)
		}
	}

	dist[omit] = uint16(sumProbs - sum)

	return dist, omit, true
}

func writeANSHistogram(w *bitWriter, dist []uint16, omit, shift int) {
	used := 0
	single := -1

	for i, d := range dist {
		if d != 0 {
			used++
			single = i
		}
	}

	if used == 1 {
		w.bits(2, 1)
		ansWriteU8(w, uint32(single))

		return
	}

	if len(dist) < 3 {
		first := -1

		for i, d := range dist {
			if d != 0 && first < 0 {
				first = i
			}
		}

		w.bits(2, 3)
		ansWriteU8(w, uint32(first))
		ansWriteU8(w, uint32(single))
		w.bits(logSumProbs, uint64(dist[first]))

		return
	}

	w.bits(2, 0)

	method := shift + 1
	log := bits.Len32(uint32(method)) - 1

	w.bits(log, 1<<log-1)

	if log != 3 {
		w.bit(false)
	}

	w.bits(log, uint64(method)&(1<<log-1))

	ansWriteU8(w, uint32(len(dist)-3))

	same := make([]int, len(dist))
	last := 0

	for i := 1; i <= len(dist); i++ {
		if i == len(dist) || i == omit || i == omit+1 || dist[i] != dist[last] {
			same[last] = i - last
			last = i
		}
	}

	width := make([]uint8, len(dist))
	omitWidth := 10

	for i, d := range dist {
		if i == omit || d == 0 {
			continue
		}

		width[i] = uint8(bits.Len32(uint32(d)))

		extra := 0
		if i < omit {
			extra = 1
		}

		omitWidth = max(omitWidth, int(width[i])+extra)
	}

	width[omit] = uint8(omitWidth)

	for i := 0; i < len(dist); i++ {
		w.bits(int(bitWidthLengths[width[i]]), uint64(bitWidthSymbols[width[i]]))

		if same[i] >= ansMinReps {
			w.bits(int(bitWidthLengths[rleMarkerSym]), uint64(bitWidthSymbols[rleMarkerSym]))
			ansWriteU8(w, uint32(same[i]-ansMinReps))

			i += same[i] - 1
		}
	}

	if shift == 0 {
		return
	}

	for i := 0; i < len(dist); i++ {
		if width[i] > 1 && i != omit {
			zeros := int(width[i]) - 1
			bitcount := popCountPrecision(zeros, shift)
			drop := zeros - bitcount

			w.bits(bitcount, uint64(dist[i]>>drop)-1<<bitcount)
		}

		if same[i] >= ansMinReps {
			i += same[i] - 1
		}
	}
}

type ansSymbolInfo struct {
	freq  uint16
	start uint16
}

type ansTable struct {
	dist   []uint16
	omit   int
	shift  int
	info   []ansSymbolInfo
	slots  []uint16
	logAlp int
}

func buildANSTable(counts []uint32, logAlphaSize int) (*ansTable, bool) {
	best := (*ansTable)(nil)
	bestBits := 0

	for shift := range logSumProbs + 1 {
		dist, omit, ok := normalizeCounts(counts, shift)
		if !ok {
			return nil, false
		}

		var probe bitWriter

		writeANSHistogram(&probe, dist, omit, shift)

		if probe.err() != nil {
			continue
		}

		bits := probe.pos() + ansCodedBits(counts, dist)

		if best == nil || bits < bestBits {
			best, bestBits = &ansTable{dist: dist, omit: omit, shift: shift}, bits
		}
	}

	if best == nil {
		return nil, false
	}

	best.logAlp = logAlphaSize
	best.fill()

	return best, true
}

func ansCodedBits(counts []uint32, dist []uint16) int {
	total := 0.0

	for i, c := range counts {
		if c == 0 || i >= len(dist) || dist[i] == 0 {
			continue
		}

		total += float64(c) * (logSumProbs - log2Int(uint64(dist[i])))
	}

	return int(total) + 1
}

func (t *ansTable) fill() {
	size := 1 << t.logAlp

	dist := make([]uint16, size)
	copy(dist, t.dist)

	var s ansScratch

	s.init(1, size)

	var h ansHistogram

	h.build(dist, len(t.dist), t.logAlp, &s)

	t.info = make([]ansSymbolInfo, size)
	t.slots = make([]uint16, sumProbs)

	start := uint16(0)

	for i, d := range dist {
		t.info[i] = ansSymbolInfo{freq: d, start: start}
		start += d
	}

	for idx := range uint32(sumProbs) {
		symbol, _, offset := h.lookup(idx)
		t.slots[uint32(t.info[symbol].start)+offset] = uint16(idx)
	}
}

func (t *ansTable) put(state uint32, symbol uint32) (uint32, uint32, int) {
	info := t.info[symbol]
	freq := uint32(info.freq)

	word, nbits := uint32(0), 0

	if state>>ansRenormShift >= freq {
		word, nbits = state&0xffff, 16
		state >>= 16
	}

	slot := uint32(t.slots[uint32(info.start)+state%freq])

	return state/freq<<logSumProbs + slot, word, nbits
}

func (h *ansHistogram) frequencies(size int) []uint16 {
	out := make([]uint16, size)

	for idx := range uint32(sumProbs) {
		s, _, _ := h.lookup(idx)
		out[s]++
	}

	return out
}
