package jxl

func (l *lz77Params) write(w *bitWriter) {
	w.bit(l.enabled)

	if !l.enabled {
		return
	}

	w.u32(l.minSymbol, dVal(224), dVal(512), dVal(4096), dBitsOff(15, 8))
	w.u32(l.minLength, dVal(3), dVal(4), dBitsOff(2, 5), dBitsOff(8, 9))
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

func (h *histograms) write(w *bitWriter, counts [][]uint32) [][]prefixCode {
	if h.lz77.enabled || !h.usePrefix || h.logAlphaSize != huffmanMaxBits {
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
