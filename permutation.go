package jxl

import "math/bits"

const numPermutationContexts = 8

func permutationContext(v uint32) int {
	return min(ceilLog2(v+1), 7)
}

func lowestSetBit(v uint32) uint32 { return v & -v }

func decodeLehmer(code []uint32, slice []uint32) ([]uint32, error) {
	n := len(slice)
	if n == 0 {
		return nil, ErrInvalid
	}

	permuted := make([]uint32, n)
	copy(permuted, slice)

	padded := 1 << bits.Len32(uint32(n)-1)
	if n == 1 {
		padded = 1
	}

	temp := make([]uint32, padded)
	for i := range temp {
		temp[i] = lowestSetBit(uint32(i) + 1)
	}

	for i := range permuted {
		var c uint32
		if i < len(code) {
			c = code[i]
		}

		if int(c) > n-i-1 {
			return nil, ErrInvalid
		}

		rank := c + 1
		bit, next := padded, 0

		for bit != 0 {
			cand := next + bit
			if cand > padded {
				return nil, ErrInvalid
			}

			bit >>= 1

			if temp[cand-1] < rank {
				next = cand
				rank -= temp[cand-1]
			}
		}

		permuted[i] = slice[next]

		for next++; next <= padded; next += int(lowestSetBit(uint32(next))) {
			temp[next-1]--
		}
	}

	return permuted, nil
}

func decodePermutation(size, skip uint32, h *histograms, r *bitReader, sr *symbolReader) []uint32 {
	end := sr.readUnsigned(h, r, permutationContext(size))

	if r.err() != nil {
		return nil
	}

	if end > size-skip {
		r.setErr(ErrInvalid)

		return nil
	}

	lehmer := make([]uint32, 0, end)
	prev := uint32(0)

	for idx := skip; idx < skip+end; idx++ {
		v := sr.readUnsigned(h, r, permutationContext(prev))

		if r.err() != nil {
			return nil
		}

		if v >= size-idx {
			r.setErr(ErrInvalid)

			return nil
		}

		lehmer = append(lehmer, v)
		prev = v
	}

	perm := make([]uint32, size)
	for i := range perm {
		perm[i] = uint32(i)
	}

	permuted, err := decodeLehmer(lehmer, perm[skip:])
	if err != nil {
		r.setErr(err)

		return nil
	}

	copy(perm[skip:], permuted)

	return perm
}

func composePermutation(dst, other []uint32) {
	tmp := make([]uint32, len(dst))
	for i := range tmp {
		tmp[i] = dst[other[i]]
	}

	copy(dst, tmp)
}

func readTOCPermutation(r *bitReader, numEntries int) []uint32 {
	var h histograms

	h.read(r, numPermutationContexts, true)

	if r.err() != nil {
		return nil
	}

	var sr symbolReader

	sr.init(&h, r, 0)

	if r.err() != nil {
		return nil
	}

	return decodePermutation(uint32(numEntries), 0, &h, r, &sr)
}
