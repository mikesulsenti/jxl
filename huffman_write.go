package jxl

import (
	"cmp"
	"math/bits"
	"slices"
)

type prefixCode struct {
	code uint16
	bits uint8
}

const codeLengthRepeatZero = codeLengthRepeatCode + 1

var (
	codeLengthCodes = [6]prefixCode{
		{0, 2}, {7, 4}, {3, 3}, {2, 2}, {1, 2}, {15, 4},
	}
	simpleLengths = [5][4]uint8{
		{}, {0}, {1, 1}, {1, 2, 2}, {1, 2, 3, 3},
	}
)

func writeSimplePrefix(w *bitWriter, alSize int, symbols []uint16, special bool) {
	if len(symbols) < 1 || len(symbols) > 4 || alSize < 2 {
		w.setErr(ErrInvalid)

		return
	}

	maxBits := ceilLog2(uint32(alSize))

	for i, s := range symbols {
		if int(s) >= alSize {
			w.setErr(ErrInvalid)

			return
		}

		for _, o := range symbols[:i] {
			if o == s {
				w.setErr(ErrInvalid)

				return
			}
		}
	}

	w.bits(2, 1)
	w.bits(2, uint64(len(symbols)-1))

	for _, s := range symbols {
		w.bits(maxBits, uint64(s))
	}

	if len(symbols) == 4 {
		w.bit(special)
	}
}

func limitedLengths(counts []uint32, limit int) []uint8 {
	type leaf struct {
		w uint32
		s int
	}

	out := make([]uint8, len(counts))

	var leaves []leaf

	for s, c := range counts {
		if c > 0 {
			leaves = append(leaves, leaf{c, s})
		}
	}

	slices.SortFunc(leaves, func(a, b leaf) int {
		return cmp.Or(cmp.Compare(a.w, b.w), cmp.Compare(a.s, b.s))
	})

	n := len(leaves)

	switch {
	case n == 0:
		return out
	case n == 1:
		out[leaves[0].s] = 1

		return out
	case n > 1<<limit:
		return nil
	}

	type coin struct {
		w    uint64
		used int
	}

	levels := make([][]coin, limit)

	for l := range levels {
		var prev []coin

		if l > 0 {
			prev = levels[l-1]
		}

		list := make([]coin, 0, n+len(prev)/2)
		i, j, used := 0, 0, 0

		for i < n || j+1 < len(prev) {
			w := uint64(0)
			pick := i < n

			if j+1 < len(prev) {
				w = prev[j].w + prev[j+1].w
				pick = i < n && uint64(leaves[i].w) <= w
			}

			if pick {
				w = uint64(leaves[i].w)
				i++
				used++
			} else {
				j += 2
			}

			list = append(list, coin{w, used})
		}

		levels[l] = list
	}

	for l, m := limit-1, 2*n-2; l >= 0 && m > 0; l-- {
		if m > len(levels[l]) {
			return nil
		}

		k := levels[l][m-1].used

		for _, lf := range leaves[:k] {
			out[lf.s]++
		}

		m = 2 * (m - k)
	}

	return out
}

func canonicalPrefix(lengths []uint8) ([]prefixCode, bool) {
	out := make([]prefixCode, len(lengths))
	used, short := 0, 0

	for _, l := range lengths {
		if l > huffmanMaxBits {
			return nil, false
		}

		if l != 0 {
			used++

			if int(l) < huffmanMaxBits {
				short++
			}
		}
	}

	if short == 1 {
		if used != 1 {
			return nil, false
		}

		return out, true
	}

	key := uint32(0)

	for bits := 1; bits <= huffmanMaxBits; bits++ {
		for s, l := range lengths {
			if int(l) != bits {
				continue
			}

			out[s] = prefixCode{code: uint16(key), bits: uint8(bits)}
			key = huffNextKey(key, bits)
		}
	}

	return out, true
}

type codeLengthToken struct {
	sym   uint8
	extra uint8
	bits  uint8
}

func appendRun(out []codeLengthToken, val uint8, n int, rep uint8, ext int) []codeLengthToken {
	if n == 3+1<<ext {
		out = append(out, codeLengthToken{sym: val})
		n--
	}

	if n < 3 {
		for range n {
			out = append(out, codeLengthToken{sym: val})
		}

		return out
	}

	start := len(out)
	n -= 3

	for {
		out = append(out, codeLengthToken{sym: rep, extra: uint8(n) & (1<<ext - 1), bits: uint8(ext)})
		n >>= ext

		if n == 0 {
			break
		}

		n--
	}

	slices.Reverse(out[start:])

	return out
}

func codeLengthTokens(lengths []uint8) []codeLengthToken {
	var out []codeLengthToken

	prev := uint8(defaultCodeLength)

	for i := 0; i < len(lengths); {
		v := lengths[i]
		n := 1

		for i+n < len(lengths) && lengths[i+n] == v {
			n++
		}

		i += n

		if v == 0 {
			out = appendRun(out, 0, n, codeLengthRepeatZero, 3)

			continue
		}

		if v != prev {
			out = append(out, codeLengthToken{sym: v})
			prev = v
			n--
		}

		out = appendRun(out, v, n, codeLengthRepeatCode, 2)
	}

	return out
}

func writeComplexPrefix(w *bitWriter, lengths []uint8) {
	last := -1

	for s, l := range lengths {
		if l != 0 {
			last = s
		}
	}

	if last < 0 {
		w.setErr(ErrInvalid)

		return
	}

	tokens := codeLengthTokens(lengths[:last+1])

	var counts [codeLengthsCode]uint32

	for _, t := range tokens {
		counts[t.sym]++
	}

	clcl := limitedLengths(counts[:], 5)
	if clcl == nil {
		w.setErr(ErrInvalid)

		return
	}

	codes, ok := canonicalPrefix(clcl)
	if !ok {
		w.setErr(ErrInvalid)

		return
	}

	skip := 0

	if clcl[1] == 0 && clcl[2] == 0 {
		skip = 2

		if clcl[3] == 0 {
			skip = 3
		}
	}

	w.bits(2, uint64(skip))

	for i, space := skip, 32; i < codeLengthsCode && space > 0; i++ {
		v := clcl[codeLengthOrder[i]]

		w.symbol(codeLengthCodes[v])

		if v != 0 {
			space -= 32 >> v
		}
	}

	for _, t := range tokens {
		w.symbol(codes[t.sym])

		if t.bits != 0 {
			w.bits(int(t.bits), uint64(t.extra))
		}
	}
}

func writePrefixCode(w *bitWriter, counts []uint32, alSize int) []prefixCode {
	if alSize < 1 || alSize > len(counts) || alSize >= 1<<huffmanMaxBits {
		w.setErr(ErrInvalid)

		return nil
	}

	counts = counts[:alSize]

	if alSize == 1 {
		return make([]prefixCode, 1)
	}

	var used []uint16

	for s, c := range counts {
		if c > 0 {
			used = append(used, uint16(s))
		}
	}

	if len(used) == 0 {
		used = append(used, 0)
	}

	lengths := make([]uint8, alSize)

	if len(used) <= 4 {
		slices.SortFunc(used, func(a, b uint16) int { return cmp.Compare(counts[b], counts[a]) })

		shape := simpleLengths[len(used)]
		special := false

		if len(used) == 4 {
			var flat, skew uint64

			for i, s := range used {
				flat += 2 * uint64(counts[s])
				skew += uint64(shape[i]) * uint64(counts[s])
			}

			if special = skew < flat; !special {
				shape = [4]uint8{2, 2, 2, 2}
			}
		}

		for i, s := range used {
			lengths[s] = shape[i]
		}

		writeSimplePrefix(w, alSize, used, special)
	} else {
		lengths = limitedLengths(counts, huffmanMaxBits)

		if lengths == nil {
			w.setErr(ErrInvalid)

			return nil
		}

		writeComplexPrefix(w, lengths)
	}

	codes, ok := canonicalPrefix(lengths)
	if !ok {
		w.setErr(ErrInvalid)

		return nil
	}

	return codes
}

func writeVarint16(w *bitWriter, v uint16) {
	if v == 0 {
		w.bit(false)

		return
	}

	w.bit(true)

	n := bits.Len16(v) - 1
	w.bits(4, uint64(n))

	if n > 0 {
		w.bits(n, uint64(v)-1<<n)
	}
}

func (c *huffmanCodes) write(w *bitWriter, counts [][]uint32) [][]prefixCode {
	sizes := make([]int, len(counts))

	for i, cnt := range counts {
		size := 1

		for s, n := range cnt {
			if n > 0 {
				size = s + 1
			}
		}

		if size >= 1<<huffmanMaxBits {
			w.setErr(ErrInvalid)

			return nil
		}

		sizes[i] = size

		writeVarint16(w, uint16(size-1))
	}

	out := make([][]prefixCode, len(counts))

	for i, cnt := range counts {
		out[i] = writePrefixCode(w, cnt, sizes[i])
	}

	return out
}

func prefixCodes(t *huffTable, alSize int) ([]prefixCode, bool) {
	out := make([]prefixCode, alSize)
	seen := make([]bool, alSize)

	for i, e := range t.entries {
		if e.bits > huffmanTableBits {
			return nil, false
		}

		s := int(e.value)

		if s >= alSize || seen[s] {
			continue
		}

		seen[s] = true
		out[s] = prefixCode{code: uint16(i), bits: e.bits}
	}

	return out, true
}

func (w *bitWriter) symbol(p prefixCode) { w.bits(int(p.bits), uint64(p.code)) }
