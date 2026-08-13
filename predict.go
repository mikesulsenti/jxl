package jxl

import "math/bits"

const (
	predZero = iota
	predWest
	predNorth
	predAvgWestNorth
	predSelect
	predGradient
	predWeighted
	predNorthEast
	predNorthWest
	predWestWest
	predAvgWestNorthWest
	predAvgNorthNorthWest
	predAvgNorthNorthEast
	predAvgAll
	numPredictors
)

type predData struct {
	left          int32
	top           int32
	toptop        int32
	topleft       int32
	topright      int32
	leftleft      int32
	toprightright int32
}

func clampedGradient(left, top, topleft int64) int64 {
	lo, hi := min(left, top), max(left, top)
	grad := left + top - topleft

	if topleft < lo {
		grad = hi
	}
	if topleft > hi {
		return lo
	}

	return grad
}

func predSelectOne(left, top, topleft int64) int64 {
	p := left + top - topleft

	if absInt64(p-left) < absInt64(p-top) {
		return left
	}

	return top
}

func absInt64(v int64) int64 {
	m := v >> 63

	return (v ^ m) - m
}

func predictOne(p uint8, d predData, wp int64) int64 {
	switch p {
	case predZero:
		return 0
	case predWest:
		return int64(d.left)
	case predNorth:
		return int64(d.top)
	case predSelect:
		return predSelectOne(int64(d.left), int64(d.top), int64(d.topleft))
	case predGradient:
		return clampedGradient(int64(d.left), int64(d.top), int64(d.topleft))
	case predWeighted:
		return wp
	case predWestWest:
		return int64(d.leftleft)
	case predNorthEast:
		return int64(d.topright)
	case predNorthWest:
		return int64(d.topleft)
	case predAvgWestNorth:
		return (int64(d.top) + int64(d.left)) / 2
	case predAvgWestNorthWest:
		return (int64(d.left) + int64(d.topleft)) / 2
	case predAvgNorthNorthWest:
		return (int64(d.top) + int64(d.topleft)) / 2
	case predAvgNorthNorthEast:
		return (int64(d.top) + int64(d.topright)) / 2
	default:
		return (6*int64(d.top) - 2*int64(d.toptop) + 7*int64(d.left) +
			int64(d.leftleft) + int64(d.toprightright) + 3*int64(d.topright) + 8) / 16
	}
}

const (
	wpNumPredictors = 4
	predExtraBits   = 3
	predictionRound = (1 << predExtraBits >> 1) - 1
)

var divLookup = [64]uint32{
	16777216, 8388608, 5592405, 4194304, 3355443, 2796202, 2396745, 2097152,
	1864135, 1677721, 1525201, 1398101, 1290555, 1198372, 1118481, 1048576,
	986895, 932067, 883011, 838860, 798915, 762600, 729444, 699050,
	671088, 645277, 621378, 599186, 578524, 559240, 541200, 524288,
	508400, 493447, 479349, 466033, 453438, 441505, 430185, 419430,
	409200, 399457, 390167, 381300, 372827, 364722, 356962, 349525,
	342392, 335544, 328965, 322638, 316551, 310689, 305040, 299593,
	294337, 289262, 284359, 279620, 275036, 270600, 266305, 262144,
}

type weightedHeader struct {
	p1c, p2c                     uint32
	p3ca, p3cb, p3cc, p3cd, p3ce uint32
	w0, w1, w2, w3               uint32
}

func defaultWeightedHeader() weightedHeader {
	return weightedHeader{
		p1c: 16, p2c: 10, p3ca: 7, p3cb: 7, p3cc: 7, p3cd: 0, p3ce: 0,
		w0: 0xd, w1: 0xc, w2: 0xc, w3: 0xc,
	}
}

func (h *weightedHeader) read(r *bitReader) {
	*h = defaultWeightedHeader()

	if r.bit() {
		return
	}

	h.p1c = uint32(r.bits(5))
	h.p2c = uint32(r.bits(5))
	h.p3ca = uint32(r.bits(5))
	h.p3cb = uint32(r.bits(5))
	h.p3cc = uint32(r.bits(5))
	h.p3cd = uint32(r.bits(5))
	h.p3ce = uint32(r.bits(5))
	h.w0 = uint32(r.bits(4))
	h.w1 = uint32(r.bits(4))
	h.w2 = uint32(r.bits(4))
	h.w3 = uint32(r.bits(4))
}

type wpState struct {
	prediction [wpNumPredictors]int64
	pred       int64
	xsize      int
	predErrors [][wpNumPredictors]uint32
	errors     []int32
	w          [4]uint32
	p1c, p2c   int64
	p3c        [5]int64
}

func (s *wpState) reset(h *weightedHeader, xsize int) {
	n := (xsize + 1) * 2

	if cap(s.predErrors) < n {
		s.predErrors = make([][wpNumPredictors]uint32, n)
		s.errors = make([]int32, n)
	}

	s.predErrors = s.predErrors[:n]
	s.errors = s.errors[:n]

	clear(s.predErrors)
	clear(s.errors)

	s.xsize = xsize
	s.w = [4]uint32{h.w0, h.w1, h.w2, h.w3}
	s.p1c = int64(h.p1c)
	s.p2c = int64(h.p2c)
	s.p3c = [5]int64{int64(h.p3ca), int64(h.p3cb), int64(h.p3cc), int64(h.p3cd), int64(h.p3ce)}
}

func errorWeight(err, w uint32) uint32 {
	shift := uint32(bits.Len64(uint64(err)+1) - 1)
	if shift > 5 {
		shift -= 5
	} else {
		shift = 0
	}

	return 4 + ((w * divLookup[err>>shift]) >> shift)
}

func (s *wpState) predictAndProperty(x, y int, d *predData) (int64, int32) {
	curRow, prevRow := s.xsize+1, 0
	if y&1 != 0 {
		curRow, prevRow = 0, s.xsize+1
	}

	posNE := x
	if x+1 < s.xsize {
		posNE = x + 1
	}

	posNW := max(x-1, 0)

	errN := &s.predErrors[prevRow+x]
	errNE := &s.predErrors[prevRow+posNE]
	errNW := &s.predErrors[prevRow+posNW]

	var wt [4]uint32
	for i := range wt {
		e := errN[i] + errNE[i] + errNW[i]
		wt[i] = errorWeight(e, s.w[i])
	}

	teW := int64(s.errors[curRow+x])
	teN := int64(s.errors[prevRow+1+x])
	teNW := int64(s.errors[prevRow+1+posNW])
	teNE := int64(s.errors[prevRow+1+posNE])
	sumWN := teN + teW

	p := teW
	if absInt64(teN) > absInt64(p) {
		p = teN
	}
	if absInt64(teNW) > absInt64(p) {
		p = teNW
	}
	if absInt64(teNE) > absInt64(p) {
		p = teNE
	}

	n := int64(d.top) << predExtraBits
	w := int64(d.left) << predExtraBits
	ne := int64(d.topright) << predExtraBits
	nw := int64(d.topleft) << predExtraBits
	nn := int64(d.toptop) << predExtraBits

	p0 := w + ne - n
	p1 := n - (((sumWN + teNE) * s.p1c) >> 5)
	p2 := w - (((sumWN + teNW) * s.p2c) >> 5)
	p3 := n - ((teNW*s.p3c[0] + teN*s.p3c[1] + teNE*s.p3c[2] +
		(nn-n)*s.p3c[3] + (nw-w)*s.p3c[4]) >> 5)

	total := uint64(wt[0]) + uint64(wt[1]) + uint64(wt[2]) + uint64(wt[3])
	logWeight := uint32(bits.Len64(total) - 1)

	w0s := int64(wt[0] >> (logWeight - 4))
	w1s := int64(wt[1] >> (logWeight - 4))
	w2s := int64(wt[2] >> (logWeight - 4))
	w3s := int64(wt[3] >> (logWeight - 4))

	weightSum := w0s + w1s + w2s + w3s
	sum := (weightSum >> 1) - 1 + w0s*p0 + w1s*p1 + w2s*p2 + w3s*p3

	pred := (sum * int64(divLookup[weightSum-1])) >> 24

	if ((teN ^ teW) | (teN ^ teNW)) <= 0 {
		mx := max(w, max(ne, n))
		mn := min(w, min(ne, n))
		pred = max(mn, min(mx, pred))
	}

	s.prediction = [4]int64{p0, p1, p2, p3}
	s.pred = pred

	return (pred + predictionRound) >> predExtraBits, int32(p)
}

func (s *wpState) updateErrors(correct int32, x, y int) {
	curRow, prevRow := s.xsize+1, 0
	if y&1 != 0 {
		curRow, prevRow = 0, s.xsize+1
	}

	val := int64(correct) << predExtraBits
	s.errors[curRow+x+1] = int32(s.pred - val)

	var errs [4]uint32
	for i := range errs {
		errs[i] = uint32((absInt64(s.prediction[i]-val) + predictionRound) >> predExtraBits)
	}

	s.predErrors[curRow+x] = errs

	prev := &s.predErrors[prevRow+x+1]
	for i := range errs {
		prev[i] += errs[i]
	}
}
