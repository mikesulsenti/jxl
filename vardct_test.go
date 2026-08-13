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
