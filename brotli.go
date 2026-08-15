package jxl

const (
	brotliCommandSymbols   = 704
	brotliBlockLenSymbols  = 26
	brotliShortDistances   = 16
	brotliLiteralContexts  = 1 << 6
	brotliDistanceContexts = 1 << 2
	brotliMaxWordLength    = 24
	brotliMaxTransformed   = brotliMaxWordLength + 13 + 13
)

var brotliDictBits = [32]uint8{
	0, 0, 0, 0, 10, 10, 11, 11,
	10, 10, 10, 10, 10, 9, 9, 8,
	7, 7, 8, 7, 7, 6, 6, 5,
	5, 0, 0, 0, 0, 0, 0, 0,
}

var brotliDictOffset = [32]uint32{
	0, 0, 0, 0, 0, 4096, 9216, 21504,
	35840, 44032, 53248, 63488, 74752, 87040, 93696, 100864,
	104704, 106752, 108928, 113536, 115968, 118528, 119872, 121280,
	122016, 122784, 122784, 122784, 122784, 122784, 122784, 122784,
}

type brotliRange struct {
	offset uint32
	bits   uint8
}

var brotliBlockLenCode = [brotliBlockLenSymbols]brotliRange{
	{1, 2}, {5, 2}, {9, 2}, {13, 2}, {17, 3}, {25, 3},
	{33, 3}, {41, 3}, {49, 4}, {65, 4}, {81, 4}, {97, 4},
	{113, 5}, {145, 5}, {177, 5}, {209, 5}, {241, 6}, {305, 6},
	{369, 7}, {497, 8}, {753, 9}, {1265, 10}, {2289, 11}, {4337, 12},
	{8433, 13}, {16625, 24},
}

type brotliCommand struct {
	insertOffset, copyOffset uint16
	insertBits, copyBits     uint8
	context                  uint8
	implicitZero             bool
}

var brotliCommandCode = func() (t [brotliCommandSymbols]brotliCommand) {
	insertBits := [24]uint8{
		0, 0, 0, 0, 0, 0, 1, 1, 2, 2, 3, 3,
		4, 4, 5, 5, 6, 7, 8, 9, 10, 12, 14, 24,
	}
	copyBits := [24]uint8{
		0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 2,
		3, 3, 4, 4, 5, 5, 6, 7, 8, 9, 10, 24,
	}
	cellPos := [11]uint32{0, 1, 0, 1, 8, 9, 2, 16, 10, 17, 18}

	var insertOffset, copyOffset [24]uint16

	copyOffset[0] = 2

	for i := range 23 {
		insertOffset[i+1] = insertOffset[i] + uint16(1)<<insertBits[i]
		copyOffset[i+1] = copyOffset[i] + uint16(1)<<copyBits[i]
	}

	for symbol := range uint32(brotliCommandSymbols) {
		cell := symbol >> 6
		pos := cellPos[cell]

		copyCode := (pos<<3)&0x18 + symbol&7
		insertCode := pos&0x18 + (symbol>>3)&7

		context := uint8(3)
		if copyOffset[copyCode] <= 4 {
			context = uint8(copyOffset[copyCode]) - 2
		}

		t[symbol] = brotliCommand{
			insertOffset: insertOffset[insertCode], copyOffset: copyOffset[copyCode],
			insertBits: insertBits[insertCode], copyBits: copyBits[copyCode],
			context: context, implicitZero: cell < 2,
		}
	}

	return t
}()

type brotliGroup struct {
	trees []huffTable
}

func (g *brotliGroup) read(r *bitReader, n, alSize int) {
	g.trees = make([]huffTable, n)

	for i := range g.trees {
		brotliReadTree(r, &g.trees[i], alSize)

		if r.err() != nil {
			return
		}
	}
}

// brotliReadTree reads one prefix code, treating a code it could not build as
// a malformed stream rather than an empty table to index into.
func brotliReadTree(r *bitReader, t *huffTable, alSize int) {
	t.read(r, alSize)

	if len(t.entries) == 0 {
		r.setErr(ErrInvalid)
	}
}

type brotliBlock struct {
	types   huffTable
	lengths huffTable
	count   uint32
	num     uint32
	current uint32
	prev    uint32
}

// next switches to the block type the stream names, where zero repeats the
// type before last and one follows on from the last.
func (b *brotliBlock) next(r *bitReader) {
	t := b.types.decode(r)

	switch t {
	case 0:
		t = b.prev
	case 1:
		t = b.current + 1
	default:
		t -= 2
	}

	if t >= b.num {
		t -= b.num
	}

	b.prev, b.current = b.current, t
	b.count = brotliReadBlockLength(r, &b.lengths)
}

func brotliReadBlockLength(r *bitReader, t *huffTable) uint32 {
	code := t.decode(r)
	if code >= brotliBlockLenSymbols {
		r.setErr(ErrInvalid)

		return 1
	}

	e := brotliBlockLenCode[code]

	return e.offset + uint32(r.bits(int(e.bits)))
}

type brotliDecoder struct {
	r      *bitReader
	out    []byte
	max    int
	window int
	bits   int
	dist   [4]uint32
	idx    uint32
}

// brotliDecode decompresses a brotli stream, refusing output past max bytes.
func brotliDecode(src []byte, max int) ([]byte, error) {
	d := &brotliDecoder{
		r: newBitReader(src), max: max,
		dist: [4]uint32{16, 15, 11, 4},
	}

	bits, ok := d.windowSize()
	if !ok {
		return nil, ErrInvalid
	}

	d.bits, d.window = bits, 1<<bits-16

	for {
		last, done := d.metablock()

		if err := d.r.err(); err != nil {
			return nil, err
		}

		if last || done {
			break
		}
	}

	return d.out, d.r.err()
}

// windowSize reads the sliding window, refusing the large window extension.
func (d *brotliDecoder) windowSize() (int, bool) {
	r := d.r

	bits := 16

	switch {
	case !r.bit():
	default:
		if n := int(r.bits(3)); n != 0 {
			bits = 17 + n

			break
		}

		switch n := int(r.bits(3)); {
		case n == 1:
			return 0, false
		case n != 0:
			bits = 8 + n
		default:
			bits = 17
		}
	}

	if bits < 10 || bits > 24 {
		return 0, false
	}

	return bits, true
}

// metablock reads one meta-block, reporting whether it was the last and
// whether the stream ended inside it.
func (d *brotliDecoder) metablock() (last, done bool) {
	r := d.r

	last = r.bit()

	if last && r.bit() {
		return true, true
	}

	nibbles := int(r.bits(2)) + 4

	if nibbles == 7 {
		return d.metadata(last)
	}

	length := 0

	for i := range nibbles {
		v := int(r.bits(4))

		if i+1 == nibbles && nibbles > 4 && v == 0 {
			r.setErr(ErrInvalid)

			return last, true
		}

		length |= v << (i * 4)
	}

	length++

	uncompressed := false
	if !last {
		uncompressed = r.bit()
	}

	if len(d.out)+length > d.max {
		r.setErr(ErrUnsupported)

		return last, true
	}

	if uncompressed {
		r.skip(r.toByte())

		for range length {
			d.out = append(d.out, byte(r.bits(8)))
		}

		return last, false
	}

	d.compressed(length)

	return last, false
}

// metadata skips a metadata meta-block, which carries no output.
func (d *brotliDecoder) metadata(last bool) (bool, bool) {
	r := d.r

	if r.bit() {
		r.setErr(ErrInvalid)

		return last, true
	}

	bytes := int(r.bits(2))
	if bytes == 0 {
		return last, false
	}

	length := 0

	for i := range bytes {
		v := int(r.bits(8))

		if i+1 == bytes && bytes > 1 && v == 0 {
			r.setErr(ErrInvalid)

			return last, true
		}

		length |= v << (i * 8)
	}

	r.skip(r.toByte())
	r.skip((length + 1) * 8)

	return last, false
}

func (d *brotliDecoder) contextMap(size int) ([]uint8, uint32) {
	r := d.r

	trees := brotliVarLenUint8(r) + 1

	m := make([]uint8, size)
	if trees <= 1 {
		return m, trees
	}

	runBits := uint32(0)
	if r.bit() {
		runBits = uint32(r.bits(4)) + 1
	}

	var t huffTable

	brotliReadTree(r, &t, int(trees+runBits))

	if r.err() != nil {
		return nil, 0
	}

	for i := 0; i < size; {
		code := t.decode(r)

		if r.err() != nil {
			return nil, 0
		}

		if code == 0 {
			m[i] = 0
			i++

			continue
		}

		if code > runBits {
			m[i] = uint8(code - runBits)
			i++

			continue
		}

		reps := int(uint32(r.bits(int(code))) + 1<<code)
		if i+reps > size {
			r.setErr(ErrInvalid)

			return nil, 0
		}

		for range reps {
			m[i] = 0
			i++
		}
	}

	if r.bit() {
		brotliInverseMoveToFront(m)
	}

	return m, trees
}

func brotliInverseMoveToFront(v []uint8) {
	var mtf [256]uint8

	for i := range mtf {
		mtf[i] = uint8(i)
	}

	for i, index := range v {
		value := mtf[index]
		v[i] = value

		moveToFront(mtf[:], index)
	}
}

// brotliVarLenUint8 reads a number in 0..255 from 1 to 11 bits.
func brotliVarLenUint8(r *bitReader) uint32 {
	if !r.bit() {
		return 0
	}

	n := int(r.bits(3))
	if n == 0 {
		return 1
	}

	return 1<<n + uint32(r.bits(n))
}

func (d *brotliDecoder) compressed(length int) {
	r := d.r

	var literal, insert, distance brotliBlock

	for _, b := range []*brotliBlock{&literal, &insert, &distance} {
		b.num = brotliVarLenUint8(r) + 1
		b.count = 1 << 24
		b.current, b.prev = 0, 1

		if b.num >= 2 {
			brotliReadTree(r, &b.types, int(b.num)+2)
			brotliReadTree(r, &b.lengths, brotliBlockLenSymbols)

			if r.err() != nil {
				return
			}

			b.count = brotliReadBlockLength(r, &b.lengths)
		}

		if r.err() != nil {
			return
		}
	}

	postfix := uint32(r.bits(2))
	direct := uint32(r.bits(4)) << postfix

	modes := make([]uint8, literal.num)
	for i := range modes {
		modes[i] = uint8(r.bits(2))
	}

	if r.err() != nil {
		return
	}

	literalMap, literalTrees := d.contextMap(int(literal.num) * brotliLiteralContexts)
	distanceMap, distanceTrees := d.contextMap(int(distance.num) * brotliDistanceContexts)

	if r.err() != nil {
		return
	}

	alphabet := int(brotliShortDistances + direct + 24<<(postfix+1))

	var literals, commands, distances brotliGroup

	literals.read(r, int(literalTrees), 256)
	commands.read(r, int(insert.num), brotliCommandSymbols)
	distances.read(r, int(distanceTrees), alphabet)

	if r.err() != nil {
		return
	}

	offsets, extras := brotliDistanceTable(alphabet, postfix, direct)

	d.commands(length, &literal, &insert, &distance, literalMap, distanceMap,
		modes, &literals, &commands, &distances, offsets, extras, postfix)
}

// brotliDistanceTable maps every distance code past the short ones to its base
// and the count of extra bits that follow it.
func brotliDistanceTable(alphabet int, postfix, direct uint32) ([]uint32, []uint8) {
	offsets := make([]uint32, alphabet)
	extras := make([]uint8, alphabet)

	i := uint32(brotliShortDistances)

	for j := range direct {
		offsets[i] = j + 1
		i++
	}

	bits, half := uint32(1), uint32(0)

	for int(i) < alphabet {
		base := direct + ((2+half)<<bits-4)<<postfix + 1

		for j := range uint32(1) << postfix {
			if int(i) >= alphabet {
				break
			}

			offsets[i] = base + j
			extras[i] = uint8(bits)
			i++
		}

		bits += half
		half ^= 1
	}

	return offsets, extras
}

// ringDistance resolves the sixteen short codes against the four distances the
// stream last used.
func (d *brotliDecoder) ringDistance(code uint32) (uint32, uint32) {
	if code <= 3 {
		context := uint32(1) >> code
		dist := d.dist[(d.idx+3-code)&3]
		d.idx -= context

		return dist, context
	}

	delta, base := uint32(3), code-10

	if code < 10 {
		base = code - 4
	} else {
		delta = 2
	}

	shift := int64(0x605142>>(4*base)) & 0xf

	dist := int64(d.dist[(d.idx+delta)&3]) + shift - 3
	if dist <= 0 {
		return 1<<31 - 1, 0
	}

	return uint32(dist), 0
}

func (d *brotliDecoder) commands(length int, literal, insert, distance *brotliBlock,
	literalMap, distanceMap []uint8, modes []uint8,
	literals, commands, distances *brotliGroup, offsets []uint32, extras []uint8,
	postfix uint32,
) {
	r := d.r
	end := len(d.out) + length

	for len(d.out) < end {
		if insert.count == 0 {
			insert.next(r)
		}

		insert.count--

		code := commands.trees[insert.current].decode(r)
		if code >= brotliCommandSymbols || r.err() != nil {
			r.setErr(ErrInvalid)

			return
		}

		cmd := brotliCommandCode[code]

		n := int(cmd.insertOffset) + int(r.bits(int(cmd.insertBits)))
		copyLen := int(cmd.copyOffset) + int(r.bits(int(cmd.copyBits)))

		if len(d.out)+n > end {
			r.setErr(ErrInvalid)

			return
		}

		for range n {
			if literal.count == 0 {
				literal.next(r)
			}

			literal.count--

			ctx := brotliContext(d.out, modes[literal.current])
			tree := literalMap[literal.current<<6+uint32(ctx)]

			if int(tree) >= len(literals.trees) {
				r.setErr(ErrInvalid)

				return
			}

			d.out = append(d.out, byte(literals.trees[tree].decode(r)))
		}

		if r.err() != nil {
			return
		}

		if len(d.out) >= end {
			break
		}

		var dist, roll uint32

		if cmd.implicitZero {
			dist, roll = d.ringDistance(0)
		} else {
			if distance.count == 0 {
				distance.next(r)
			}

			distance.count--

			tree := distanceMap[distance.current<<2+uint32(cmd.context)]
			if int(tree) >= len(distances.trees) {
				r.setErr(ErrInvalid)

				return
			}

			code := distances.trees[tree].decode(r)
			if int(code) >= len(offsets) {
				r.setErr(ErrInvalid)

				return
			}

			if code < brotliShortDistances {
				dist, roll = d.ringDistance(code)
			} else {
				dist = offsets[code] + uint32(r.bits(int(extras[code])))<<postfix
			}
		}

		if r.err() != nil {
			return
		}

		if !d.copy(dist, roll, copyLen, end) {
			return
		}
	}

	if len(d.out) != end {
		r.setErr(ErrInvalid)
	}
}

// copy emits a back reference, or a dictionary word when the distance reaches
// past everything the stream has produced.
func (d *brotliDecoder) copy(dist, roll uint32, length, end int) bool {
	if int(dist) <= min(len(d.out), d.window) {
		if length > end-len(d.out) {
			d.r.setErr(ErrInvalid)

			return false
		}

		start := len(d.out) - int(dist)

		for i := range length {
			d.out = append(d.out, d.out[start+i])
		}

		d.dist[d.idx&3] = dist
		d.idx++

		return true
	}

	d.idx += roll

	word, ok := brotliDictionaryWord(dist-uint32(min(len(d.out), d.window))-1, length)
	if !ok {
		d.r.setErr(ErrInvalid)

		return false
	}

	if len(d.out)+len(word) > end {
		d.r.setErr(ErrInvalid)

		return false
	}

	d.out = append(d.out, word...)

	return true
}

// brotliDictionaryWord expands one static dictionary entry, which is a word of
// the requested length put through one of the transforms.
func brotliDictionaryWord(index uint32, length int) ([]byte, bool) {
	if length < 4 || length > brotliMaxWordLength {
		return nil, false
	}

	bits := brotliDictBits[length]
	if bits == 0 {
		return nil, false
	}

	word := index & (1<<bits - 1)
	transform := index >> bits

	if int(transform) >= len(brotliTransforms) {
		return nil, false
	}

	start := brotliDictOffset[length] + word*uint32(length)
	if int(start)+length > len(brotliDictionary) {
		return nil, false
	}

	return brotliTransform(brotliDictionary[start:int(start)+length], transform), true
}

func brotliTransform(word string, index uint32) []byte {
	t := brotliTransforms[index]

	prefix := brotliPrefixSuffixAt(t[0])
	suffix := brotliPrefixSuffixAt(t[2])
	kind := t[1]

	out := make([]byte, 0, brotliMaxTransformed)
	out = append(out, prefix...)

	switch {
	case kind <= 9:
		if int(kind) >= len(word) {
			word = ""
		} else {
			word = word[:len(word)-int(kind)]
		}
	case kind >= 12 && kind <= 20:
		if skip := int(kind) - 11; skip >= len(word) {
			word = ""
		} else {
			word = word[skip:]
		}
	}

	body := len(out)
	out = append(out, word...)

	switch kind {
	case 10:
		brotliUpperCase(out[body:], false)
	case 11:
		brotliUpperCase(out[body:], true)
	}

	return append(out, suffix...)
}

// brotliUpperCase applies brotli's own simplified uppercasing, which is not
// Unicode's, to the first character or to all of them.
func brotliUpperCase(b []byte, all bool) {
	for i := 0; i < len(b); {
		n := 1

		switch {
		case b[i] < 0xc0:
			if b[i] >= 'a' && b[i] <= 'z' {
				b[i] ^= 32
			}
		case b[i] < 0xe0:
			if i+1 < len(b) {
				b[i+1] ^= 32
			}

			n = 2
		default:
			if i+2 < len(b) {
				b[i+2] ^= 5
			}

			n = 3
		}

		if !all {
			return
		}

		i += n
	}
}

func brotliPrefixSuffixAt(id uint8) string {
	if int(id) >= len(brotliPrefixSuffixMap) {
		return ""
	}

	off := int(brotliPrefixSuffixMap[id])
	if off >= len(brotliPrefixSuffix) {
		return ""
	}

	n := int(brotliPrefixSuffix[off])
	if off+1+n > len(brotliPrefixSuffix) {
		return ""
	}

	return brotliPrefixSuffix[off+1 : off+1+n]
}

// brotliContext derives the literal context from the last two bytes under the
// mode the block type selected.
func brotliContext(out []byte, mode uint8) uint8 {
	var p1, p2 byte

	if n := len(out); n > 0 {
		p1 = out[n-1]

		if n > 1 {
			p2 = out[n-2]
		}
	}

	base := int(mode) * 512

	return brotliContextLookup[base+int(p1)] | brotliContextLookup[base+256+int(p2)]
}
