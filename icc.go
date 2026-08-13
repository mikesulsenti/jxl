package jxl

import "encoding/binary"

const (
	iccContexts   = 41
	iccHeaderSize = 128
)

type iccStream struct {
	data []byte
	pos  int
}

func (s *iccStream) left() int { return len(s.data) - s.pos }

func (s *iccStream) byteAt() (uint8, bool) {
	if s.pos >= len(s.data) {
		return 0, false
	}

	b := s.data[s.pos]
	s.pos++

	return b, true
}

func (s *iccStream) take(n int) ([]byte, bool) {
	if n < 0 || n > s.left() {
		return nil, false
	}

	b := s.data[s.pos : s.pos+n]
	s.pos += n

	return b, true
}

func iccVarint(next func() (uint8, bool)) (uint64, bool) {
	var value uint64

	for shift := 0; shift < 63; shift += 7 {
		b, ok := next()
		if !ok {
			return 0, false
		}

		value |= uint64(b&0x7f) << shift

		if b&0x80 == 0 {
			break
		}
	}

	return value, true
}

func (s *iccStream) varint() (uint64, bool) { return iccVarint(s.byteAt) }

var (
	iccCommonTags = [19][4]byte{
		{'r', 'T', 'R', 'C'}, {'r', 'X', 'Y', 'Z'}, {'c', 'p', 'r', 't'},
		{'w', 't', 'p', 't'}, {'b', 'k', 'p', 't'}, {'r', 'X', 'Y', 'Z'},
		{'g', 'X', 'Y', 'Z'}, {'b', 'X', 'Y', 'Z'}, {'k', 'X', 'Y', 'Z'},
		{'r', 'T', 'R', 'C'}, {'g', 'T', 'R', 'C'}, {'b', 'T', 'R', 'C'},
		{'k', 'T', 'R', 'C'}, {'c', 'h', 'a', 'd'}, {'d', 'e', 's', 'c'},
		{'c', 'h', 'r', 'm'}, {'d', 'm', 'n', 'd'}, {'d', 'm', 'd', 'd'},
		{'l', 'u', 'm', 'i'},
	}
	iccCommonData = [8][4]byte{
		{'X', 'Y', 'Z', ' '}, {'d', 'e', 's', 'c'}, {'t', 'e', 'x', 't'},
		{'m', 'l', 'u', 'c'}, {'p', 'a', 'r', 'a'}, {'c', 'u', 'r', 'v'},
		{'s', 'f', '3', '2'}, {'g', 'b', 'd', ' '},
	}
)

func iccContext(out []byte, b1, b2 uint8) int {
	if len(out) <= iccHeaderSize {
		return 0
	}

	var p1 int

	switch {
	case b1 >= 'a' && b1 <= 'z' || b1 >= 'A' && b1 <= 'Z':
		p1 = 0
	case b1 >= '0' && b1 <= '9' || b1 == '.' || b1 == ',':
		p1 = 1
	case b1 <= 1:
		p1 = 2 + int(b1)
	case b1 <= 15:
		p1 = 4
	case b1 >= 241 && b1 <= 254:
		p1 = 5
	case b1 == 255:
		p1 = 6
	default:
		p1 = 7
	}

	var p2 int

	switch {
	case b2 >= 'a' && b2 <= 'z' || b2 >= 'A' && b2 <= 'Z':
		p2 = 0
	case b2 >= '0' && b2 <= '9' || b2 == '.' || b2 == ',':
		p2 = 1
	case b2 <= 15:
		p2 = 2
	case b2 >= 241:
		p2 = 3
	default:
		p2 = 4
	}

	return 1 + p1 + 8*p2
}

func readICCStream(r *bitReader) []byte {
	n := r.u64()
	if n > 1<<24 || int(n) > r.left() {
		r.setErr(ErrInvalid)

		return nil
	}

	var h histograms
	h.read(r, iccContexts, true)

	if r.err() != nil {
		return nil
	}

	var sr symbolReader

	sr.init(&h, r, 0)

	out := make([]byte, 0, n)
	var b1, b2 uint8

	for range n {
		sym := sr.readUnsigned(&h, r, iccContext(out, b1, b2))

		if sym >= 256 || r.err() != nil {
			r.setErr(ErrInvalid)

			return nil
		}

		out = append(out, uint8(sym))
		b1, b2 = uint8(sym), b1
	}

	if err := sr.checkFinalState(&h, r); err != nil {
		r.setErr(err)

		return nil
	}

	return out
}

func iccPredictHeader(profile []byte, header []byte, pos int, outputSize uint32) uint8 {
	switch {
	case pos <= 3:
		return byte(outputSize >> (8 * (3 - pos)))
	case pos == 8:
		return 4
	case pos >= 12 && pos <= 23:
		return "mntrRGB XYZ "[pos-12]
	case pos >= 36 && pos <= 39:
		return "acsp"[pos-36]
	case pos >= 41 && pos <= 43 && len(profile) > 40:
		switch profile[40] {
		case 'A':
			return "?PPL"[pos-40]
		case 'M':
			return "?SFT"[pos-40]
		case 'S':
			if pos >= 42 && len(profile) > 41 {
				switch profile[41] {
				case 'G':
					return "??I "[pos-40]
				case 'U':
					return "??NW"[pos-40]
				}
			}
		}
	case pos == 70:
		return 246
	case pos == 71:
		return 214
	case pos == 73:
		return 1
	case pos == 78:
		return 211
	case pos == 79:
		return 45
	case pos >= 80 && pos <= 83 && len(profile) > 4+pos-80:
		return profile[4+pos-80]
	}

	return 0
}

func iccShuffle(b []byte, width int) []byte {
	out := make([]byte, 0, len(b))

	if width == 2 {
		height, odd := len(b)/2, len(b)%2

		for i := range height {
			out = append(out, b[i], b[i+height+odd])
		}
		if odd != 0 {
			out = append(out, b[height])
		}

		return out
	}

	step, wide := len(b)/4, len(b)%4

	for i := range step {
		base := i

		for range wide {
			out = append(out, b[base])
			base += step + 1
		}
		for j := wide; j < 4; j++ {
			out = append(out, b[base])
			base += step
		}
	}

	for i := 1; i <= wide; i++ {
		out = append(out, b[(step+1)*i-1])
	}

	return out
}

type iccWriter struct {
	buf []byte
	pos int
}

func (w *iccWriter) write(b ...byte) bool {
	if w.pos+len(b) > len(w.buf) {
		return false
	}

	copy(w.buf[w.pos:], b)
	w.pos += len(b)

	return true
}

func (w *iccWriter) writeU32(v uint32) bool {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)

	return w.write(b[:]...)
}

func decodeICC(encoded []byte) ([]byte, bool) {
	data := &iccStream{data: encoded}

	outputSize, ok := data.varint()
	if !ok {
		return nil, false
	}

	commandsSize, ok := data.varint()
	if !ok {
		return nil, false
	}

	if uint64(data.pos)+commandsSize > uint64(len(encoded)) ||
		outputSize > 1<<28 || outputSize+65536 < uint64(len(encoded)) {
		return nil, false
	}

	raw, ok := data.take(int(commandsSize))
	if !ok {
		return nil, false
	}

	cmds := &iccStream{data: raw}

	headerSize := min(outputSize, iccHeaderSize)

	headerData, ok := data.take(int(headerSize))
	if !ok {
		return nil, false
	}

	profile := make([]byte, 0, outputSize)
	for i, e := range headerData {
		profile = append(profile, iccPredictHeader(profile, headerData, i, uint32(outputSize))+e)
	}

	if outputSize <= iccHeaderSize {
		return profile, true
	}

	out := &iccWriter{buf: make([]byte, outputSize), pos: iccHeaderSize}
	copy(out.buf, profile)

	v, ok := cmds.varint()
	if !ok {
		return nil, false
	}

	if v >= 1 {
		numTags := v - 1
		if (outputSize-iccHeaderSize)/12 < numTags {
			return nil, false
		}

		if !out.writeU32(uint32(numTags)) {
			return nil, false
		}

		if !iccTagList(data, cmds, out, uint32(numTags), outputSize) {
			return nil, false
		}
	}

	for {
		command, ok := cmds.byteAt()
		if !ok {
			break
		}

		if !iccCommand(data, cmds, out, command) {
			return nil, false
		}
	}

	if out.pos != int(outputSize) {
		return nil, false
	}

	return out.buf, true
}

func iccTagList(data, cmds *iccStream, out *iccWriter, numTags uint32, outputSize uint64) bool {
	prevStart := numTags*12 + iccHeaderSize
	prevSize := uint32(0)

	for {
		command, ok := cmds.byteAt()
		if !ok {
			return true
		}

		code := command & 63

		var tag [4]byte

		switch {
		case code == 0:
			return true
		case code == 1:
			b, ok := data.take(4)
			if !ok {
				return false
			}

			copy(tag[:], b)
		case code <= 20:
			tag = iccCommonTags[code-2]
		default:
			return false
		}

		start := prevStart + prevSize

		if command&64 != 0 {
			v, ok := cmds.varint()
			if !ok {
				return false
			}

			start = uint32(v)
		}

		size := prevSize

		switch {
		case command&128 != 0:
			v, ok := cmds.varint()
			if !ok {
				return false
			}

			size = uint32(v)
		case tag == iccCommonTags[1] || tag == iccCommonTags[6] || tag == iccCommonTags[7] ||
			tag == iccCommonTags[8] || tag == iccCommonTags[3] || tag == iccCommonTags[4] ||
			tag == iccCommonTags[18]:
			size = 20
		}

		if uint64(start)+uint64(size) > outputSize {
			return false
		}

		prevStart, prevSize = start, size

		if !out.write(tag[:]...) || !out.writeU32(start) || !out.writeU32(size) {
			return false
		}

		switch code {
		case 2:
			if !out.write('g', 'T', 'R', 'C') || !out.writeU32(start) || !out.writeU32(size) ||
				!out.write('b', 'T', 'R', 'C') || !out.writeU32(start) || !out.writeU32(size) {
				return false
			}
		case 3:
			if !out.write('g', 'X', 'Y', 'Z') || !out.writeU32(start+size) || !out.writeU32(size) ||
				!out.write('b', 'X', 'Y', 'Z') || !out.writeU32(start+size*2) || !out.writeU32(size) {
				return false
			}
		}
	}
}

func iccCommand(data, cmds *iccStream, out *iccWriter, command uint8) bool {
	switch {
	case command == 1:
		n, ok := cmds.varint()
		if !ok {
			return false
		}

		b, ok := data.take(int(n))

		return ok && out.write(b...)

	case command == 2 || command == 3:
		n, ok := cmds.varint()
		if !ok {
			return false
		}

		b, ok := data.take(int(n))
		if !ok {
			return false
		}

		width := 2
		if command == 3 {
			width = 4
		}

		return out.write(iccShuffle(b, width)...)

	case command == 4:
		return iccPredictCommand(data, cmds, out)

	case command == 10:
		var b [20]byte
		copy(b[:], "XYZ ")

		v, ok := data.take(12)
		if !ok {
			return false
		}

		copy(b[8:], v)

		return out.write(b[:]...)

	case command >= 16 && command <= 23:
		t := iccCommonData[command-16]

		return out.write(t[0], t[1], t[2], t[3], 0, 0, 0, 0)
	}

	return false
}

func iccPredictCommand(data, cmds *iccStream, out *iccWriter) bool {
	flags, ok := cmds.byteAt()
	if !ok {
		return false
	}

	width := int(flags&3) + 1
	order := int(flags>>2) & 3

	if width == 3 || order == 3 {
		return false
	}

	stride := width

	if flags&16 != 0 {
		v, ok := cmds.varint()
		if !ok {
			return false
		}

		stride = int(v)
		if stride < width {
			return false
		}
	}

	if stride*4 >= out.pos {
		return false
	}

	n, ok := cmds.varint()
	if !ok {
		return false
	}

	b, ok := data.take(int(n))
	if !ok {
		return false
	}

	if width > 1 {
		b = iccShuffle(b, width)
	}

	for i := 0; i < int(n); i += width {
		base := out.pos

		var prev [3]uint32

		for j := 0; j <= order; j++ {
			off := base - stride*(j+1)
			if off < 0 || off+width > len(out.buf) {
				return false
			}

			var pb [4]byte
			copy(pb[4-width:], out.buf[off:off+width])
			prev[j] = binary.BigEndian.Uint32(pb[:])
		}

		var p uint32

		switch order {
		case 0:
			p = prev[0]
		case 1:
			p = 2*prev[0] - prev[1]
		default:
			p = 3*(prev[0]-prev[1]) + prev[2]
		}

		for j := 0; j < width && i+j < int(n); j++ {
			v := uint32(b[i+j]) + (p >> (8 * (width - 1 - j)))

			if !out.write(uint8(v)) {
				return false
			}
		}
	}

	return true
}
