package jxl

import (
	"image"
	"io"
	"math"
)

func encodeFilter() restorationFilter {
	f := defaultRestorationFilter()
	f.gab = false
	f.epfIters = 0

	return f
}

func newEncodeFrame(m *imageMetadata, t *customTransformData, w, h int) (*frame, error) {
	f := &frame{}

	f.hdr.width, f.hdr.height = uint32(w), uint32(h)
	f.hdr.encoding = encVarDCT
	f.hdr.upsampling = 1
	f.hdr.groupSizeShift = 1
	f.hdr.pass.numPasses = 1
	f.hdr.xQMScale, f.hdr.bQMScale = 3, 2
	f.hdr.isLast = true
	f.hdr.flags = flagSkipAdaptiveLFSmoothing
	f.hdr.filter = encodeFilter()
	f.hdr.ecUpsampling = make([]uint32, len(m.extraChannels))

	for i := range f.hdr.ecUpsampling {
		f.hdr.ecUpsampling[i] = 1
	}

	f.lfQuant.factors = lfQuantDefault
	f.vardct = newVarDCT(f, t.opsin.quant, nil)

	v := f.vardct

	v.dq.tables = libraryTables
	v.bctx = blockContextMap{contextMap: defaultBlockContextMap, numLFContexts: 1, numContexts: 15}
	v.cmap = defaultColorCorrelation()
	v.numHistograms = 1
	v.xdm = powf(1/1.25, float32(f.hdr.xQMScale)-2)
	v.bdm = powf(1/1.25, float32(f.hdr.bQMScale)-2)

	v.passes = make([]varDCTPass, 1)

	for o := range numOrders {
		for c := range 3 {
			v.passes[0].orders[3*o+c] = naturalOrders[o]
		}
	}

	return f, nil
}

func (f *frame) fillXYB(fwd *xybForward, src rowSource, w, h int) {
	v := f.vardct
	stride := v.bw * blockDim
	n := v.bh * blockDim * stride

	var row [3][]float32

	for c := range 3 {
		row[c] = make([]float32, w)
	}

	for y := range v.bh * blockDim {
		o := y * stride

		if y >= h {
			for c := range 3 {
				copy(v.pixels[c][o:o+stride], v.pixels[c][o-stride:o])
			}

			continue
		}

		src(y, &row)

		for x := range w {
			r, g, b := row[0][x], row[1][x], row[2][x]

			v.pixels[0][o+x] = (fwd.inv[0]*r + fwd.inv[1]*g + fwd.inv[2]*b - fwd.scaledBias[0]) /
				fwd.intensityScale
			v.pixels[1][o+x] = (fwd.inv[3]*r + fwd.inv[4]*g + fwd.inv[5]*b - fwd.scaledBias[1]) /
				fwd.intensityScale
			v.pixels[2][o+x] = (fwd.inv[6]*r + fwd.inv[7]*g + fwd.inv[8]*b - fwd.scaledBias[2]) /
				fwd.intensityScale
		}

		for c := range 3 {
			edge := v.pixels[c][o+w-1]

			for x := w; x < stride; x++ {
				v.pixels[c][o+x] = edge
			}
		}
	}

	for c := range 3 {
		cbrtPlane(v.pixels[c][:n])
	}

	for i := range n {
		l := v.pixels[0][i] + fwd.biasCbrt[0]
		m := v.pixels[1][i] + fwd.biasCbrt[1]

		v.pixels[0][i] = (l - m) / 2
		v.pixels[1][i] = (l + m) / 2
		v.pixels[2][i] += fwd.biasCbrt[2]
	}
}

func (f *frame) blockToScratch(s *dctScratch, t hfTransform, bx, by int, lf *[3][]float32) bool {
	v := f.vardct
	cx, cy := t.blocks()
	stride := v.bw * blockDim
	bw, bh := cx*blockDim, cy*blockDim

	for c := range 3 {
		buf := s.block[c][:cx*cy*blockSize]

		for y := range bh {
			copy(buf[y*bw:], v.pixels[c][(by*blockDim+y)*stride+bx*blockDim:][:bw])
		}

		if !transformFromPixels(t, buf, lf[c], s.scratch) {
			return false
		}
	}

	return true
}

func (f *frame) blockCost(s *dctScratch, t hfTransform, bx, by int, lf *[3][]float32) (float64, float64) {
	v := f.vardct
	cx, cy := t.blocks()
	numCoeffs := cx * cy * blockSize
	bw, bh := cx*blockDim, cy*blockDim
	stride := v.bw * blockDim

	if !f.blockToScratch(s, t, bx, by, lf) {
		return math.Inf(1), 0
	}

	v.quantizeBlock(s, t, v.quant.invGlobalScale(), uint32(v.rawQuant[by*v.bw+bx]),
		v.cmap.yToX(0), v.cmap.yToB(0), numCoeffs, 0)

	order := naturalOrders[hfShapeID[t]]
	rate := 0.0

	for c := range 3 {
		for k := cx * cy; k < numCoeffs; k++ {
			if q := s.coeffs[c][order[k]]; q != 0 {
				rate += 1 + log2Int(uint64(absInt32(q))+1)
			}
		}

		for k := range cx * cy {
			s.coeffs[c][order[k]] = 0
		}
	}

	v.dequantBlock(s, t, v.quant.invGlobalScale(), uint32(v.rawQuant[by*v.bw+bx]),
		v.cmap.yToX(0), v.cmap.yToB(0), numCoeffs, 0)

	sse := 0.0

	for c := range 3 {
		transformToPixels(t, lf[c], s.block[c][:numCoeffs], s.scratch)

		for y := range bh {
			row := v.pixels[c][(by*blockDim+y)*stride+bx*blockDim:][:bw]

			for x, want := range row {
				d := float64(s.block[c][y*bw+x] - want)
				sse += d * d
			}
		}
	}

	return rate, sse
}

const (
	transformRateMargin   = 0.7
	adaptiveQuantStrength = 0.25
)

var transformCandidates = [...]hfTransform{
	hfDCT32X32, hfDCT32X16, hfDCT16X32, hfDCT32X8, hfDCT8X32, hfDCT16X16, hfDCT16X8, hfDCT8X16,
}

func (f *frame) regionFree(bx, by, cx, cy int) bool {
	v := f.vardct

	if bx+cx > v.bw || by+cy > v.bh {
		return false
	}

	if bx+cx > (bx/32+1)*32 || by+cy > (by/32+1)*32 {
		return false
	}

	for iy := range cy {
		for ix := range cx {
			if v.transform[(by+iy)*v.bw+bx+ix] != invalidTransform {
				return false
			}
		}
	}

	return true
}

func newLFBuffers() [3][]float32 {
	var lf [3][]float32

	for c := range 3 {
		lf[c] = make([]float32, maxCoeffBlocks*maxCoeffBlocks)
	}

	return lf
}

func (f *frame) chooseTransforms(scratch []dctScratch) {
	v := f.vardct

	for i := range v.transform {
		v.transform[i] = invalidTransform
	}

	rate8 := make([]float64, v.bw*v.bh)
	sse8 := make([]float64, v.bw*v.bh)

	nw, numGroups := len(scratch), f.hdr.numGroups()

	overTasks(nw, numGroups, func(w, g int) error {
		s, rc, lf := &scratch[w], f.blockGroupRect(g), newLFBuffers()

		for by := range rc.h {
			for bx := range rc.w {
				pos := (rc.y0+by)*v.bw + rc.x0 + bx

				rate8[pos], sse8[pos] = f.blockCost(s, hfDCT, rc.x0+bx, rc.y0+by, &lf)
			}
		}

		return nil
	})

	overTasks(nw, numGroups, func(w, g int) error {
		s, rc, lf := &scratch[w], f.blockGroupRect(g), newLFBuffers()

		for by := range rc.h {
			for bx := range rc.w {
				x, y := rc.x0+bx, rc.y0+by

				if v.transform[y*v.bw+x] != invalidTransform {
					continue
				}

				best, saving := hfDCT, 0.0

				for _, t := range transformCandidates {
					cx, cy := t.blocks()

					if !f.regionFree(x, y, cx, cy) {
						continue
					}

					baseRate, baseSSE := 0.0, 0.0

					for iy := range cy {
						for ix := range cx {
							baseRate += rate8[(y+iy)*v.bw+x+ix]
							baseSSE += sse8[(y+iy)*v.bw+x+ix]
						}
					}

					rate, sse := f.blockCost(s, t, x, y, &lf)

					if rate < baseRate*transformRateMargin && sse <= baseSSE && baseRate-rate > saving {
						best, saving = t, baseRate-rate
					}
				}

				cx, cy := best.blocks()

				for iy := range cy {
					for ix := range cx {
						id := uint8(best)

						if ix == 0 && iy == 0 {
							id |= 128
						}

						v.transform[(y+iy)*v.bw+x+ix] = id
					}
				}
			}
		}

		return nil
	})
}

func (f *frame) transformGroup(s *dctScratch, g int) bool {
	v := f.vardct
	rc := f.blockGroupRect(g)
	lf := newLFBuffers()
	offset := 0

	for by := range rc.h {
		for bx := range rc.w {
			x, y := rc.x0+bx, rc.y0+by
			pos := y*v.bw + x

			if v.transform[pos] < 128 {
				continue
			}

			t := hfTransform(v.transform[pos] & 127)
			cx, cy := t.blocks()
			numCoeffs := cx * cy * blockSize

			if !f.blockToScratch(s, t, x, y, &lf) {
				return false
			}

			for c := range 3 {
				for iy := range cy {
					for ix := range cx {
						v.lf[c][(y+iy)*v.bw+x+ix] = lf[c][iy*cx+ix]
					}
				}
			}

			v.quantizeBlock(s, t, v.quant.invGlobalScale(), uint32(v.rawQuant[pos]),
				v.cmap.yToX(0), v.cmap.yToB(0), numCoeffs, offset)

			order := naturalOrders[hfShapeID[t]]

			for c := range 3 {
				for k := range cx * cy {
					s.coeffs[c][offset+int(order[k])] = 0
				}
			}

			offset += numCoeffs
		}
	}

	return true
}

func codedBits(h *histograms, counts [][]uint32, codes *symbolWriter) int {
	total := 0

	for c, cnt := range counts {
		for t, n := range cnt {
			if n == 0 {
				continue
			}

			bits := logSumProbs
			if codes.codes != nil {
				bits = int(codes.codes[c][t].bits)
			} else if f := codes.tables[c].info[t].freq; f != 0 {
				bits = logSumProbs - int(log2Int(uint64(f)))
			}

			total += int(n) * (bits + h.uintConfigs[c].extraBits(uint32(t)))
		}
	}

	return total
}

func reserveGroups(sections []*bitWriter, perGroup []int, bits int) {
	values := 0

	for _, n := range perGroup {
		values += n
	}

	if values == 0 {
		return
	}

	for g, n := range perGroup {
		sections[g].reserve(bits/8*n/values + 64)
	}
}

func (f *frame) encodeSections(scratch []dctScratch) ([][]byte, error) {
	numLF := f.hdr.numLFGroups()
	numGroups := f.hdr.numGroups()

	nw := len(scratch)
	perWorker := make([][]valueCounts, nw)

	for i := range perWorker {
		perWorker[i] = f.acValues()
	}

	perGroup := make([]int, numGroups)

	err := overTasks(nw, numGroups, func(w, g int) error {
		if !f.transformGroup(&scratch[w], g) {
			return ErrUnsupported
		}

		n, err := f.collectACGroup(&scratch[w], g, perWorker[w])
		perGroup[g] = n

		return err
	})
	if err != nil {
		return nil, err
	}

	values := perWorker[0]

	for _, other := range perWorker[1:] {
		for i := range other {
			values[i].merge(&other[i])
		}
	}

	hist, counts := f.finishACHistograms(values)

	sections := make([]*bitWriter, 2+numLF+numGroups)

	for i := range sections {
		sections[i] = &bitWriter{}
	}

	hdr := groupHeader{wp: defaultWeightedHeader()}
	lfTree := channelTree(3, predGradient)
	metaTree := channelTree(4, predGradient)

	f.encodeLFGlobal(sections[0])

	for g := range numLF {
		f.encodeVarDCTLF(sections[1+g], g, &hdr, lfTree)
		f.encodeHFMetadata(sections[1+g], g, &hdr, metaTree)
	}

	codes := f.encodeHFGlobal(sections[1+numLF], hist, counts)

	if err := sections[1+numLF].err(); err != nil {
		return nil, err
	}

	hist.resize(f.acContexts() + zeroDensityContextLimit - zeroDensityContextCount)

	reserveGroups(sections[2+numLF:], perGroup, codedBits(hist, counts, codes))

	writers := make([]symbolWriter, nw)

	err = overTasks(nw, numGroups, func(w, g int) error {
		if !f.transformGroup(&scratch[w], g) {
			return ErrUnsupported
		}

		return f.encodeVarDCTGroup(sections[2+numLF+g], g, &scratch[w], hist,
			codes, &writers[w], 0)
	})
	if err != nil {
		return nil, err
	}

	for _, sec := range sections {
		if err := sec.err(); err != nil {
			return nil, err
		}
	}

	if f.hdr.numTOCEntries() == 1 {
		var w bitWriter

		for _, sec := range sections {
			w.append(sec)
		}

		return [][]byte{w.bytes()}, w.err()
	}

	out := make([][]byte, len(sections))

	for i, sec := range sections {
		out[i] = sec.bytes()
	}

	return out, nil
}

type encodeParams struct {
	distance float32
	threads  int
	strength float32
}

func encodeVarDCTImage(src rowSource, w, h int, p encodeParams) ([]byte, error) {
	if w <= 0 || h <= 0 {
		return nil, ErrInvalid
	}

	var head fileHeader

	head.size = sizeHeader{xsize: uint32(w), ysize: uint32(h)}
	head.meta = defaultImageMetadata()
	head.transform = defaultCustomTransformData()

	f, err := newEncodeFrame(&head.meta, &head.transform, w, h)
	if err != nil {
		return nil, err
	}

	params := newXYBParams(&head.transform.opsin, head.meta.tone.intensityTarget)

	fwd, ok := newXYBForward(&params)
	if !ok {
		return nil, ErrInvalid
	}

	f.fillXYB(&fwd, src, w, h)
	f.setQuantField(p.distance, p.strength)

	scratch := make([]dctScratch, min(numThreads(p.threads), f.hdr.numGroups()))

	for i := range scratch {
		scratch[i].alloc(f)
	}

	f.chooseTransforms(scratch)
	f.adjustQuantField()

	body, err := f.encodeSections(scratch)
	if err != nil {
		return nil, err
	}

	var out bitWriter

	head.write(&out)
	f.encode(&out, &head.meta, uint32(w), uint32(h), body)

	if err := out.err(); err != nil {
		return nil, err
	}

	return out.bytes(), nil
}

// EncodeOptions controls the encoder. The zero value is a reasonable default.
type EncodeOptions struct {
	// Quality runs from 1 to 100. Zero means DefaultQuality.
	Quality int
	// Threads bounds the goroutines encoding one frame. Zero means GOMAXPROCS,
	// one encodes serially.
	Threads int
}

// DefaultQuality is the quality used when EncodeOptions.Quality is zero.
const DefaultQuality = 90

func encodeOptions(opts []EncodeOptions) EncodeOptions {
	var o EncodeOptions

	if len(opts) > 0 {
		o = opts[0]
	}

	if o.Quality == 0 {
		o.Quality = DefaultQuality
	}

	return o
}

func qualityToDistance(q int) float32 {
	f := float32(min(max(q, 1), 100))

	if f >= 30 {
		return 0.1 + (100-f)*0.09
	}

	return 53.0/3000*f*f - 23.0/20*f + 25
}

var srgb8Linear = func() (t [256]float32) {
	for i := range t {
		t[i] = srgbToLinear(float32(i*257) / 65535)
	}

	return t
}()

func linearFrom16(v uint32) float32 {
	if v>>8 == v&0xff {
		return srgb8Linear[v>>8]
	}

	return srgbToLinear(float32(v) / 65535)
}

type rowSource func(y int, rgb *[3][]float32)

func linearRows(at func(x, y int) (uint32, uint32, uint32)) rowSource {
	return func(y int, rgb *[3][]float32) {
		for x := range rgb[0] {
			r, g, b := at(x, y)

			rgb[0][x] = linearFrom16(r)
			rgb[1][x] = linearFrom16(g)
			rgb[2][x] = linearFrom16(b)
		}
	}
}

func imageRows(m image.Image) rowSource {
	b := m.Bounds()

	switch src := m.(type) {
	case *image.NRGBA:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := src.NRGBAAt(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	case *image.RGBA:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := src.RGBAAt(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	case *image.NRGBA64:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := src.NRGBA64At(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	case *image.RGBA64:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := src.RGBA64At(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	case *image.Gray:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := src.GrayAt(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	case *image.YCbCr:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := src.YCbCrAt(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	default:
		return linearRows(func(x, y int) (uint32, uint32, uint32) {
			r, g, bl, _ := m.At(b.Min.X+x, b.Min.Y+y).RGBA()

			return r, g, bl
		})
	}
}

// Encode writes m as a JPEG XL codestream. Colour is treated as sRGB.
func Encode(w io.Writer, m image.Image, opts ...EncodeOptions) error {
	o := encodeOptions(opts)

	b := m.Bounds()
	width, height := b.Dx(), b.Dy()

	if width <= 0 || height <= 0 {
		return ErrInvalid
	}

	data, err := encodeVarDCTImage(imageRows(m), width, height, encodeParams{
		distance: qualityToDistance(o.Quality),
		threads:  o.Threads,
		strength: adaptiveQuantStrength,
	})
	if err != nil {
		return err
	}

	_, err = w.Write(data)

	return err
}
