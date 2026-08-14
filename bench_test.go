package jxl

import (
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func benchStream(b *testing.B, name string) {
	b.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		b.Skip(err)
	}

	for _, tc := range []struct {
		name    string
		threads int
	}{{"serial", 1}, {"threaded", 0}} {
		b.Run(tc.name, func(b *testing.B) {
			var pixels int64

			b.ReportAllocs()

			for b.Loop() {
				f, err := decodeFloatBytes(data, Options{Level: 10, Threads: tc.threads})
				if err != nil {
					b.Fatal(err)
				}

				pixels += int64(f.Width) * int64(f.Height) * int64(len(f.Frames))
			}

			b.StopTimer()

			if n := b.Elapsed().Seconds(); n > 0 {
				b.ReportMetric(float64(pixels)/n/1e6, "Mpixel/s")
			}
		})
	}
}

func BenchmarkVarDCT(b *testing.B)      { benchStream(b, "bench/dct_big.jxl") }
func BenchmarkVarDCTSmall(b *testing.B) { benchStream(b, "bench/dct_mid_e7.jxl") }
func BenchmarkEPF(b *testing.B)         { benchStream(b, "streams/dct_epf3.jxl") }
func BenchmarkProgressive(b *testing.B) { benchStream(b, "bench/dct_prog.jxl") }
func BenchmarkSqueeze(b *testing.B)     { benchStream(b, "bench/rgb_squeeze.jxl") }
func BenchmarkLossless(b *testing.B)    { benchStream(b, "streams/rgb_lossless.jxl") }
func BenchmarkTree(b *testing.B)        { benchStream(b, "streams/tree_rct.jxl") }
func BenchmarkGray(b *testing.B)        { benchStream(b, "streams/gray_lossless.jxl") }

func BenchmarkEncode(b *testing.B) {
	for _, dims := range [][2]int{{256, 256}, {1920, 1080}} {
		src := texturedImage(dims[0], dims[1])

		for _, tc := range []struct {
			name    string
			threads int
		}{{"serial", 1}, {"threaded", 0}} {
			b.Run(fmt.Sprintf("%dx%d/%s", dims[0], dims[1], tc.name), func(b *testing.B) {
				var pixels int64

				b.ReportAllocs()

				for b.Loop() {
					if err := Encode(io.Discard, src, EncodeOptions{Threads: tc.threads}); err != nil {
						b.Fatal(err)
					}

					pixels += int64(dims[0]) * int64(dims[1])
				}

				b.StopTimer()

				if n := b.Elapsed().Seconds(); n > 0 {
					b.ReportMetric(float64(pixels)/n/1e6, "Mpixel/s")
				}
			})
		}
	}
}

func BenchmarkCbrt(b *testing.B) {
	if !useCbrt {
		b.Skip("no kernel")
	}

	r := rand.New(rand.NewPCG(9, 10))

	src := make([]float32, 1<<16)
	for i := range src {
		src[i] = float32(r.NormFloat64())
	}

	buf := make([]float32, len(src))

	wide := useCbrt512
	defer func() { useCbrt512 = wide }()

	for _, tc := range []struct {
		name string
		w    bool
	}{{"a_avx2", false}, {"b_avx512", true}, {"c_avx2", false}, {"d_avx512", true}} {
		if tc.w && !wide {
			continue
		}

		b.Run(tc.name, func(b *testing.B) {
			useCbrt512 = tc.w

			var n int64

			for b.Loop() {
				copy(buf, src)
				cbrtBlock(buf)

				n += int64(len(buf))
			}

			b.StopTimer()

			if e := b.Elapsed().Seconds(); e > 0 {
				b.ReportMetric(float64(n)/e/1e6, "Msample/s")
			}
		})
	}
}

func BenchmarkDequant(b *testing.B) {
	if !useDequant512 {
		b.Skip("no AVX-512 kernel")
	}

	r := rand.New(rand.NewPCG(11, 12))

	v := varDCT{biases: [4]float32{0.9453, 0.9299, 0.9501, 0.145}, xdm: 0.8, bdm: 1.25}

	for i := range numQuantTables {
		n := quantSizeX[i] * quantSizeY[i] * blockSize
		v.dq.tables[i] = make([]float32, 3*n)

		for j := range v.dq.tables[i] {
			v.dq.tables[i][j] = r.Float32()*4 + 0.01
		}
	}

	var s dctScratch

	for c := range 3 {
		s.coeffs[c] = make([]int32, maxCoeffArea)
		s.block[c] = make([]float32, maxCoeffArea)

		for i := range s.coeffs[c] {
			if r.IntN(5) == 0 {
				continue
			}

			s.coeffs[c][i] = int32(r.IntN(2001) - 1000)
		}
	}

	wide := useDequant512
	defer func() { useDequant512 = wide }()

	for _, tc := range []struct {
		name string
		w    bool
	}{{"a_avx2", false}, {"b_avx512", true}, {"c_avx2", false}, {"d_avx512", true}} {
		b.Run(tc.name, func(b *testing.B) {
			useDequant512 = tc.w

			var n int64

			for b.Loop() {
				for _, tr := range []hfTransform{hfDCT, hfDCT16X16, hfDCT32X32, hfDCT8X16} {
					cx, cy := tr.blocks()
					c := cx * cy * blockSize

					v.dequantBlock(&s, tr, 0.7, 13, 0.31, 1.2, c, 0)
					n += int64(c)
				}
			}

			b.StopTimer()

			if e := b.Elapsed().Seconds(); e > 0 {
				b.ReportMetric(float64(n)/e/1e6, "Mcoeff/s")
			}
		})
	}
}

func BenchmarkXYBKernel(b *testing.B) {
	if xybSRGB512Asm == nil {
		b.Skip("no AVX-512 kernel")
	}

	const n = 4096

	r := rand.New(rand.NewPCG(3, 4))

	o := defaultOpsinInverseMatrix()
	p := newXYBParams(&o, 255)
	k := p.srgbConsts()
	k512 := p.srgbConsts512()

	x := make([]float32, n)
	y := make([]float32, n)
	v := make([]float32, n)

	for i := range x {
		x[i] = r.Float32()*0.1 - 0.05
		y[i] = r.Float32()*1.2 - 0.1
		v[i] = r.Float32()*1.2 - 0.1
	}

	for _, tc := range []struct {
		name string
		w    bool
	}{{"a_avx2", false}, {"b_avx512", true}, {"c_avx2", false}, {"d_avx512", true}} {
		b.Run(tc.name, func(b *testing.B) {
			var c int64

			for b.Loop() {
				if tc.w {
					xybSRGB512Asm(&x[0], &y[0], &v[0], n, k512)
				} else {
					xybSRGBAsm(&x[0], &y[0], &v[0], n, k)
				}

				c += n
			}

			b.StopTimer()

			if e := b.Elapsed().Seconds(); e > 0 {
				b.ReportMetric(float64(c)/e/1e6, "Msample/s")
			}
		})
	}
}
