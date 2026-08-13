package jxl

import "math"

const (
	numQuantTables   = 17
	maxDistanceBands = 1 + (1 << 4)
	almostZero       = 1e-8
	globalScaleDenom = 1 << 16
)

var (
	quantSizeX = [numQuantTables]int{1, 1, 1, 1, 2, 4, 1, 1, 2, 1, 1, 8, 4, 16, 8, 32, 16}
	quantSizeY = [numQuantTables]int{1, 1, 1, 1, 2, 4, 2, 4, 4, 1, 1, 8, 8, 16, 16, 32, 32}

	quantTableOf = [numHFTransforms]int{
		0, 1, 2, 3, 4, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 10, 10, 11, 12, 12, 13, 14, 14, 15, 16, 16,
	}
)

type quantEncodingKind uint8

const (
	qeLibrary quantEncodingKind = iota
	qeIdentity
	qeDCT2
	qeDCT4
	qeDCT4X8
	qeAFV
	qeDCT
	qeRaw
)

type dctQuantParams struct {
	bands    [3][maxDistanceBands]float32
	numBands int
}

func (p *dctQuantParams) read(r *bitReader) {
	p.numBands = int(r.bits(4)) + 1

	for c := range 3 {
		for i := range p.numBands {
			p.bands[c][i] = r.f16()
		}

		if p.bands[c][0] < almostZero {
			r.setErr(ErrInvalid)

			return
		}

		p.bands[c][0] *= 64
	}
}

type quantEncoding struct {
	kind       quantEncodingKind
	identity   [3][3]float32
	dct2       [3][6]float32
	dct4Mul    [3][2]float32
	dct4x8Mul  [3]float32
	afvWeights [3][9]float32
	params     dctQuantParams
	params4x4  dctQuantParams
	qtable     []int32
	qtableDen  float32
}

func (e *quantEncoding) read(r *bitReader, f *frame, index int, globalTree *maTree) {
	*e = quantEncoding{}

	sizeX, sizeY := quantSizeX[index], quantSizeY[index]
	single := sizeX*sizeY == 1

	mode := r.bits(3)

	readF16Row := func(dst []float32, scale float32) {
		for i := range dst {
			dst[i] = r.f16()

			if absF(dst[i]) < almostZero {
				r.setErr(ErrInvalid)

				return
			}

			dst[i] *= scale
		}
	}

	switch mode {
	case 0:
		e.kind = qeLibrary
	case 1:
		if !single {
			r.setErr(ErrInvalid)

			return
		}

		e.kind = qeIdentity

		for c := range 3 {
			readF16Row(e.identity[c][:], 64)
		}
	case 2:
		if !single {
			r.setErr(ErrInvalid)

			return
		}

		e.kind = qeDCT2

		for c := range 3 {
			readF16Row(e.dct2[c][:], 64)
		}
	case 3:
		if !single {
			r.setErr(ErrInvalid)

			return
		}

		e.kind = qeDCT4

		for c := range 3 {
			readF16Row(e.dct4Mul[c][:], 1)
		}

		e.params.read(r)
	case 4:
		if !single {
			r.setErr(ErrInvalid)

			return
		}

		e.kind = qeDCT4X8

		readF16Row(e.dct4x8Mul[:], 1)
		e.params.read(r)
	case 5:
		if !single {
			r.setErr(ErrInvalid)

			return
		}

		e.kind = qeAFV

		for c := range 3 {
			for i := range e.afvWeights[c] {
				e.afvWeights[c][i] = r.f16()
			}

			for i := range 6 {
				e.afvWeights[c][i] *= 64
			}
		}

		e.params.read(r)
		e.params4x4.read(r)
	case 6:
		e.kind = qeDCT

		e.params.read(r)
	case 7:
		e.kind = qeRaw

		e.qtableDen = r.f16()
		if e.qtableDen < almostZero {
			r.setErr(ErrInvalid)

			return
		}

		e.qtable = f.readQuantTable(r, index, sizeX*blockDim, sizeY*blockDim, globalTree)
	default:
		r.setErr(ErrInvalid)
	}
}

func quantMult(v float32) float32 {
	if v > 0 {
		return 1 + v
	}

	return 1 / (1 - v)
}

func interpolateVec(pos float32, bands []float32) float32 {
	idx := int(math.Floor(float64(pos)))
	frac := pos - float32(idx)
	a, b := bands[idx], bands[idx+1]

	return float32(math.Pow(float64(b/a), float64(frac))) * a
}

func interpolate(pos, maxPos float32, bands []float32) float32 {
	scaled := pos * float32(len(bands)-1) / maxPos
	idx := int(scaled)
	a, b := bands[idx], bands[idx+1]

	return a * float32(math.Pow(float64(b/a), float64(scaled-float32(idx))))
}

func quantWeights(rows, cols int, p *dctQuantParams, out []float32) error {
	for c := range 3 {
		var bands [maxDistanceBands]float32

		bands[0] = p.bands[c][0]
		if bands[0] < almostZero {
			return ErrInvalid
		}

		for i := 1; i < p.numBands; i++ {
			bands[i] = bands[i-1] * quantMult(p.bands[c][i])

			if bands[i] < almostZero {
				return ErrInvalid
			}
		}

		scale := float32(p.numBands-1) / (float32(math.Sqrt2) + 1e-6)
		rcpcol := scale / float32(cols-1)
		rcprow := scale / float32(rows-1)

		for y := range rows {
			dy := float32(y) * rcprow
			dy2 := dy * dy

			for x := range cols {
				dx := float32(x) * rcpcol
				dist := float32(math.Sqrt(float64(dx*dx + dy2)))

				w := bands[0]
				if p.numBands > 1 {
					w = interpolateVec(dist, bands[:p.numBands])
				}

				out[c*cols*rows+y*cols+x] = w
			}
		}
	}

	return nil
}

var afvFreqs = [16]float32{
	0xBAD, 0xBAD, 0.8517778890324296, 5.37778436506804,
	0xBAD, 0xBAD, 4.734747904497923, 5.449245381693219,
	1.6598270267479331, 4, 7.275749096817861, 10.423227632456525,
	2.662932286148962, 7.630657783650829, 8.962388608184032, 12.97166202570235,
}

func computeQuantTable(e *quantEncoding, index int) ([]float32, error) {
	wrows := blockDim * quantSizeX[index]
	wcols := blockDim * quantSizeY[index]
	num := wrows * wcols

	weights := make([]float32, 3*num)

	switch e.kind {
	case qeIdentity:
		for c := range 3 {
			for i := range blockSize {
				weights[blockSize*c+i] = e.identity[c][0]
			}

			weights[blockSize*c+1] = e.identity[c][1]
			weights[blockSize*c+blockDim] = e.identity[c][1]
			weights[blockSize*c+blockDim+1] = e.identity[c][2]
		}
	case qeDCT2:
		for c := range 3 {
			w := &e.dct2[c]
			s := c * blockSize

			weights[s] = 0xBAD
			weights[s+1] = w[0]
			weights[s+blockDim] = w[0]
			weights[s+blockDim+1] = w[1]

			for y := range 2 {
				for x := range 2 {
					weights[s+y*blockDim+x+2] = w[2]
					weights[s+(y+2)*blockDim+x] = w[2]
					weights[s+(y+2)*blockDim+x+2] = w[3]
				}
			}

			for y := range 4 {
				for x := range 4 {
					weights[s+y*blockDim+x+4] = w[4]
					weights[s+(y+4)*blockDim+x] = w[4]
					weights[s+(y+4)*blockDim+x+4] = w[5]
				}
			}
		}
	case qeDCT4:
		var w4x4 [3 * 16]float32

		if err := quantWeights(4, 4, &e.params, w4x4[:]); err != nil {
			return nil, err
		}

		for c := range 3 {
			for y := range blockDim {
				for x := range blockDim {
					weights[c*num+y*blockDim+x] = w4x4[c*16+(y/2)*4+x/2]
				}
			}

			weights[c*num+1] /= e.dct4Mul[c][0]
			weights[c*num+blockDim] /= e.dct4Mul[c][0]
			weights[c*num+blockDim+1] /= e.dct4Mul[c][1]
		}
	case qeDCT4X8:
		var w4x8 [3 * 32]float32

		if err := quantWeights(4, blockDim, &e.params, w4x8[:]); err != nil {
			return nil, err
		}

		for c := range 3 {
			for y := range blockDim {
				for x := range blockDim {
					weights[c*num+y*blockDim+x] = w4x8[c*32+(y/2)*blockDim+x]
				}
			}

			weights[c*num+blockDim] /= e.dct4x8Mul[c]
		}
	case qeDCT:
		if err := quantWeights(wrows, wcols, &e.params, weights); err != nil {
			return nil, err
		}
	case qeRaw:
		if len(e.qtable) != 3*num {
			return nil, ErrInvalid
		}

		for i := range weights {
			weights[i] = 1 / (e.qtableDen * float32(e.qtable[i]))
		}

	case qeAFV:
		var w4x8 [3 * 32]float32

		if err := quantWeights(4, blockDim, &e.params, w4x8[:]); err != nil {
			return nil, err
		}

		var w4x4 [3 * 16]float32

		if err := quantWeights(4, 4, &e.params4x4, w4x4[:]); err != nil {
			return nil, err
		}

		const (
			afvLo = 0.8517778890324296
			afvHi = 12.97166202570235 - afvLo + 1e-6
		)

		for c := range 3 {
			var bands [4]float32

			bands[0] = e.afvWeights[c][5]
			if bands[0] < almostZero {
				return nil, ErrInvalid
			}

			for i := 1; i < 4; i++ {
				bands[i] = bands[i-1] * quantMult(e.afvWeights[c][i+5])

				if bands[i] < almostZero {
					return nil, ErrInvalid
				}
			}

			s := c * blockSize

			weights[s] = 1
			weights[s+blockDim] = e.afvWeights[c][0]
			weights[s+1] = e.afvWeights[c][1]
			weights[s+2*blockDim] = e.afvWeights[c][2]
			weights[s+2] = e.afvWeights[c][3]
			weights[s+2*blockDim+2] = e.afvWeights[c][4]

			for y := range 4 {
				for x := range 4 {
					if x < 2 && y < 2 {
						continue
					}

					weights[s+2*y*blockDim+2*x] = interpolate(afvFreqs[y*4+x]-afvLo, afvHi, bands[:])
				}
			}

			for y := range blockDim / 2 {
				for x := range blockDim {
					if x == 0 && y == 0 {
						continue
					}

					weights[c*num+(2*y+1)*blockDim+x] = w4x8[c*32+y*blockDim+x]
				}
			}

			for y := range blockDim / 2 {
				for x := range blockDim / 2 {
					if x == 0 && y == 0 {
						continue
					}

					weights[c*num+2*y*blockDim+2*x+1] = w4x4[c*16+y*4+x]
				}
			}
		}
	default:
		return nil, ErrInvalid
	}

	for i, w := range weights {
		if w < almostZero || w > 1/almostZero {
			return nil, ErrInvalid
		}

		weights[i] = 1 / w
	}

	return weights, nil
}

type dequantMatrices struct {
	tables [numQuantTables][]float32
}

var libraryTables [numQuantTables][]float32

func init() {
	for i := range numQuantTables {
		t, err := computeQuantTable(&quantLibrary[i], i)
		if err != nil {
			panic(err)
		}

		libraryTables[i] = t
	}
}

func (d *dequantMatrices) matrix(t hfTransform, c int) []float32 {
	idx := quantTableOf[t]
	num := quantSizeX[idx] * quantSizeY[idx] * blockSize

	return d.tables[idx][c*num:]
}

func (d *dequantMatrices) read(r *bitReader, f *frame, globalTree *maTree) {
	if r.bit() {
		d.tables = libraryTables

		return
	}

	for i := range numQuantTables {
		var e quantEncoding

		e.read(r, f, i, globalTree)

		if err := r.err(); err != nil {
			return
		}

		if e.kind == qeLibrary {
			d.tables[i] = libraryTables[i]

			continue
		}

		t, err := computeQuantTable(&e, i)
		if err != nil {
			r.setErr(err)

			return
		}

		d.tables[i] = t
	}
}

func (f *frame) readQuantTable(r *bitReader, index, w, h int, globalTree *maTree) []int32 {
	var img modImage

	img.bitDepth = 8

	for range 3 {
		img.channels = append(img.channels, newModChannel(w, h, 0, 0))
	}

	img.decodeStream(r, nil, globalTree, 1+3*f.hdr.numLFGroups()+index)

	if r.err() != nil {
		return nil
	}

	out := make([]int32, 3*w*h)
	i := 0

	for _, c := range img.channels {
		for y := range h {
			for _, v := range c.row(y) {
				if v <= 0 {
					r.setErr(ErrInvalid)

					return nil
				}

				out[i] = v
				i++
			}
		}
	}

	return out
}
