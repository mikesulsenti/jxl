package jxl

import (
	"math"
	"math/rand/v2"
	"slices"
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

func TestFDCT2DInvertsIDCT2D(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 13))

	for _, rows := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256} {
		for _, cols := range []int{1, 2, 4, 8, 16, 32, 64, 128, 256} {
			if rows*cols > maxCoeffArea || max(rows, cols) > 8*min(rows, cols) {
				continue
			}

			n := rows * cols

			in := make([]float32, n)
			for i := range in {
				in[i] = float32(rng.NormFloat64())
			}

			data := make([]float32, n)
			copy(data, in)

			scratch := make([]float32, 2*n+6*max(rows, cols))

			idct2d(rows, cols, data, scratch)
			fdct2d(rows, cols, data, scratch)

			var worst float64

			for i := range data {
				worst = math.Max(worst, math.Abs(float64(data[i]-in[i])))
			}

			if worst > 1e-4 {
				t.Fatalf("%dx%d: coefficients round-tripped with error %g", rows, cols, worst)
			}

			copy(data, in)

			fdct2d(rows, cols, data, scratch)
			idct2d(rows, cols, data, scratch)

			worst = 0

			for i := range data {
				worst = math.Max(worst, math.Abs(float64(data[i]-in[i])))
			}

			if worst > 1e-4 {
				t.Fatalf("%dx%d: pixels round-tripped with error %g", rows, cols, worst)
			}
		}
	}
}

func TestTransformFromPixels(t *testing.T) {
	rng := rand.New(rand.NewPCG(17, 19))

	scratch := make([]float32, 4*maxCoeffArea)

	for range 200 {
		var pixels, buf, lf [blockSize]float32

		for i := range pixels {
			pixels[i] = float32(rng.NormFloat64()) * 100
		}

		buf = pixels

		if !transformFromPixels(hfDCT, buf[:], lf[:], scratch) {
			t.Fatal("the plain 8x8 transform was refused")
		}

		buf[0] = 0

		transformToPixels(hfDCT, lf[:], buf[:], scratch)

		for i := range pixels {
			if d := math.Abs(float64(buf[i] - pixels[i])); d > 2e-3 {
				t.Fatalf("sample %d: %g, want %g", i, buf[i], pixels[i])
			}
		}
	}

	for tr := hfTransform(0); tr < numHFTransforms; tr++ {
		var buf, lf [maxCoeffArea]float32

		cx, cy := tr.blocks()
		want := cx*cy > 1 || slices.Contains(smallTransforms, tr)

		if got := transformFromPixels(tr, buf[:], lf[:], scratch); got != want {
			t.Fatalf("transform %d: accepted is %v, want %v", tr, got, want)
		}
	}
}

var smallTransforms = []hfTransform{hfDCT, hfDCT4X4, hfDCT8X4, hfDCT4X8, hfDCT2X2}

func TestTransformFromPixelsSmall(t *testing.T) {
	rng := rand.New(rand.NewPCG(23, 29))

	scratch := make([]float32, 4*maxCoeffArea)

	for _, tr := range smallTransforms {
		worst := 0.0

		for range 200 {
			var pixels, buf, lf [blockSize]float32

			for i := range pixels {
				pixels[i] = float32(rng.NormFloat64()) * 100
			}

			buf = pixels

			if !transformFromPixels(tr, buf[:], lf[:], scratch) {
				t.Fatalf("transform %d was refused", tr)
			}

			if lf[0] != buf[0] {
				t.Fatalf("transform %d: lf is %g, the DC is %g", tr, lf[0], buf[0])
			}

			buf[0] = 0

			transformToPixels(tr, lf[:], buf[:], scratch)

			for i := range pixels {
				worst = math.Max(worst, math.Abs(float64(buf[i]-pixels[i])))
			}
		}

		t.Logf("transform %d round-trips to within %.2e", tr, worst)

		if worst > 5e-3 {
			t.Fatalf("transform %d round-trips to %g", tr, worst)
		}
	}
}

func TestTransformFromPixelsMultiBlock(t *testing.T) {
	rng := rand.New(rand.NewPCG(37, 41))

	scratch := make([]float32, 8*maxCoeffArea)

	for tr := hfTransform(0); tr < numHFTransforms; tr++ {
		cx, cy := tr.blocks()

		if cx*cy == 1 {
			continue
		}

		n := cx * cy * blockSize

		pixels := make([]float32, n)
		for i := range pixels {
			pixels[i] = float32(rng.NormFloat64()) * 100
		}

		buf := make([]float32, n)
		copy(buf, pixels)

		lf := make([]float32, maxCoeffBlocks*maxCoeffBlocks)

		if !transformFromPixels(tr, buf, lf, scratch) {
			t.Fatalf("transform %d was refused", tr)
		}

		transformToPixels(tr, lf, buf, scratch)

		var worst float64

		for i := range pixels {
			worst = math.Max(worst, math.Abs(float64(buf[i]-pixels[i])))
		}

		if worst > 5e-3 {
			t.Fatalf("transform %d: round trip error %g", tr, worst)
		}
	}
}

func TestUnscaledDCT2DInvertsScaled(t *testing.T) {
	rng := rand.New(rand.NewPCG(29, 31))

	for _, rows := range []int{1, 2, 4, 8, 16, 32} {
		for _, cols := range []int{1, 2, 4, 8, 16, 32} {
			if max(rows, cols) > 8*min(rows, cols) {
				continue
			}

			n := rows * cols

			in := make([]float32, n)
			for i := range in {
				in[i] = float32(rng.NormFloat64())
			}

			stride := blockDim * max(rows, cols)

			out := make([]float32, stride*max(rows, cols)+stride)
			scratch := make([]float32, 4*n+16*max(rows, cols))

			scaledDCT2D(rows, cols, in, out, stride, scratch)

			back := make([]float32, n)

			unscaledDCT2D(rows, cols, out, stride, back, scratch)

			var worst float64

			for i := range in {
				worst = math.Max(worst, math.Abs(float64(back[i]-in[i])))
			}

			if worst > 1e-4 {
				t.Fatalf("%dx%d: round trip error %g", rows, cols, worst)
			}
		}
	}
}
