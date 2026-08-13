package jxl

import (
	"math"
	"math/bits"
	"slices"
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

func valueHistogram(values []uint32) ([]uint32, []uint32) {
	sorted := make([]uint32, len(values))
	copy(sorted, values)
	slices.Sort(sorted)

	var distinct, weights []uint32

	for i := 0; i < len(sorted); {
		j := i

		for j < len(sorted) && sorted[j] == sorted[i] {
			j++
		}

		distinct = append(distinct, sorted[i])
		weights = append(weights, uint32(j-i))
		i = j
	}

	return distinct, weights
}

func uintConfigCost(h hybridUint, distinct, weights []uint32, tokens []uint32, maxAlphabet int) (float64, bool) {
	var raw, total float64

	for i, v := range distinct {
		token, nbits, _ := h.encode(v)

		if int(token) >= maxAlphabet {
			for _, u := range distinct[:i] {
				t, _, _ := h.encode(u)
				tokens[t] = 0
			}

			return 0, false
		}

		tokens[token] += weights[i]
		raw += float64(weights[i]) * float64(nbits)
		total += float64(weights[i])
	}

	cost := raw

	for _, v := range distinct {
		token, _, _ := h.encode(v)

		if c := tokens[token]; c > 0 {
			cost += float64(c)*math.Log2(total/float64(c)) + tableBitsPerSymbol
			tokens[token] = 0
		}
	}

	return cost, true
}

const tableBitsPerSymbol = 3

func chooseUintConfig(distinct, weights []uint32, logAlphaSize, maxAlphabet int) hybridUint {
	best := newHybridUint(uint32(logAlphaSize), 0, 0)
	bestCost := math.Inf(1)
	tokens := make([]uint32, maxAlphabet)

	for se := 0; se <= logAlphaSize; se++ {
		for msb := 0; msb <= se; msb++ {
			for lsb := 0; lsb+msb <= se; lsb++ {
				if se == logAlphaSize && (msb != 0 || lsb != 0) {
					continue
				}

				h := newHybridUint(uint32(se), uint32(msb), uint32(lsb))

				cost, ok := uintConfigCost(h, distinct, weights, tokens, maxAlphabet)
				if ok && cost < bestCost {
					best, bestCost = h, cost
				}
			}
		}
	}

	return best
}
