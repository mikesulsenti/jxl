package jxl

import (
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
				f, err := decodeFloat(data, Options{Level: 10, Threads: tc.threads})
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
