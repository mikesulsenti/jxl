package jxl

import "slices"

const (
	huffmanMaxBits       = 15
	huffmanTableBits     = 8
	huffmanTableSize     = 1 << huffmanTableBits
	codeLengthsCode      = 18
	defaultCodeLength    = 8
	codeLengthRepeatCode = 16
)

type huffEntry struct {
	value uint16
	bits  uint8
}

type huffTable struct {
	entries []huffEntry
}

func huffNextKey(key uint32, length int) uint32 {
	step := uint32(1) << (length - 1)
	for key&step != 0 {
		step >>= 1
	}

	return (key & (step - 1)) + step
}

func huffReplicate(table []huffEntry, step int, v huffEntry) {
	for i := 0; i < len(table); i += step {
		table[i] = v
	}
}

func huffNextTableBitSize(count *[huffmanMaxBits + 1]uint16, length, rootBits int) int {
	left := uint16(1) << (length - rootBits)

	for length < huffmanMaxBits {
		if left <= count[length] {
			break
		}

		left -= count[length]
		length++
		left <<= 1
	}

	return length - rootBits
}

func huffSimpleTable(r *bitReader, alSize int) []huffEntry {
	maxBits := ceilLog2(uint32(alSize))
	num := int(r.bits(2)) + 1

	var symbols [4]uint16

	for i := range num {
		s := int(r.bits(maxBits))
		if s >= alSize {
			r.setErr(ErrInvalid)

			return nil
		}

		symbols[i] = uint16(s)
	}

	for i := 1; i < num; i++ {
		if slices.Contains(symbols[:i], symbols[i]) {
			r.setErr(ErrInvalid)

			return nil
		}
	}

	special := num == 4 && r.bit()
	out := make([]huffEntry, huffmanTableSize)

	switch {
	case num == 1:
		for i := range out {
			out[i] = huffEntry{value: symbols[0]}
		}
	case num == 2:
		slices.Sort(symbols[0:2])

		for i := 0; i < huffmanTableSize; i += 2 {
			out[i] = huffEntry{symbols[0], 1}
			out[i+1] = huffEntry{symbols[1], 1}
		}
	case num == 3:
		slices.Sort(symbols[1:3])

		for i := 0; i < huffmanTableSize; i += 4 {
			out[i] = huffEntry{symbols[0], 1}
			out[i+1] = huffEntry{symbols[1], 2}
			out[i+2] = huffEntry{symbols[0], 1}
			out[i+3] = huffEntry{symbols[2], 2}
		}
	case !special:
		slices.Sort(symbols[:])

		for i := 0; i < huffmanTableSize; i += 4 {
			out[i] = huffEntry{symbols[0], 2}
			out[i+1] = huffEntry{symbols[2], 2}
			out[i+2] = huffEntry{symbols[1], 2}
			out[i+3] = huffEntry{symbols[3], 2}
		}
	default:
		slices.Sort(symbols[2:4])

		for i := 0; i < huffmanTableSize; i += 8 {
			out[i] = huffEntry{symbols[0], 1}
			out[i+1] = huffEntry{symbols[1], 2}
			out[i+2] = huffEntry{symbols[0], 1}
			out[i+3] = huffEntry{symbols[2], 3}
			out[i+4] = huffEntry{symbols[0], 1}
			out[i+5] = huffEntry{symbols[1], 2}
			out[i+6] = huffEntry{symbols[0], 1}
			out[i+7] = huffEntry{symbols[3], 3}
		}
	}

	return out
}

func huffBuild(r *bitReader, rootBits int, codeLengths []uint8) []huffEntry {
	if len(codeLengths) > 1<<huffmanMaxBits {
		r.setErr(ErrInvalid)

		return nil
	}

	var counts [huffmanMaxBits + 1]uint16

	for _, v := range codeLengths {
		counts[v]++
	}

	sorted := make([]uint16, len(codeLengths))

	var (
		offset    [huffmanMaxBits + 1]uint16
		maxLength = 1
		sum       uint16
	)

	for l := 1; l <= huffmanMaxBits; l++ {
		offset[l] = sum

		if counts[l] != 0 {
			sum += counts[l]
			maxLength = l
		}
	}

	for symbol, l := range codeLengths {
		if l != 0 {
			sorted[offset[l]] = uint16(symbol)
			offset[l]++
		}
	}

	tableBits := rootBits
	tableSize := 1 << tableBits
	tablePos := 0
	table := make([]huffEntry, tableSize)

	if offset[huffmanMaxBits] == 1 {
		for i := range table {
			table[i] = huffEntry{value: sorted[0]}
		}

		return table
	}

	if tableBits > maxLength {
		tableBits = maxLength
		tableSize = 1 << tableBits
	}

	key := uint32(0)
	symbol := 0

	for bits := 1; bits <= tableBits; bits++ {
		for counts[bits] != 0 {
			if symbol >= len(sorted) || int(key) >= tableSize {
				r.setErr(ErrInvalid)

				return nil
			}

			huffReplicate(table[key:], 1<<bits, huffEntry{sorted[symbol], uint8(bits)})
			symbol++
			key = huffNextKey(key, bits)
			counts[bits]--
		}
	}

	for len(table) != tableSize {
		copy(table[tableSize:2*tableSize], table[:tableSize])
		tableSize <<= 1
	}

	mask := uint32(len(table) - 1)
	low := ^uint32(0)

	for l := rootBits + 1; l <= maxLength; l++ {
		for counts[l] != 0 {
			if key&mask != low {
				tablePos += tableSize
				tableBits = huffNextTableBitSize(&counts, l, rootBits)
				tableSize = 1 << tableBits
				low = key & mask
				table[low].bits = uint8(tableBits + rootBits)
				table[low].value = uint16(tablePos - int(low))

				if n := tablePos + tableSize; len(table) < n {
					table = append(table, make([]huffEntry, n-len(table))...)
				}
			}

			counts[l]--

			pos := tablePos + int(key>>rootBits)
			if symbol >= len(sorted) || pos >= len(table) {
				r.setErr(ErrInvalid)

				return nil
			}

			huffReplicate(table[pos:], 1<<(l-rootBits), huffEntry{sorted[symbol], uint8(l - rootBits)})
			symbol++
			key = huffNextKey(key, l)
		}
	}

	return table
}

func huffCodeLengths(r *bitReader, clcl [codeLengthsCode]uint8, alSize int) []uint8 {
	table := huffBuild(r, 5, clcl[:])
	if table == nil {
		return nil
	}

	var (
		symbol        int
		prevCodeLen   = uint8(defaultCodeLength)
		repeat        int
		repeatCodeLen uint8
		space         = 1 << 15
	)

	lengths := make([]uint8, alSize)

	for symbol < alSize && space > 0 {
		e := table[r.peek(5)]
		r.consume(int(e.bits))

		codeLen := uint8(e.value)

		if codeLen < codeLengthRepeatCode {
			repeat = 0
			lengths[symbol] = codeLen
			symbol++

			if codeLen != 0 {
				prevCodeLen = codeLen
				space -= 32768 >> codeLen

				if space < 0 {
					r.setErr(ErrInvalid)

					return nil
				}
			}

			continue
		}

		extraBits := int(codeLen) - 14
		newLen := uint8(0)

		if codeLen == codeLengthRepeatCode {
			newLen = prevCodeLen
		}
		if repeatCodeLen != newLen {
			repeat = 0
			repeatCodeLen = newLen
		}

		old := repeat

		if repeat > 0 {
			repeat -= 2
			repeat <<= extraBits
		}

		repeat += int(r.bits(extraBits)) + 3

		delta := repeat - old
		if symbol+delta > alSize {
			r.setErr(ErrInvalid)

			return nil
		}

		for i := range delta {
			lengths[symbol+i] = repeatCodeLen
		}

		symbol += delta

		if repeatCodeLen != 0 {
			space -= delta << (15 - repeatCodeLen)

			if space < 0 {
				r.setErr(ErrInvalid)

				return nil
			}
		}
	}

	if space != 0 {
		r.setErr(ErrInvalid)

		return nil
	}

	return lengths
}

var (
	staticHuffBits  = [16]uint8{2, 2, 2, 3, 2, 2, 2, 4, 2, 2, 2, 3, 2, 2, 2, 4}
	staticHuffVals  = [16]uint8{0, 4, 3, 2, 0, 4, 3, 1, 0, 4, 3, 2, 0, 4, 3, 5}
	codeLengthOrder = [codeLengthsCode]uint8{
		1, 2, 3, 4, 0, 5, 17, 6, 16, 7, 8, 9, 10, 11, 12, 13, 14, 15,
	}
)

func (t *huffTable) read(r *bitReader, alSize int) {
	if alSize == 1 {
		t.entries = make([]huffEntry, huffmanTableSize)

		return
	}

	if alSize >= 1<<huffmanMaxBits {
		r.setErr(ErrInvalid)

		return
	}

	skip := int(r.bits(2))

	if skip == 1 {
		t.entries = huffSimpleTable(r, alSize)

		return
	}

	var (
		clcl     [codeLengthsCode]uint8
		space    = 32
		numCodes int
	)

	for i := skip; i < codeLengthsCode; i++ {
		if space <= 0 {
			break
		}

		idx := r.peek(4)
		r.consume(int(staticHuffBits[idx]))

		v := staticHuffVals[idx]
		clcl[codeLengthOrder[i]] = v

		if v != 0 {
			space -= 32 >> v
			numCodes++
		}
	}

	if numCodes != 1 && space != 0 {
		r.setErr(ErrInvalid)

		return
	}

	lengths := huffCodeLengths(r, clcl, alSize)
	if lengths == nil {
		return
	}

	t.entries = huffBuild(r, huffmanTableBits, lengths)
}

func (t *huffTable) decode(r *bitReader) uint32 {
	pos := int(r.peek(huffmanTableBits))
	nbits := int(t.entries[pos].bits)

	if nbits > huffmanTableBits {
		r.consumeOpt(huffmanTableBits)
		nbits -= huffmanTableBits
		pos += int(t.entries[pos].value)
		pos += int(r.peek(nbits))
	}

	e := t.entries[pos]
	r.consumeOpt(int(e.bits))

	return uint32(e.value)
}

type huffmanCodes struct {
	tables []huffTable
}

func decodeVarint16(r *bitReader) uint16 {
	if !r.bit() {
		return 0
	}

	n := int(r.bits(4))
	if n == 0 {
		return 1
	}

	return uint16(1<<n) + uint16(r.bits(n))
}

func (h *huffmanCodes) read(r *bitReader, num int) {
	sizes := make([]int, num)
	maxSize := 0

	for i := range sizes {
		sizes[i] = int(decodeVarint16(r)) + 1
		maxSize = max(maxSize, sizes[i])
	}

	if maxSize >= 1<<huffmanMaxBits {
		r.setErr(ErrInvalid)

		return
	}

	h.tables = make([]huffTable, num)

	for i, s := range sizes {
		h.tables[i].read(r, s)

		if r.err() != nil {
			return
		}
	}
}

func (h *huffmanCodes) decode(r *bitReader, ctx int) uint32 { return h.tables[ctx].decode(r) }
