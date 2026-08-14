package jxl

import (
	"math"
	"math/rand/v2"
	"testing"
)

var testBiases = map[string][4]float32{
	"default": defaultOpsinInverseMatrix().quant,
	"custom":  {0.3, 0.62, 0.94, 0.5},
}

func testVarDCT(biases [4]float32) *varDCT {
	v := &varDCT{biases: biases}

	xqm, bqm := float32(3), float32(2)

	v.dq.tables = libraryTables
	v.xdm = powf(1/1.25, xqm-2)
	v.bdm = powf(1/1.25, bqm-2)

	return v
}

func testScratch() *dctScratch {
	var s dctScratch

	for c := range 3 {
		s.coeffs[c] = make([]int32, maxCoeffArea)
		s.block[c] = make([]float32, maxCoeffArea)
	}

	s.scratch = make([]float32, maxCoeffArea+2*maxBlockDim)

	return &s
}

func TestQuantizeRecoversIntegers(t *testing.T) {
	rng := rand.New(rand.NewPCG(23, 29))

	s := testScratch()

	for name, biases := range testBiases {
		v := testVarDCT(biases)

		for _, quant := range []uint32{1, 3, 17, 255} {
			for _, cc := range [][2]float32{{0, 0}, {0.3, -0.7}, {-1.5, 2}} {
				want := make([]int32, 3*blockSize)

				for i := range want {
					want[i] = rng.Int32N(2001) - 1000

					if rng.IntN(3) == 0 {
						want[i] = rng.Int32N(5) - 2
					}
				}

				for c := range 3 {
					copy(s.coeffs[c], want[c*blockSize:(c+1)*blockSize])
				}

				invGlobalScale := float32(1.0 / 4096)

				v.dequantBlock(s, hfDCT, invGlobalScale, quant, cc[0], cc[1], blockSize, 0)

				for c := range 3 {
					clear(s.coeffs[c][:blockSize])
				}

				v.quantizeBlock(s, hfDCT, invGlobalScale, quant, cc[0], cc[1], blockSize, 0)

				for c := range 3 {
					for k := range blockSize {
						if got := s.coeffs[c][k]; got != want[c*blockSize+k] {
							t.Fatalf("%s biases quant %d correlation %v channel %d coefficient %d: %d, want %d",
								name, quant, cc, c, k, got, want[c*blockSize+k])
						}
					}
				}
			}
		}
	}
}

func quantStep(v *varDCT, c, k int, invGlobalScale float32, quant uint32) float32 {
	scale := invGlobalScale / float32(quant)

	switch c {
	case 0:
		scale *= v.xdm
	case 2:
		scale *= v.bdm
	}

	return v.dq.matrix(hfDCT, c)[k] * scale
}

func TestQuantizeIsStable(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 37))

	s := testScratch()

	worst := 0.0

	for name, biases := range testBiases {
		v := testVarDCT(biases)

		for _, quant := range []uint32{1, 9, 64} {
			for range 50 {
				var target [3][blockSize]float32

				invGlobalScale := float32(1.0 / 4096)

				for c := range 3 {
					for k := range blockSize {
						target[c][k] = float32(rng.NormFloat64()*40) * quantStep(v, c, k, invGlobalScale, quant)
						s.block[c][k] = target[c][k]
					}
				}

				v.quantizeBlock(s, hfDCT, invGlobalScale, quant, 0.4, -0.2, blockSize, 0)

				first := make([]int32, 3*blockSize)

				for c := range 3 {
					copy(first[c*blockSize:], s.coeffs[c][:blockSize])
				}

				v.dequantBlock(s, hfDCT, invGlobalScale, quant, 0.4, -0.2, blockSize, 0)

				for c := range 3 {
					for k := range blockSize {
						step := quantStep(v, c, k, invGlobalScale, quant)
						d := math.Abs(float64(s.block[c][k]-target[c][k])) / float64(step)
						worst = math.Max(worst, d)
					}
				}

				v.quantizeBlock(s, hfDCT, invGlobalScale, quant, 0.4, -0.2, blockSize, 0)

				for c := range 3 {
					for k := range blockSize {
						if got := s.coeffs[c][k]; got != first[c*blockSize+k] {
							t.Fatalf("%s biases quant %d channel %d coefficient %d: requantised to %d, was %d",
								name, quant, c, k, got, first[c*blockSize+k])
						}
					}
				}

				base := make([]float32, 3*blockSize)

				for c := range 3 {
					copy(s.coeffs[c], first[c*blockSize:])
				}

				v.dequantBlock(s, hfDCT, invGlobalScale, quant, 0.4, -0.2, blockSize, 0)

				for c := range 3 {
					copy(base[c*blockSize:], s.block[c][:blockSize])
				}

				for _, c := range [2]int{0, 2} {
					for _, delta := range [2]int32{-1, 1} {
						for i := range 3 {
							copy(s.coeffs[i], first[i*blockSize:])
						}

						for k := range blockSize {
							s.coeffs[c][k] += delta
						}

						v.dequantBlock(s, hfDCT, invGlobalScale, quant, 0.4, -0.2, blockSize, 0)

						for k := range blockSize {
							was := math.Abs(float64(base[c*blockSize+k] - target[c][k]))
							now := math.Abs(float64(s.block[c][k] - target[c][k]))

							if now < was*(1-1e-6) {
								t.Fatalf("%s biases quant %d channel %d coefficient %d: %d is %g from the target, %d is %g",
									name, quant, c, k, first[c*blockSize+k], was,
									first[c*blockSize+k]+delta, now)
							}
						}
					}
				}
			}
		}
	}

	if worst > 1.0 {
		t.Fatalf("reconstruction is %g quantisation steps from the target", worst)
	}

	t.Logf("reconstruction is at most %.3f quantisation steps from the target", worst)
}

func testFrame(w, h int) *frame {
	f := &frame{}

	f.hdr.width, f.hdr.height = uint32(w), uint32(h)
	f.hdr.encoding = encVarDCT
	f.hdr.upsampling = 1
	f.hdr.groupSizeShift = 1
	f.hdr.pass.numPasses = 1
	f.hdr.xQMScale, f.hdr.bQMScale = 3, 2

	f.vardct = newVarDCT(f, defaultOpsinInverseMatrix().quant, nil)

	v := f.vardct

	v.dq.tables = libraryTables
	v.bctx = blockContextMap{contextMap: defaultBlockContextMap, numLFContexts: 1, numContexts: 15}
	v.cmap = colorCorrelation{factor: defaultColorFactor, baseB: 1}
	v.quant = quantizerParams{globalScale: 4096, quantLF: 16}
	v.numHistograms = 1
	v.xdm = powf(1/1.25, float32(f.hdr.xQMScale)-2)
	v.bdm = powf(1/1.25, float32(f.hdr.bQMScale)-2)

	v.passes = make([]varDCTPass, 1)

	for o := range numOrders {
		for c := range 3 {
			v.passes[0].orders[3*o+c] = naturalOrders[o]
		}
	}

	return f
}

func TestACGroupRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(41, 43))

	for _, dims := range [][2]int{{64, 48}, {8, 8}, {129, 17}, {256, 128}} {
		f := testFrame(dims[0], dims[1])
		v := f.vardct

		for i := range v.transform {
			v.rawQuant[i] = int32(1 + rng.IntN(24))
			v.quantLF[i] = 0
		}

		big := dims[0] == 256

		for by := range v.bh {
			for bx := range v.bw {
				pos := by*v.bw + bx

				if v.transform[pos] != invalidTransform {
					continue
				}

				t := hfDCT

				if big && bx%4 == 0 && by%4 == 0 && bx+4 <= v.bw && by+4 <= v.bh {
					t = [3]hfTransform{hfDCT16X16, hfDCT32X32, hfDCT16X8}[rng.IntN(3)]
				}

				cx, cy := t.blocks()

				v.transform[pos] = 128 | uint8(t)

				for iy := range cy {
					for ix := range cx {
						if ix != 0 || iy != 0 {
							v.transform[(by+iy)*v.bw+bx+ix] = 0
						}
					}
				}
			}
		}

		var enc worker

		enc.alloc(f, 1)

		src := &enc.dct

		for c := range 3 {
			for i := range src.coeffs[c] {
				src.coeffs[c][i] = 0
			}
		}

		rc := f.blockGroupRect(0)

		type placed struct {
			t      hfTransform
			bx, by int
			offset int
		}

		var blocks []placed

		total := 0

		for by := range rc.h {
			for bx := range rc.w {
				raw := v.transform[(rc.y0+by)*v.bw+rc.x0+bx]
				if raw < 128 {
					continue
				}

				t := hfTransform(raw & 127)
				cx, cy := t.blocks()

				blocks = append(blocks, placed{t, bx, by, total})
				total += cx * cy * blockSize
			}
		}

		for c := range 3 {
			for i := range total {
				switch {
				case rng.IntN(4) == 0:
					src.coeffs[c][i] = rng.Int32N(41) - 20
				case rng.IntN(16) == 0:
					src.coeffs[c][i] = rng.Int32N(4001) - 2000
				default:
					src.coeffs[c][i] = 0
				}
			}
		}

		want := make([][]int32, 3)

		for c := range 3 {
			want[c] = make([]int32, total)
			copy(want[c], src.coeffs[c][:total])

			for _, b := range blocks {
				cx, cy := b.t.blocks()
				order := naturalOrders[hfShapeID[b.t]]

				for k := range cx * cy {
					want[c][b.offset+int(order[k])] = 0
				}
			}
		}

		numContexts := int(v.numHistograms) * v.bctx.numACContexts()
		values := make([]valueCounts, numContexts+zeroDensityContextLimit-zeroDensityContextCount)

		if err := v.walkACGroup(rc, src, 0, 0, 0, &v.passes[0].orders, func(ctx int, packed uint32) {
			values[ctx].add(packed)
		}); err != nil {
			t.Fatalf("%v: collecting gave %v", dims, err)
		}

		used, perCluster := clusterContexts(values[:numContexts], 256)

		for i := numContexts; i < len(values); i++ {
			perCluster[0].merge(&values[i])
		}

		hist, counts := buildHistograms(used, perCluster)

		var bw bitWriter

		sw := symbolWriter{codes: hist.write(&bw, counts)}

		if err := bw.err(); err != nil {
			t.Fatalf("%v: histograms gave %v", dims, err)
		}

		hist.resize(numContexts + zeroDensityContextLimit - zeroDensityContextCount)

		if err := v.walkACGroup(rc, src, 0, 0, 0, &v.passes[0].orders, func(ctx int, packed uint32) {
			sw.writeUnsigned(hist, &bw, ctx, packed)
		}); err != nil {
			t.Fatalf("%v: writing gave %v", dims, err)
		}

		if err := bw.err(); err != nil {
			t.Fatalf("%v: body gave %v", dims, err)
		}

		var dec worker

		dec.alloc(f, 1)

		r := newBitReader(append(bw.bytes(), make([]byte, 64)...))

		v.passes[0].hist.read(r, numContexts, true)

		if err := r.err(); err != nil {
			t.Fatalf("%v: reading histograms gave %v", dims, err)
		}

		v.passes[0].hist.resize(numContexts + zeroDensityContextLimit - zeroDensityContextCount)

		if err := f.decodeVarDCTGroup(&dec, 0, []*bitReader{r}); err != nil {
			t.Fatalf("%v: decoding gave %v", dims, err)
		}

		ref := make([][]float32, 3)

		for c := range 3 {
			ref[c] = make([]float32, len(v.pixels[c]))
		}

		rs := testScratch()

		xcc, bcc := v.cmap.yToX(0), v.cmap.yToB(0)

		for _, b := range blocks {
			pos := (rc.y0+b.by)*v.bw + rc.x0 + b.bx
			cx, cy := b.t.blocks()
			numCoeffs := cx * cy * blockSize

			for c := range 3 {
				copy(rs.coeffs[c], want[c][b.offset:b.offset+numCoeffs])
			}

			v.dequantBlock(rs, b.t, v.quant.invGlobalScale(), uint32(v.rawQuant[pos]),
				xcc, bcc, numCoeffs, 0)

			for c := range 3 {
				lf := make([]float32, maxCoeffBlocks*maxCoeffBlocks)

				transformToPixels(b.t, lf, rs.block[c][:numCoeffs], rs.scratch)

				stride := v.bw * blockDim
				bwPix, bhPix := cx*blockDim, cy*blockDim

				for y := range bhPix {
					copy(ref[c][((rc.y0+b.by)*blockDim+y)*stride+(rc.x0+b.bx)*blockDim:][:bwPix],
						rs.block[c][y*bwPix:])
				}
			}
		}

		for c := range 3 {
			for i := range ref[c] {
				if got := v.pixels[c][i]; got != ref[c][i] {
					t.Fatalf("%v: channel %d sample %d is %g, want %g", dims, c, i, got, ref[c][i])
				}
			}
		}
	}
}

func TestVarDCTHeadersRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(53, 59))

	for range 300 {
		var (
			lq lfQuantFactors
			qp quantizerParams
			cc colorCorrelation
		)

		lq.factors = lfQuantDefault

		if rng.IntN(2) == 0 {
			for i := range lq.factors {
				lq.factors[i] = float32(rng.IntN(2047)+1) / 128 / 128
			}
		}

		qp.globalScale = uint32(rng.IntN(73728)) + 1
		qp.quantLF = uint32(rng.IntN(1 << 16))

		if qp.quantLF == 0 || rng.IntN(4) == 0 {
			qp.quantLF = 16
		}

		cc = defaultColorCorrelation()

		if rng.IntN(2) == 0 {
			cc = colorCorrelation{
				factor: []uint32{defaultColorFactor, 256, uint32(rng.IntN(256)) + 2,
					uint32(rng.IntN(1<<16)) + 258}[rng.IntN(4)],
				baseX:  float32(rng.IntN(2049)-1024) / 512,
				baseB:  float32(rng.IntN(2049)-1024) / 512,
				ytoxLF: int32(rng.IntN(256)) - 128,
				ytobLF: int32(rng.IntN(256)) - 128,
			}
		}

		var w bitWriter

		lq.write(&w)
		qp.write(&w)
		cc.write(&w)

		if err := w.err(); err != nil {
			t.Fatalf("%v %v %v: write gave %v", lq, qp, cc, err)
		}

		r := newBitReader(append(w.bytes(), make([]byte, 16)...))

		var (
			lq2 lfQuantFactors
			qp2 quantizerParams
			cc2 colorCorrelation
		)

		lq2.read(r)
		qp2.read(r)
		cc2.read(r)

		if err := r.err(); err != nil {
			t.Fatalf("%v %v %v: read gave %v", lq, qp, cc, err)
		}

		if lq2 != lq || qp2 != qp || cc2 != cc {
			t.Fatalf("read back %v %v %v, want %v %v %v", lq2, qp2, cc2, lq, qp, cc)
		}
	}
}

func TestQuantizerParamsAreShortest(t *testing.T) {
	cases := []struct {
		globalScale, quantLF uint32
		bits                 int
	}{
		{1, 16, 15}, {2048, 16, 15}, {2049, 16, 15}, {4096, 16, 15},
		{4097, 16, 16}, {8192, 16, 16}, {8193, 16, 20}, {73728, 16, 20},
		{1, 1, 20}, {1, 32, 20}, {1, 33, 23}, {1, 256, 23},
		{1, 257, 31}, {1, 65536, 31},
	}

	for _, c := range cases {
		var w bitWriter

		q := quantizerParams{globalScale: c.globalScale, quantLF: c.quantLF}
		q.write(&w)

		if err := w.err(); err != nil {
			t.Fatalf("%v: %v", c, err)
		}

		if w.pos() != c.bits {
			t.Fatalf("scale %d quantLF %d took %d bits, want %d",
				c.globalScale, c.quantLF, w.pos(), c.bits)
		}

		r := newBitReader(append(w.bytes(), make([]byte, 8)...))

		var got quantizerParams

		got.read(r)

		if got != q {
			t.Fatalf("read back %v, want %v", got, q)
		}
	}
}

func TestDequantMatricesWrite(t *testing.T) {
	var w bitWriter

	d := dequantMatrices{tables: libraryTables}
	d.write(&w)

	if err := w.err(); err != nil {
		t.Fatalf("the library tables gave %v", err)
	}

	r := newBitReader(append(w.bytes(), make([]byte, 8)...))

	if !r.bit() {
		t.Fatal("the library flag was not set")
	}

	var custom bitWriter

	d.tables[3] = append([]float32(nil), d.tables[3]...)
	d.write(&custom)

	if custom.err() != ErrUnsupported {
		t.Fatalf("a custom table gave %v, want %v", custom.err(), ErrUnsupported)
	}
}

func TestBlockContextMapRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(61, 67))

	cases, custom := 0, 0

	for range 200 {
		b := blockContextMap{
			contextMap:    defaultBlockContextMap,
			numLFContexts: 1,
			numContexts:   15,
		}

		if rng.IntN(2) == 0 {
			lf := 1

			for c := range 3 {
				n := rng.IntN(2)
				b.lfThresholds[c] = make([]int32, n)

				for i := range b.lfThresholds[c] {
					b.lfThresholds[c][i] = int32(rng.IntN(2001)) - 1000
				}

				lf *= n + 1
			}

			nqf := rng.IntN(3)

			for lf*(nqf+1) > 64 {
				nqf--
			}

			b.qfThresholds = make([]uint32, nqf)

			for i := range b.qfThresholds {
				b.qfThresholds[i] = uint32(rng.IntN(299)) + 1
			}

			b.numLFContexts = lf

			size := 3 * numOrders * lf * (nqf + 1)
			b.contextMap = make([]uint8, size)

			clusters := rng.IntN(16) + 1

			for i := range b.contextMap {
				b.contextMap[i] = uint8(rng.IntN(clusters))
			}

			for i := range min(clusters, size) {
				b.contextMap[i] = uint8(i)
			}

			m := uint8(0)

			for _, v := range b.contextMap {
				m = max(m, v)
			}

			b.numContexts = int(m) + 1
			custom++
		}

		var w bitWriter

		b.write(&w)

		if err := w.err(); err != nil {
			t.Fatalf("write gave %v", err)
		}

		r := newBitReader(append(w.bytes(), make([]byte, 16)...))

		var got blockContextMap

		got.read(r)

		if err := r.err(); err != nil {
			t.Fatalf("read gave %v", err)
		}

		if got.numContexts != b.numContexts || got.numLFContexts != b.numLFContexts {
			t.Fatalf("read back %d/%d contexts, want %d/%d",
				got.numContexts, got.numLFContexts, b.numContexts, b.numLFContexts)
		}

		if len(got.contextMap) != len(b.contextMap) {
			t.Fatalf("read back a map of %d, want %d", len(got.contextMap), len(b.contextMap))
		}

		for c := range 3 {
			if len(got.lfThresholds[c]) != len(b.lfThresholds[c]) {
				t.Fatalf("channel %d read back %d lf thresholds, want %d",
					c, len(got.lfThresholds[c]), len(b.lfThresholds[c]))
			}

			for i, v := range b.lfThresholds[c] {
				if got.lfThresholds[c][i] != v {
					t.Fatalf("channel %d lf threshold %d read back as %d, want %d",
						c, i, got.lfThresholds[c][i], v)
				}
			}
		}

		if len(got.qfThresholds) != len(b.qfThresholds) {
			t.Fatalf("read back %d quant factor thresholds, want %d",
				len(got.qfThresholds), len(b.qfThresholds))
		}

		for i, v := range b.qfThresholds {
			if got.qfThresholds[i] != v {
				t.Fatalf("quant factor threshold %d read back as %d, want %d",
					i, got.qfThresholds[i], v)
			}
		}

		for i := range b.contextMap {
			if got.contextMap[i] != b.contextMap[i] {
				t.Fatalf("map entry %d read back as %d, want %d",
					i, got.contextMap[i], b.contextMap[i])
			}
		}

		cases++
	}

	t.Logf("%d block context maps round-tripped, %d of them custom", cases, custom)
}

func trivialTree() *maTree {
	return &maTree{nodes: []treeNode{{property: -1 - predGradient, child: 1}}}
}

func TestLFAndHFMetadataRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(71, 73))

	for _, dims := range [][2]int{{64, 48}, {8, 8}, {200, 130}} {
		f := testFrame(dims[0], dims[1])
		v := f.vardct

		f.lfQuant.factors = lfQuantDefault
		v.cmap.ytoxLF, v.cmap.ytobLF = 127, -128

		for by := range v.bh {
			for bx := range v.bw {
				pos := by*v.bw + bx

				if v.transform[pos] != invalidTransform {
					continue
				}

				tr := hfDCT

				if bx%2 == 0 && by%2 == 0 && bx+2 <= v.bw && by+2 <= v.bh && rng.IntN(3) == 0 {
					tr = hfDCT16X16
				}

				cx, cy := tr.blocks()
				q := int32(1 + rng.IntN(200))

				for iy := range cy {
					for ix := range cx {
						id := uint8(tr)

						if ix == 0 && iy == 0 {
							id |= 128
						}

						v.transform[(by+iy)*v.bw+bx+ix] = id
						v.rawQuant[(by+iy)*v.bw+bx+ix] = q
					}
				}
			}
		}

		for i := range v.epf {
			v.epf[i] = uint8(rng.IntN(8))
		}

		for i := range v.ytox {
			v.ytox[i] = int8(rng.IntN(256) - 128)
			v.ytob[i] = int8(rng.IntN(256) - 128)
		}

		for c := range 3 {
			for i := range v.lf[c] {
				v.lf[c][i] = float32(rng.NormFloat64()) * 0.02
			}
		}

		target := make([][]float32, 3)

		for c := range 3 {
			target[c] = append([]float32(nil), v.lf[c]...)
		}

		wantTransform := append([]uint8(nil), v.transform...)
		wantQuant := append([]int32(nil), v.rawQuant...)
		wantEPF := append([]uint8(nil), v.epf...)
		wantYtoX := append([]int8(nil), v.ytox...)
		wantYtoB := append([]int8(nil), v.ytob...)

		var w bitWriter

		hdr := groupHeader{wp: defaultWeightedHeader()}

		f.encodeVarDCTLF(&w, 0, &hdr, trivialTree())
		f.encodeHFMetadata(&w, 0, &hdr, trivialTree())

		if err := w.err(); err != nil {
			t.Fatalf("%v: write gave %v", dims, err)
		}

		wantLF := make([][]float32, 3)

		for c := range 3 {
			wantLF[c] = append([]float32(nil), v.lf[c]...)
		}

		g := testFrame(dims[0], dims[1])
		g.lfQuant.factors = lfQuantDefault
		g.vardct.cmap.ytoxLF, g.vardct.cmap.ytobLF = 127, -128

		r := newBitReader(append(w.bytes(), make([]byte, 64)...))

		g.decodeVarDCTLF(r, 0, nil)
		g.decodeHFMetadata(r, 0, nil)

		if err := r.err(); err != nil {
			t.Fatalf("%v: read gave %v", dims, err)
		}

		gv := g.vardct

		for i := range wantTransform {
			if gv.transform[i] != wantTransform[i] || gv.rawQuant[i] != wantQuant[i] {
				t.Fatalf("%v: block %d is transform %d quant %d, want %d and %d",
					dims, i, gv.transform[i], gv.rawQuant[i], wantTransform[i], wantQuant[i])
			}

			if gv.epf[i] != wantEPF[i] {
				t.Fatalf("%v: block %d sharpness %d, want %d", dims, i, gv.epf[i], wantEPF[i])
			}
		}

		for i := range wantYtoX {
			if gv.ytox[i] != wantYtoX[i] || gv.ytob[i] != wantYtoB[i] {
				t.Fatalf("%v: tile %d correlation %d/%d, want %d/%d",
					dims, i, gv.ytox[i], gv.ytob[i], wantYtoX[i], wantYtoB[i])
			}
		}

		for c := range 3 {
			for i := range wantLF[c] {
				if gv.lf[c][i] != wantLF[c][i] {
					t.Fatalf("%v: channel %d lf sample %d is %g, want %g",
						dims, c, i, gv.lf[c][i], wantLF[c][i])
				}
			}
		}

		invQuantLF := float32(globalScaleDenom) /
			(float32(v.quant.globalScale) * float32(v.quant.quantLF))

		for c := range 3 {
			step := float64(f.lfQuant.factors[c] * invQuantLF)

			for i := range target[c] {
				if d := math.Abs(float64(gv.lf[c][i] - target[c][i])); d > step*0.5001 {
					t.Fatalf("%v: channel %d lf sample %d is %g from the target, half a step is %g",
						dims, c, i, d, step*0.5)
				}
			}
		}
	}
}

func TestHFMetadataRejectsQuant(t *testing.T) {
	f := testFrame(16, 16)
	v := f.vardct

	for i := range v.transform {
		v.transform[i] = 128 | uint8(hfDCT)
		v.rawQuant[i] = 257
	}

	var w bitWriter

	f.encodeHFMetadata(&w, 0, &groupHeader{wp: defaultWeightedHeader()}, trivialTree())

	if w.err() != ErrInvalid {
		t.Fatalf("a quantiser of 257 gave %v, want %v", w.err(), ErrInvalid)
	}
}

func quantizeBiasReference(c int, target float32, biases *[4]float32) int32 {
	if target != target {
		return 0
	}

	target = min(max(target, -1<<30), 1<<30)

	near := int32(math.Round(float64(target)))
	best := near
	bestErr := float32(math.Inf(1))

	for cand := near - 1; cand <= near+1; cand++ {
		e := adjustQuantBias(c, cand, biases) - target

		if e < 0 {
			e = -e
		}

		if e < bestErr {
			best, bestErr = cand, e
		}
	}

	return best
}

func TestQuantizeBiasMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewPCG(53, 59))

	biasSets := [][4]float32{
		{0.9453, 0.9299, 0.9501, 0.145},
		{0.3, 0.62, 0.94, 0.5},
		{0.001, 1, 0.5, 2},
		{0, 0, 0, 0},
	}

	targets := []float32{
		0, -0, 0.5, -0.5, 1.5, -1.5, 2.5, -2.5, 0.49999997, -0.49999997,
		1, -1, 1 << 23, -(1 << 23), (1 << 23) + 1,
		1 << 30, -1 << 30,
		1 << 31, float32(math.Inf(1)), float32(math.Inf(-1)),
		float32(math.NaN()),
	}

	for i := range 400000 {
		var v float32

		switch i % 4 {
		case 0:
			v = float32(rng.NormFloat64()) * 4
		case 1:
			v = float32(rng.NormFloat64()) * 1000
		case 2:
			v = float32(rng.IntN(2000)-1000) + 0.5
		default:
			v = float32(rng.NormFloat64()) * 1e6
		}

		targets = append(targets, v)
	}

	for _, biases := range biasSets {
		b := biases

		for _, v := range targets {
			for c := range 3 {
				want := quantizeBiasReference(c, v, &b)

				if got := quantizeBias(c, v, &b); got != want {
					t.Fatalf("quantizeBias(%d, %v, %v) is %d, want %d", c, v, b, got, want)
				}
			}
		}
	}

	t.Logf("%d targets x %d bias sets x 3 channels agree exactly", len(targets), len(biasSets))
}

func TestVarDCTBodyRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(83, 89))

	for _, dims := range [][2]int{{64, 48}, {8, 8}, {120, 96}} {
		f := testFrame(dims[0], dims[1])
		v := f.vardct

		f.lfQuant.factors = lfQuantDefault
		v.cmap = colorCorrelation{factor: 137, baseX: 0.25, baseB: 0.75, ytoxLF: 9, ytobLF: -6}

		for i := range v.transform {
			v.transform[i] = 128 | uint8(hfDCT)
			v.rawQuant[i] = 200
		}

		pixels := make([][]float32, 3)

		for c := range 3 {
			pixels[c] = make([]float32, len(v.pixels[c]))

			for i := range pixels[c] {
				pixels[c][i] = float32(rng.NormFloat64()) * 0.1
			}
		}

		var enc worker

		enc.alloc(f, 1)

		s := &enc.dct
		stride := v.bw * blockDim

		for by := range v.bh {
			for bx := range v.bw {
				b := by*v.bw + bx

				for c := range 3 {
					var buf, lf [blockSize]float32

					for y := range blockDim {
						copy(buf[y*blockDim:], pixels[c][(by*blockDim+y)*stride+bx*blockDim:][:blockDim])
					}

					if !transformFromPixels(hfDCT, buf[:], lf[:], s.scratch) {
						t.Fatal("the plain transform was refused")
					}

					s.block[c] = append(s.block[c][:0], buf[:]...)
					v.lf[c][b] = lf[0]
				}

				v.quantizeBlock(s, hfDCT, v.quant.invGlobalScale(), uint32(v.rawQuant[b]),
					v.cmap.yToX(0), v.cmap.yToB(0), blockSize, b*blockSize)
			}
		}

		for c := range 3 {
			for b := range v.bw * v.bh {
				s.coeffs[c][b*blockSize] = 0
			}
		}

		hist, counts, err := f.buildACHistograms(s, []int{0})
		if err != nil {
			t.Fatalf("%v: histograms gave %v", dims, err)
		}

		var w bitWriter

		hdr := groupHeader{wp: defaultWeightedHeader()}

		f.encodeLFGlobal(&w)
		f.encodeVarDCTLF(&w, 0, &hdr, trivialTree())
		f.encodeHFMetadata(&w, 0, &hdr, trivialTree())
		codes := f.encodeHFGlobal(&w, hist, counts)

		if err := w.err(); err != nil {
			t.Fatalf("%v: header gave %v", dims, err)
		}

		hist.resize(f.acContexts() + zeroDensityContextLimit - zeroDensityContextCount)

		sw := symbolWriter{codes: codes}

		if err := f.encodeVarDCTGroup(&w, 0, s, hist, &sw, 0); err != nil {
			t.Fatalf("%v: group gave %v", dims, err)
		}

		if err := w.err(); err != nil {
			t.Fatalf("%v: body gave %v", dims, err)
		}

		g := testFrame(dims[0], dims[1])
		g.lfQuant.factors = lfQuantDefault

		r := newBitReader(append(w.bytes(), make([]byte, 64)...))

		g.lfQuant.read(r)
		g.vardct.quant.read(r)
		g.vardct.bctx.read(r)
		g.vardct.cmap.read(r)

		if r.bit() {
			t.Fatalf("%v: a global tree was announced", dims)
		}

		g.decodeVarDCTLF(r, 0, nil)
		g.decodeHFMetadata(r, 0, nil)
		g.decodeHFGlobal(r, nil)

		if err := r.err(); err != nil {
			t.Fatalf("%v: headers read gave %v", dims, err)
		}

		var dec worker

		dec.alloc(g, 1)

		if err := g.decodeVarDCTGroup(&dec, 0, []*bitReader{r}); err != nil {
			t.Fatalf("%v: group read gave %v", dims, err)
		}

		var worst float64

		for c := range 3 {
			for i := range pixels[c] {
				d := math.Abs(float64(g.vardct.pixels[c][i] - pixels[c][i]))
				worst = math.Max(worst, d)
			}
		}

		if worst > 0.02 {
			t.Fatalf("%v: reconstruction is %g from the source", dims, worst)
		}

		t.Logf("%v: reconstructed to within %.5f", dims, worst)
	}
}

func TestFrameEncodeSplitsBack(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 13))

	for _, dims := range [][2]int{{64, 48}, {600, 300}, {257, 1000}} {
		f := testFrame(dims[0], dims[1])
		f.hdr.flags = flagSkipAdaptiveLFSmoothing

		body := make([][]byte, f.hdr.numTOCEntries())

		for i := range body {
			body[i] = make([]byte, 1+rng.IntN(9))

			for j := range body[i] {
				body[i][j] = uint8(rng.IntN(256))
			}
		}

		m := defaultImageMetadata()

		var w bitWriter

		f.encode(&w, &m, uint32(dims[0]), uint32(dims[1]), body)

		if err := w.err(); err != nil {
			t.Fatalf("%v: encode gave %v", dims, err)
		}

		r := newBitReader(append(w.bytes(), make([]byte, 16)...))

		var got frameHeader

		got.read(r, &m, uint32(dims[0]), uint32(dims[1]))

		var tc toc

		tc.read(r, got.numTOCEntries())

		if err := r.err(); err != nil {
			t.Fatalf("%v: read gave %v", dims, err)
		}

		if len(tc.entries) != len(body) {
			t.Fatalf("%v: read back %d entries, want %d", dims, len(tc.entries), len(body))
		}

		if got.flags != f.hdr.flags || got.encoding != encVarDCT {
			t.Fatalf("%v: read back flags %d encoding %d", dims, got.flags, got.encoding)
		}

		for i, sec := range body {
			if tc.entries[i] != uint32(len(sec)) {
				t.Fatalf("%v: entry %d is %d, want %d", dims, i, tc.entries[i], len(sec))
			}

			for j, b := range sec {
				if v := uint8(r.bits(8)); v != b {
					t.Fatalf("%v: section %d byte %d is %d, want %d", dims, i, j, v, b)
				}
			}
		}
	}
}
