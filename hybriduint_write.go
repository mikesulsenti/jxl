package jxl

import (
	"math"
	"math/bits"
)

func newHybridUint(splitExponent, msbInToken, lsbInToken uint32) hybridUint {
	return hybridUint{
		splitToken:    1 << splitExponent,
		splitExponent: splitExponent,
		msbInToken:    msbInToken,
		lsbInToken:    lsbInToken,
	}
}

func (h *hybridUint) write(w *bitWriter, logAlphaSize int) {
	if h.splitExponent > uint32(logAlphaSize) || h.splitToken != 1<<h.splitExponent ||
		h.msbInToken+h.lsbInToken > h.splitExponent {
		w.setErr(ErrInvalid)

		return
	}

	w.bits(ceilLog2(uint32(logAlphaSize)+1), uint64(h.splitExponent))

	if h.splitExponent == uint32(logAlphaSize) {
		if h.msbInToken != 0 || h.lsbInToken != 0 {
			w.setErr(ErrInvalid)
		}

		return
	}

	w.bits(ceilLog2(h.splitExponent+1), uint64(h.msbInToken))
	w.bits(ceilLog2(h.splitExponent-h.msbInToken+1), uint64(h.lsbInToken))
}

func (h hybridUint) encode(v uint32) (token, nbits, extra uint32) {
	if v < h.splitToken {
		return v, 0, 0
	}

	n := uint32(bits.Len32(v)) - 1
	inToken := h.msbInToken + h.lsbInToken
	nbits = n - inToken
	low := v & (1<<h.lsbInToken - 1)
	msb := (v >> (nbits + h.lsbInToken)) & (1<<h.msbInToken - 1)
	extra = (v >> h.lsbInToken) & (1<<nbits - 1)
	token = h.splitToken + ((n-h.splitExponent)<<inToken | msb<<h.lsbInToken | low)

	return token, nbits, extra
}

func (h hybridUint) extraBits(token uint32) int {
	if token < h.splitToken {
		return 0
	}

	inToken := h.msbInToken + h.lsbInToken

	return int(h.splitExponent + (token-h.splitToken)>>inToken - inToken)
}

func uintConfigCost(h hybridUint, hist []histEntry, s *uintCostScratch,
	maxAlphabet int, limit float64,
) (float64, bool) {
	tokens, used := s.tokens, s.used[:0]

	defer func() { s.used = used }()

	var raw, total float64

	for _, e := range hist {
		token, nbits, _ := h.encode(e.value)

		if int(token) >= maxAlphabet || raw >= limit {
			for _, t := range used {
				tokens[t] = 0
			}

			return 0, false
		}

		if tokens[token] == 0 {
			used = append(used, token)
		}

		tokens[token] += e.count
		raw += float64(e.count) * float64(nbits)
		total += float64(e.count)
	}

	cost := raw

	for _, t := range used {
		c := tokens[t]
		cost += float64(c)*math.Log2(total/float64(c)) + tableBitsPerSymbol
		tokens[t] = 0
	}

	return cost, true
}

type uintCostScratch struct {
	tokens []uint32
	used   []uint32
}

const tableBitsPerSymbol = 3

func chooseUintConfig(hist []histEntry, logAlphaSize, maxAlphabet int,
	s *uintCostScratch,
) hybridUint {
	best := newHybridUint(uint32(logAlphaSize), 0, 0)
	bestCost := math.Inf(1)

	if cap(s.tokens) < maxAlphabet {
		s.tokens = make([]uint32, maxAlphabet)
	}

	s.tokens = s.tokens[:maxAlphabet]

	for se := 0; se <= logAlphaSize; se++ {
		for msb := 0; msb <= se; msb++ {
			for lsb := 0; lsb+msb <= se; lsb++ {
				if se == logAlphaSize && (msb != 0 || lsb != 0) {
					continue
				}

				h := newHybridUint(uint32(se), uint32(msb), uint32(lsb))

				cost, ok := uintConfigCost(h, hist, s, maxAlphabet, bestCost)
				if ok && cost < bestCost {
					best, bestCost = h, cost
				}
			}
		}
	}

	return best
}
