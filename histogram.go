package jxl

type lz77Params struct {
	enabled   bool
	minSymbol uint32
	minLength uint32
}

func (l *lz77Params) read(r *bitReader) {
	*l = lz77Params{}

	l.enabled = r.bit()
	if !l.enabled {
		return
	}

	l.minSymbol = r.u32(dVal(224), dVal(512), dVal(4096), dBitsOff(15, 8))
	l.minLength = r.u32(dVal(3), dVal(4), dBitsOff(2, 5), dBitsOff(8, 9))
}

var singleContext = [1]uint8{0}

type histograms struct {
	lz77           lz77Params
	lz77LengthUint hybridUint
	contextMap     []uint8
	lzDistCluster  uint8
	logAlphaSize   int
	uintConfigs    []hybridUint
	usePrefix      bool
	huff           huffmanCodes
	ans            ansCodes
}

func moveToFront(v []uint8, index uint8) {
	value := v[index]
	for i := int(index); i >= 1; i-- {
		v[i] = v[i-1]
	}

	v[0] = value
}

func inverseMoveToFront(v []uint8) {
	var mtf [256]uint8
	for i := range mtf {
		mtf[i] = uint8(i)
	}

	for i, index := range v {
		v[i] = mtf[index]

		if index != 0 {
			moveToFront(mtf[:], index)
		}
	}
}

func decodeContextMap(r *bitReader, numContexts int) []uint8 {
	if r.bit() {
		bitsPerEntry := int(r.bits(2))

		m := make([]uint8, numContexts)
		if bitsPerEntry == 0 {
			return m
		}

		for i := range m {
			m[i] = uint8(r.bits(bitsPerEntry))
		}

		return verifyContextMap(r, m)
	}

	useMTF := r.bit()

	var h histograms
	h.read(r, 1, numContexts > 2)

	if r.err() != nil {
		return nil
	}

	var sr symbolReader

	sr.init(&h, r, 0)

	m := make([]uint8, numContexts)
	for i := range m {
		v := sr.readUnsigned(&h, r, 0)

		if v > 0xff {
			r.setErr(ErrInvalid)

			return nil
		}

		m[i] = uint8(v)
	}

	if err := sr.checkFinalState(&h, r); err != nil {
		r.setErr(err)

		return nil
	}

	if useMTF {
		inverseMoveToFront(m)
	}

	return verifyContextMap(r, m)
}

func verifyContextMap(r *bitReader, m []uint8) []uint8 {
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
		r.setErr(ErrInvalid)

		return nil
	}

	return m
}

func (h *histograms) read(r *bitReader, numContexts int, allowLZ77 bool) {
	*h = histograms{}

	h.lz77.read(r)

	if h.lz77.enabled {
		if !allowLZ77 {
			r.setErr(ErrInvalid)

			return
		}

		h.lz77LengthUint.read(r, 8)
		numContexts++
	}

	if numContexts > 1 {
		h.contextMap = decodeContextMap(r, numContexts)
	} else {
		h.contextMap = singleContext[:]
	}

	if r.err() != nil {
		return
	}

	if h.lz77.enabled {
		h.lzDistCluster = h.contextMap[len(h.contextMap)-1]
	}

	usePrefix := r.bit()

	h.logAlphaSize = huffmanMaxBits
	if !usePrefix {
		h.logAlphaSize = int(r.bits(2)) + 5
	}

	numHistograms := 0
	for _, c := range h.contextMap {
		numHistograms = max(numHistograms, int(c)+1)
	}

	h.uintConfigs = make([]hybridUint, numHistograms)
	for i := range h.uintConfigs {
		h.uintConfigs[i].read(r, h.logAlphaSize)
	}

	if r.err() != nil {
		return
	}

	h.usePrefix = usePrefix

	if usePrefix {
		h.huff.read(r, numHistograms)
	} else {
		h.ans.read(r, numHistograms, h.logAlphaSize)
	}
}

func (h *histograms) resize(numContexts int) {
	for len(h.contextMap) < numContexts {
		h.contextMap = append(h.contextMap, 0)
	}
}

func (h *histograms) token(r *bitReader, sr *symbolReader, cluster int) uint32 {
	if h.usePrefix {
		return h.huff.decode(r, cluster)
	}

	return sr.ans.decode(&h.ans, r, cluster)
}

const (
	lz77LogWindow  = 20
	lz77WindowMask = 1<<lz77LogWindow - 1
)

var lz77SpecialDistances = [120][2]int8{
	{0, 1}, {1, 0}, {1, 1}, {-1, 1}, {0, 2}, {2, 0}, {1, 2}, {-1, 2}, {2, 1}, {-2, 1},
	{2, 2}, {-2, 2}, {0, 3}, {3, 0}, {1, 3}, {-1, 3}, {3, 1}, {-3, 1}, {2, 3}, {-2, 3},
	{3, 2}, {-3, 2}, {0, 4}, {4, 0}, {1, 4}, {-1, 4}, {4, 1}, {-4, 1}, {3, 3}, {-3, 3},
	{2, 4}, {-2, 4}, {4, 2}, {-4, 2}, {0, 5}, {3, 4}, {-3, 4}, {4, 3}, {-4, 3}, {5, 0},
	{1, 5}, {-1, 5}, {5, 1}, {-5, 1}, {2, 5}, {-2, 5}, {5, 2}, {-5, 2}, {4, 4}, {-4, 4},
	{3, 5}, {-3, 5}, {5, 3}, {-5, 3}, {0, 6}, {6, 0}, {1, 6}, {-1, 6}, {6, 1}, {-6, 1},
	{2, 6}, {-2, 6}, {6, 2}, {-6, 2}, {4, 5}, {-4, 5}, {5, 4}, {-5, 4}, {3, 6}, {-3, 6},
	{6, 3}, {-6, 3}, {0, 7}, {7, 0}, {1, 7}, {-1, 7}, {5, 5}, {-5, 5}, {7, 1}, {-7, 1},
	{4, 6}, {-4, 6}, {6, 4}, {-6, 4}, {2, 7}, {-2, 7}, {7, 2}, {-7, 2}, {3, 7}, {-3, 7},
	{7, 3}, {-7, 3}, {5, 6}, {-5, 6}, {6, 5}, {-6, 5}, {8, 0}, {4, 7}, {-4, 7}, {7, 4},
	{-7, 4}, {8, 1}, {8, 2}, {6, 6}, {-6, 6}, {8, 3}, {5, 7}, {-5, 7}, {7, 5}, {-7, 5},
	{8, 4}, {6, 7}, {-6, 7}, {7, 6}, {-7, 6}, {8, 5}, {7, 7}, {-7, 7}, {8, 6}, {8, 7},
}

type lz77State struct {
	minSymbol      uint32
	minLength      uint32
	distMultiplier uint32
	window         []uint32
	numToCopy      uint32
	copyPos        uint32
	numDecoded     uint32
}

func (s *lz77State) applyCopy(distanceSym, numToCopy uint32) {
	var distSub1 uint32

	switch {
	case s.distMultiplier == 0:
		distSub1 = distanceSym
	case distanceSym >= 120:
		distSub1 = distanceSym - 120
	default:
		d := lz77SpecialDistances[distanceSym]
		v := int64(s.distMultiplier)*int64(d[1]) + int64(d[0]) - 1

		if v < 0 || v > 0xffffffff {
			distSub1 = 0
		} else {
			distSub1 = uint32(v)
		}
	}

	distance := min(min(distSub1, (1<<20)-1)+1, s.numDecoded)
	s.copyPos = s.numDecoded - distance
	s.numToCopy = numToCopy
}

func (s *lz77State) push(token uint32) {
	offset := s.numDecoded & lz77WindowMask

	if int(offset) < len(s.window) {
		s.window[offset] = token
	} else {
		s.window = append(s.window, token)
	}

	s.numDecoded++
}

func (s *lz77State) pull() (uint32, bool) {
	if s.numToCopy == 0 {
		return 0, false
	}

	sym := s.window[s.copyPos&lz77WindowMask]
	s.copyPos++
	s.numToCopy--

	return sym, true
}

type symbolReader struct {
	useLZ77 bool
	lz77    lz77State
	ans     ansReader
	bad     bool
}

func (s *symbolReader) init(h *histograms, r *bitReader, imageWidth int) {
	*s = symbolReader{}

	if !h.usePrefix {
		s.ans.init(r)
	} else {
		s.ans.state = ansChecksum
	}

	if h.lz77.enabled {
		s.useLZ77 = true
		s.lz77 = lz77State{
			minSymbol:      h.lz77.minSymbol,
			minLength:      h.lz77.minLength,
			distMultiplier: uint32(imageWidth),
		}
	}
}

func (s *symbolReader) readUnsigned(h *histograms, r *bitReader, context int) uint32 {
	return s.readClustered(h, r, int(h.contextMap[context]))
}

func (s *symbolReader) readSigned(h *histograms, r *bitReader, context int) int32 {
	return unpackSigned(s.readUnsigned(h, r, context))
}

func (s *symbolReader) readClustered(h *histograms, r *bitReader, cluster int) uint32 {
	if !s.useLZ77 {
		return h.uintConfigs[cluster].decode(h.token(r, s, cluster), r)
	}

	if sym, ok := s.lz77.pull(); ok {
		s.lz77.push(sym)

		return sym
	}

	token := h.token(r, s, cluster)

	if token < s.lz77.minSymbol {
		sym := h.uintConfigs[cluster].decode(token, r)
		s.lz77.push(sym)

		return sym
	}

	if s.lz77.numDecoded == 0 {
		s.bad = true

		return 0
	}

	numToCopy := h.lz77LengthUint.decode(token-s.lz77.minSymbol, r)

	if numToCopy+s.lz77.minLength < numToCopy {
		s.bad = true

		return 0
	}

	numToCopy += s.lz77.minLength

	dc := int(h.lzDistCluster)
	distanceSym := h.uintConfigs[dc].decode(h.token(r, s, dc), r)

	s.lz77.applyCopy(distanceSym, numToCopy)

	sym, ok := s.lz77.pull()
	if !ok {
		s.bad = true

		return 0
	}

	s.lz77.push(sym)

	return sym
}

func (s *symbolReader) checkFinalState(h *histograms, r *bitReader) error {
	if s.bad {
		return ErrInvalid
	}
	if err := r.err(); err != nil {
		return err
	}
	if h.usePrefix {
		return nil
	}

	return s.ans.checkFinalState()
}
