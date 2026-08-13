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
		values := make([][]uint32, numContexts+zeroDensityContextLimit-zeroDensityContextCount)

		if err := v.walkACGroup(rc, src, 0, 0, 0, &v.passes[0].orders, func(ctx int, packed uint32) {
			values[ctx] = append(values[ctx], packed)
		}); err != nil {
			t.Fatalf("%v: collecting gave %v", dims, err)
		}

		used, perCluster := clusterContexts(values[:numContexts], 256)

		for _, vs := range values[numContexts:] {
			perCluster[0] = append(perCluster[0], vs...)
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
