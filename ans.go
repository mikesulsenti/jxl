package jxl

const (
	logSumProbs  = 12
	sumProbs     = 1 << logSumProbs
	rleMarkerSym = logSumProbs + 1
	ansChecksum  = 0x130000
)

type ansBucket struct {
	dist         uint16
	aliasOffset  uint16
	aliasDistXor uint16
	aliasSymbol  uint8
	aliasCutoff  uint8
}

type ansHistogram struct {
	buckets      []ansBucket
	logBucket    int
	bucketMask   uint32
	singleSymbol int32
}

func ansReadU8(r *bitReader) uint32 {
	if !r.bit() {
		return 0
	}

	n := int(r.bits(3))

	return uint32(1<<n) + uint32(r.bits(n))
}

var ansPrefixTable = [128][2]uint8{
	{10, 3}, {12, 7}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {0, 5}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {11, 6}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {0, 5}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {13, 7}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {0, 5}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {11, 6}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
	{10, 3}, {0, 5}, {7, 3}, {3, 4}, {6, 3}, {8, 3}, {9, 3}, {5, 4},
	{10, 3}, {4, 4}, {7, 3}, {1, 4}, {6, 3}, {8, 3}, {9, 3}, {2, 4},
}

func ansReadPrefix(r *bitReader) uint16 {
	e := ansPrefixTable[r.peek(7)]
	r.consume(int(e[1]))

	return uint16(e[0])
}

func ansDistTwoSymbols(r *bitReader, dist []uint16) int {
	v0, v1 := int(ansReadU8(r)), int(ansReadU8(r))

	if v0 == v1 || max(v0, v1)+1 > len(dist) {
		r.setErr(ErrInvalid)

		return 0
	}

	prob := uint16(r.bits(logSumProbs))
	dist[v0] = prob
	dist[v1] = sumProbs - prob

	return max(v0, v1) + 1
}

func ansDistSingleSymbol(r *bitReader, dist []uint16) int {
	v := int(ansReadU8(r))
	if v+1 > len(dist) {
		r.setErr(ErrInvalid)

		return 0
	}

	dist[v] = sumProbs

	return v + 1
}

func ansDistEven(r *bitReader, dist []uint16) int {
	n := int(ansReadU8(r)) + 1
	if n > len(dist) {
		r.setErr(ErrInvalid)

		return 0
	}

	base, rem := sumProbs/n, sumProbs%n

	for i := range rem {
		dist[i] = uint16(base) + 1
	}
	for i := rem; i < n; i++ {
		dist[i] = uint16(base)
	}

	return n
}

type repeatRange struct{ start, end int }

func ansDistComplex(r *bitReader, dist []uint16) int {
	l := 0
	for l < 3 && r.bit() {
		l++
	}

	shift := int16(r.bits(l)) + int16(1<<l) - 1
	if shift > 13 {
		r.setErr(ErrInvalid)

		return 0
	}

	n := int(ansReadU8(r)) + 3
	if n > len(dist) {
		r.setErr(ErrInvalid)

		return 0
	}

	var (
		repeats []repeatRange
		omitLog uint16
		omitPos = -1
	)

	for i := 0; i < n; {
		dist[i] = ansReadPrefix(r)

		if dist[i] == rleMarkerSym {
			count := int(ansReadU8(r)) + 4
			if i+count > n {
				r.setErr(ErrInvalid)

				return 0
			}

			repeats = append(repeats, repeatRange{i, i + count})
			i += count

			continue
		}

		if omitPos < 0 || dist[i] > omitLog {
			omitLog, omitPos = dist[i], i
		}

		i++
	}

	if omitPos < 0 || (omitPos+1 < len(dist) && dist[omitPos+1] == rleMarkerSym) {
		r.setErr(ErrInvalid)

		return 0
	}

	var (
		ri       int
		acc      uint16
		prevDist uint16
	)

	for i := range dist {
		if ri < len(repeats) && repeats[ri].start <= i {
			if repeats[ri].end == i {
				ri++
			} else {
				dist[i] = prevDist
				acc += dist[i]

				if acc >= sumProbs {
					r.setErr(ErrInvalid)

					return 0
				}

				continue
			}
		}

		if dist[i] == 0 || i == omitPos {
			prevDist = 0

			continue
		}

		if dist[i] > 1 {
			zeros := int16(dist[i]) - 1
			bitcount := min(max(shift-((logSumProbs-zeros)>>1), 0), zeros)
			dist[i] = uint16(1<<zeros) + uint16(r.bits(int(bitcount)))<<(zeros-bitcount)
		}

		prevDist = dist[i]
		acc += dist[i]

		if acc >= sumProbs {
			r.setErr(ErrInvalid)

			return 0
		}
	}

	dist[omitPos] = sumProbs - acc

	return n
}

type ansWorking struct {
	dist        uint16
	aliasSymbol uint16
	aliasOffset uint16
	aliasCutoff uint16
}

type ansScratch struct {
	dist      []uint16
	work      []ansWorking
	underfull []int
	overfull  []int
	buckets   []ansBucket
}

func (s *ansScratch) init(num, tableSize int) {
	s.dist = make([]uint16, tableSize)
	s.work = make([]ansWorking, tableSize)
	s.underfull = make([]int, 0, tableSize)
	s.overfull = make([]int, 0, tableSize)
	s.buckets = make([]ansBucket, num*tableSize)
}

func (s *ansScratch) take(n int) []ansBucket {
	out := s.buckets[:n:n]
	s.buckets = s.buckets[n:]

	return out
}

func buildAliasMap(alphabetSize, logBucket int, dist []uint16, s *ansScratch) []ansBucket {
	bucketSize := uint16(1) << logBucket
	w := s.work[:len(dist)]

	for i, d := range dist {
		w[i] = ansWorking{dist: d, aliasCutoff: d}

		if i < alphabetSize {
			w[i].aliasSymbol = uint16(i)
		}
	}

	underfull := s.underfull[:0]
	overfull := s.overfull[:0]

	for i := range w {
		switch {
		case w[i].dist < bucketSize:
			underfull = append(underfull, i)
		case w[i].dist > bucketSize:
			overfull = append(overfull, i)
		}
	}

	for len(overfull) > 0 && len(underfull) > 0 {
		o := overfull[len(overfull)-1]
		overfull = overfull[:len(overfull)-1]
		u := underfull[len(underfull)-1]
		underfull = underfull[:len(underfull)-1]

		by := bucketSize - w[u].aliasCutoff
		w[o].aliasCutoff -= by
		w[u].aliasSymbol = uint16(o)
		w[u].aliasOffset = w[o].aliasCutoff

		switch {
		case w[o].aliasCutoff < bucketSize:
			underfull = append(underfull, o)
		case w[o].aliasCutoff > bucketSize:
			overfull = append(overfull, o)
		}
	}

	out := s.take(len(w))

	for i, b := range w {
		if b.aliasCutoff == bucketSize {
			out[i] = ansBucket{dist: b.dist, aliasSymbol: uint8(i)}

			continue
		}

		out[i] = ansBucket{
			dist:         b.dist,
			aliasSymbol:  uint8(b.aliasSymbol),
			aliasOffset:  b.aliasOffset - b.aliasCutoff,
			aliasCutoff:  uint8(b.aliasCutoff),
			aliasDistXor: b.dist ^ w[b.aliasSymbol].dist,
		}
	}

	return out
}

func (h *ansHistogram) read(r *bitReader, logAlphaSize int, s *ansScratch) {
	tableSize := 1 << logAlphaSize

	dist := s.dist[:tableSize]

	clear(dist)

	var alphabetSize int

	switch {
	case r.bit():
		if r.bit() {
			alphabetSize = ansDistTwoSymbols(r, dist)
		} else {
			alphabetSize = ansDistSingleSymbol(r, dist)
		}
	case r.bit():
		alphabetSize = ansDistEven(r, dist)
	default:
		alphabetSize = ansDistComplex(r, dist)
	}

	if r.err() != nil {
		return
	}

	h.build(dist, alphabetSize, logAlphaSize, s)
}

func (h *ansHistogram) build(dist []uint16, alphabetSize, logAlphaSize int, s *ansScratch) {
	h.logBucket = logSumProbs - logAlphaSize
	bucketSize := uint16(1) << h.logBucket
	h.bucketMask = uint32(bucketSize) - 1
	h.singleSymbol = -1

	for i, d := range dist {
		if d == sumProbs {
			h.singleSymbol = int32(i)

			break
		}
	}

	if h.singleSymbol >= 0 {
		h.buckets = s.take(len(dist))

		for i, d := range dist {
			h.buckets[i] = ansBucket{
				dist:         d,
				aliasSymbol:  uint8(h.singleSymbol),
				aliasOffset:  bucketSize * uint16(i),
				aliasDistXor: d ^ sumProbs,
			}
		}

		return
	}

	h.buckets = buildAliasMap(alphabetSize, h.logBucket, dist, s)
}

func (h *ansHistogram) lookup(idx uint32) (symbol, dist, offset uint32) {
	i := idx >> h.logBucket
	pos := idx & h.bucketMask

	b := h.buckets[i]
	alias := uint32(0)

	if pos >= uint32(b.aliasCutoff) {
		alias = 1
	}

	dist = uint32(b.dist) ^ uint32(b.aliasDistXor)*alias
	symbol = uint32(b.aliasSymbol)*alias | i*(1-alias)
	offset = uint32(b.aliasOffset)*alias + pos

	return symbol, dist, offset
}

func (h *ansHistogram) decode(r *bitReader, state *uint32) uint32 {
	symbol, dist, offset := h.lookup(*state & 0xfff)

	next := (*state >> logSumProbs) * dist
	next += offset

	if next < 1<<16 {
		next = next<<16 | uint32(r.peek(16))
		r.consumeOpt(16)
	}

	*state = next

	return symbol
}

type ansCodes struct {
	histograms []ansHistogram
}

func (a *ansCodes) read(r *bitReader, num, logAlphaSize int) {
	a.histograms = make([]ansHistogram, num)

	var s ansScratch

	s.init(num, 1<<logAlphaSize)

	for i := range a.histograms {
		a.histograms[i].read(r, logAlphaSize, &s)

		if r.err() != nil {
			return
		}
	}
}

type ansReader struct {
	state uint32
}

func (a *ansReader) init(r *bitReader) { a.state = uint32(r.bits(32)) }

func (a *ansReader) decode(codes *ansCodes, r *bitReader, ctx int) uint32 {
	return codes.histograms[ctx].decode(r, &a.state)
}

func (a *ansReader) checkFinalState() error {
	if a.state != ansChecksum {
		return ErrInvalid
	}

	return nil
}
