package jxl

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestDCTRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))

	for l := 0; l <= 8; l++ {
		n := 1 << l

		in := make([]float32, n)
		for i := range in {
			in[i] = float32(rng.NormFloat64())
		}

		mem := make([]float32, n)
		copy(mem, in)

		tmp := make([]float32, 3*n)
		dct1d(n, mem, tmp)

		for i := range mem {
			mem[i] /= float32(n)
		}

		out := make([]float32, n)
		idct1d(n, mem, 1, out, 1, tmp)

		for i := range out {
			if d := math.Abs(float64(out[i] - in[i])); d > 2e-5 {
				t.Fatalf("n=%d index %d: %g, want %g", n, i, out[i], in[i])
			}
		}
	}
}

func TestIDCTIsCosineBasis(t *testing.T) {
	for l := 0; l <= 8; l++ {
		n := 1 << l

		for f := range n {
			coeff := make([]float32, n)
			coeff[f] = 1

			out := make([]float32, n)
			idct1d(n, coeff, 1, out, 1, make([]float32, 2*n))

			var num, den float64

			for k := range n {
				c := math.Cos(math.Pi * float64(f) * (float64(k) + 0.5) / float64(n))
				num += float64(out[k]) * c
				den += c * c
			}

			scale := num / den

			want := 1.0
			if f > 0 {
				want = math.Sqrt2
			}

			if math.Abs(scale-want) > 1e-5 {
				t.Fatalf("n=%d freq %d: amplitude %g, want %g", n, f, scale, want)
			}

			for k := range n {
				want := scale * math.Cos(math.Pi*float64(f)*(float64(k)+0.5)/float64(n))

				if d := math.Abs(float64(out[k]) - want); d > 1e-4 {
					t.Fatalf("n=%d freq %d index %d: %g, want %g", n, f, k, out[k], want)
				}
			}
		}
	}
}

func TestIDCT2DSeparable(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))

	sizes := [][2]int{{8, 8}, {8, 16}, {16, 8}, {32, 8}, {8, 32}, {16, 32}, {32, 16}, {64, 64}, {4, 4}, {4, 8}, {8, 4}}

	for _, s := range sizes {
		rows, cols := s[0], s[1]
		n := rows * cols

		in := make([]float32, n)
		for i := range in {
			in[i] = float32(rng.NormFloat64())
		}

		lo, hi := min(rows, cols), max(rows, cols)

		want := make([]float32, n)
		tmp := make([]float32, 2*hi)
		mid := make([]float32, n)

		src := in
		if rows < cols {
			src = make([]float32, n)
			transposeBlock(rows, cols, in, cols, src, rows)
		}

		for j := range rows {
			idct1d(cols, src[j:], rows, mid[j:], rows, tmp)
		}

		transposeBlock(cols, rows, mid, rows, want, cols)
		copy(mid, want)

		for j := range cols {
			idct1d(rows, mid[j:], cols, want[j:], cols, tmp)
		}

		got := make([]float32, n)
		copy(got, in)
		idct2d(rows, cols, got, make([]float32, n+2*hi))

		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%dx%d (lo %d) index %d: %g, want %g", rows, cols, lo, i, got[i], want[i])
			}
		}
	}
}

func TestScaledDCT2DAgainstIDCT(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))

	for _, cx := range []int{1, 2, 4, 8} {
		for _, cy := range []int{1, 2, 4, 8} {
			rows, cols := cy*blockDim, cx*blockDim
			stride := max(cols, rows)

			lf := make([]float32, cx*cy)
			for i := range lf {
				lf[i] = float32(rng.NormFloat64())
			}

			block := make([]float32, rows*cols)
			scaledDCT2D(cy, cx, lf, block, stride, make([]float32, 2*cx*cy+3*max(cx, cy)))

			idct2d(rows, cols, block, make([]float32, rows*cols+2*stride))

			for y := range cy {
				for x := range cx {
					var sum float32

					for iy := range blockDim {
						for ix := range blockDim {
							sum += block[(y*blockDim+iy)*cols+x*blockDim+ix]
						}
					}

					got := sum / float32(blockSize)
					if d := math.Abs(float64(got - lf[y*cx+x])); d > 1e-5 {
						t.Fatalf("cx=%d cy=%d block (%d,%d): mean %g, want %g", cx, cy, x, y, got, lf[y*cx+x])
					}
				}
			}
		}
	}
}

func TestNaturalCoeffOrder(t *testing.T) {
	order1x1 := []uint32{
		0, 1, 8, 16, 9, 2, 3, 10, 17, 24, 32, 25, 18, 11, 4, 5, 12, 19, 26, 33, 40, 48, 41, 34, 27,
		20, 13, 6, 7, 14, 21, 28, 35, 42, 49, 56, 57, 50, 43, 36, 29, 22, 15, 23, 30, 37, 44, 51,
		58, 59, 52, 45, 38, 31, 39, 46, 53, 60, 61, 54, 47, 55, 62, 63,
	}

	order2x1 := []uint32{
		0, 1, 16, 2, 3, 17, 32, 18, 4, 5, 19, 33, 48, 34, 20, 6, 7, 21, 35, 49, 64, 50, 36, 22, 8,
		9, 23, 37, 51, 65, 80, 66, 52, 38, 24, 10, 11, 25, 39, 53, 67, 81, 96, 82, 68, 54, 40, 26,
		12, 13, 27, 41, 55, 69, 83, 97, 112, 98, 84, 70, 56, 42, 28, 14, 15, 29, 43, 57, 71, 85,
		99, 113, 114, 100, 86, 72, 58, 44, 30, 31, 45, 59, 73, 87, 101, 115, 116, 102, 88, 74, 60,
		46, 47, 61, 75, 89, 103, 117, 118, 104, 90, 76, 62, 63, 77, 91, 105, 119, 120, 106, 92, 78,
		79, 93, 107, 121, 122, 108, 94, 95, 109, 123, 124, 110, 111, 125, 126, 127,
	}

	for _, c := range []struct {
		t    hfTransform
		want []uint32
	}{{hfDCT, order1x1}, {hfDCT8X16, order2x1}} {
		got := naturalCoeffOrder(c.t)

		if len(got) != len(c.want) {
			t.Fatalf("transform %d: %d entries, want %d", c.t, len(got), len(c.want))
		}

		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("transform %d index %d: %d, want %d", c.t, i, got[i], c.want[i])
			}
		}
	}
}

func TestTransformDCOnly(t *testing.T) {
	scratch := make([]float32, maxCoeffArea+2*maxBlockDim)

	for tt := hfTransform(0); tt < numHFTransforms; tt++ {
		cx, cy := tt.blocks()

		lf := make([]float32, cx*cy)
		for i := range lf {
			lf[i] = 2
		}

		buf := make([]float32, cx*cy*blockSize)
		transformToPixels(tt, lf, buf, scratch)

		for i, v := range buf {
			if d := math.Abs(float64(v - 2)); d > 1e-4 {
				t.Fatalf("transform %d pixel %d: %g, want 2", tt, i, v)
			}
		}
	}
}

func TestLehmerGolden(t *testing.T) {
	code := []uint32{1, 1, 2, 3, 3, 6, 0, 1}
	size, skip := uint32(16), uint32(4)

	perm := make([]uint32, size)
	for i := range perm {
		perm[i] = uint32(i)
	}

	out, err := decodeLehmer(code, perm[skip:])
	if err != nil {
		t.Fatal(err)
	}

	copy(perm[skip:], out)

	want := []uint32{0, 1, 2, 3, 5, 6, 8, 10, 11, 15, 4, 9, 7, 12, 13, 14}
	for i := range want {
		if perm[i] != want[i] {
			t.Fatalf("index %d: %d, want %d", i, perm[i], want[i])
		}
	}
}
