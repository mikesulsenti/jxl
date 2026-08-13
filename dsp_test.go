package jxl

import (
	"math/rand/v2"
	"testing"
)

func randomEPF(r *rand.Rand, w, h, planes int, amp float32) *epfPlanes {
	e := &epfPlanes{w: w, h: h, sw: divCeil(w, blockDim)}

	e.planes = make([][]float32, planes)
	for c := range e.planes {
		e.planes[c] = make([]float32, w*h)

		for i := range e.planes[c] {
			e.planes[c][i] = (r.Float32() - 0.5) * amp
		}
	}

	e.scales = make([]float32, planes)
	for c := range e.scales {
		e.scales[c] = r.Float32() + 0.25
	}

	e.sigma = make([]float32, e.sw*divCeil(h, blockDim))
	for i := range e.sigma {
		e.sigma[i] = -(r.Float32()*3.5 + 0.3)

		if i%7 == 0 {
			e.sigma[i] = -100
		}
	}

	e.init()

	return e
}

func (e *epfPlanes) clone() *epfPlanes {
	c := *e
	c.planes = make([][]float32, len(e.planes))

	for i := range e.planes {
		c.planes[i] = make([]float32, len(e.planes[i]))
		copy(c.planes[i], e.planes[i])
	}

	c.init()

	return &c
}

func TestGaborishAsmMatchesGo(t *testing.T) {
	if gaborishAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(7, 8))

	for _, size := range [][2]int{{24, 5}, {32, 8}, {33, 9}, {64, 16}, {129, 33}} {
		w, h := size[0], size[1]

		plane := make([]float32, w*h)
		for i := range plane {
			plane[i] = r.Float32()*2 - 1
		}

		want := make([]float32, w*h)
		got := make([]float32, w*h)

		copy(want, plane)
		copy(got, plane)

		asm := gaborishAsm
		gaborishAsm = nil
		want = gaborish(want, make([]float32, w*h), w, h, 1, 0.11, 0.02)
		gaborishAsm = asm

		got = gaborish(got, make([]float32, w*h), w, h, 1, 0.11, 0.02)

		for i, v := range want {
			if got[i] != v {
				t.Fatalf("%dx%d sample %d (%d,%d): got %v, want %v",
					w, h, i, i%w, i/w, got[i], v)
			}
		}
	}
}

func TestIDCTColsAsmMatchesGo(t *testing.T) {
	if !useIDCTCols {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(9, 10))

	scratch := make([]float32, maxCoeffArea+2*maxBlockDim)

	for _, size := range [][2]int{{4, 8}, {8, 4}, {8, 16}, {16, 8}, {16, 16}, {16, 32}, {32, 16}, {32, 32}} {
		rows, cols := size[0], size[1]

		want := make([]float32, rows*cols)
		for i := range want {
			want[i] = r.Float32()*40 - 20
		}

		got := make([]float32, rows*cols)
		copy(got, want)

		o8, oc := useIDCT8x8, useIDCTCols
		useIDCT8x8, useIDCTCols = false, false
		idct2d(rows, cols, want, scratch)
		useIDCT8x8, useIDCTCols = o8, oc

		idct2d(rows, cols, got, scratch)

		for i, v := range want {
			if got[i] != v {
				t.Fatalf("%dx%d sample %d (%d,%d): got %v, want %v",
					rows, cols, i, i%cols, i/cols, got[i], v)
			}
		}
	}
}

func TestIDCTAsmMatchesGo(t *testing.T) {
	if !useIDCT8x8 {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(5, 6))

	scratch := make([]float32, maxCoeffArea+2*maxBlockDim)

	for range 64 {
		var want, got [64]float32

		for i := range want {
			want[i] = r.Float32()*40 - 20
		}

		got = want

		useIDCT8x8 = false
		idct2d(blockDim, blockDim, want[:], scratch)
		useIDCT8x8 = true

		idct2d(blockDim, blockDim, got[:], scratch)

		for i, v := range want {
			if got[i] != v {
				t.Fatalf("sample %d (%d,%d): got %v, want %v", i, i%8, i/8, got[i], v)
			}
		}
	}
}

func TestXYBAsmMatchesGo(t *testing.T) {
	if xybSRGBAsm == nil {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(3, 4))

	o := defaultOpsinInverseMatrix()
	p := newXYBParams(&o, 255)
	k := p.srgbConsts()
	k512 := p.srgbConsts512()

	for _, n := range []int{8, 16, 24, 1024} {
		x := make([]float32, n)
		y := make([]float32, n)
		b := make([]float32, n)

		for i := range x {
			x[i] = r.Float32()*0.1 - 0.05
			y[i] = r.Float32()*1.2 - 0.1
			b[i] = r.Float32()*1.2 - 0.1
		}

		wx := make([]float32, n)
		wy := make([]float32, n)
		wb := make([]float32, n)

		copy(wx, x)
		copy(wy, y)
		copy(wb, b)

		for i := range wx {
			cr, cg, cb := p.apply(wx[i], wy[i], wb[i])
			wx[i], wy[i], wb[i] = linearToSRGB(cr), linearToSRGB(cg), linearToSRGB(cb)
		}

		for _, wide := range []bool{false, true} {
			if wide && (xybSRGB512Asm == nil || n&15 != 0) {
				continue
			}

			gx := make([]float32, n)
			gy := make([]float32, n)
			gb := make([]float32, n)

			copy(gx, x)
			copy(gy, y)
			copy(gb, b)

			if wide {
				xybSRGB512Asm(&gx[0], &gy[0], &gb[0], n, k512)
			} else {
				xybSRGBAsm(&gx[0], &gy[0], &gb[0], n, k)
			}

			for i := range wx {
				if gx[i] != wx[i] || gy[i] != wy[i] || gb[i] != wb[i] {
					t.Fatalf("wide %v n=%d sample %d: got %v %v %v, want %v %v %v",
						wide, n, i, gx[i], gy[i], gb[i], wx[i], wy[i], wb[i])
				}
			}
		}
	}
}

func TestEPFAsmMatchesGo(t *testing.T) {
	have := [3]bool{epf0Asm != nil, epf1Asm != nil, epf2Asm != nil}

	if have == [3]bool{} {
		t.Skip("no kernel compiled in")
	}

	run := func(e *epfPlanes, pass int) {
		switch pass {
		case 0:
			e.pass0(0.9, 0.75)
		case 1:
			e.pass1(1, 0.75)
		default:
			e.pass2(6.5, 0.75)
		}
	}

	disable := func(pass int) func() {
		switch pass {
		case 0:
			a := epf0Asm
			epf0Asm = nil

			return func() { epf0Asm = a }
		case 1:
			a := epf1Asm
			epf1Asm = nil

			return func() { epf1Asm = a }
		default:
			a := epf2Asm
			epf2Asm = nil

			return func() { epf2Asm = a }
		}
	}

	r := rand.New(rand.NewPCG(1, 2))

	for pass := range have {
		if !have[pass] {
			continue
		}

		for _, amp := range []float32{2, 0.3, 0.05} {
			for _, size := range [][2]int{{24, 9}, {32, 16}, {33, 17}, {64, 24}, {100, 40}, {256, 33}} {
				want := randomEPF(r, size[0], size[1], 3, amp)
				got := want.clone()

				restore := disable(pass)
				run(want, pass)
				restore()

				run(got, pass)

				for c := range want.planes {
					for i, v := range want.planes[c] {
						if got.planes[c][i] != v {
							t.Fatalf("pass %d amp %v %dx%d plane %d sample %d (%d,%d): got %v, want %v",
								pass, amp, size[0], size[1], c, i, i%size[0], i/size[0],
								got.planes[c][i], v)
						}
					}
				}
			}
		}
	}
}

func TestDequantAsmMatchesGo(t *testing.T) {
	if !useDequant {
		t.Skip("no kernel compiled in")
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
	}

	wide := useDequant512
	defer func() { useDequant512 = wide }()

	for _, w := range []bool{false, true} {
		if w && !wide {
			continue
		}

		useDequant512 = w

		for _, tr := range []hfTransform{hfDCT, hfDCT16X16, hfDCT32X32, hfDCT8X16} {
			cx, cy := tr.blocks()
			n := cx * cy * blockSize

			for c := range 3 {
				for i := range s.coeffs[c][:n] {
					switch r.IntN(5) {
					case 0:
						s.coeffs[c][i] = 0
					case 1:
						s.coeffs[c][i] = 1
					case 2:
						s.coeffs[c][i] = -1
					default:
						s.coeffs[c][i] = int32(r.IntN(2001) - 1000)
					}
				}
			}

			var want [3][]float32

			useDequant = false
			v.dequantBlock(&s, tr, 0.7, 13, 0.31, 1.2, n, 0)
			useDequant = true

			for c := range 3 {
				want[c] = make([]float32, n)
				copy(want[c], s.block[c][:n])
			}

			v.dequantBlock(&s, tr, 0.7, 13, 0.31, 1.2, n, 0)

			for c := range 3 {
				for i, x := range want[c] {
					if s.block[c][i] != x {
						t.Fatalf("transform %d channel %d coefficient %d (q=%d): got %v, want %v",
							tr, c, i, s.coeffs[c][i], s.block[c][i], x)
					}
				}
			}
		}
	}
}

func TestTransposeAsmMatchesGo(t *testing.T) {
	if !useTranspose8 {
		t.Skip("no kernel compiled in")
	}

	r := rand.New(rand.NewPCG(13, 14))

	for _, size := range [][2]int{{8, 8}, {8, 16}, {16, 8}, {16, 16}, {32, 16}, {16, 32}, {32, 32}} {
		rows, cols := size[0], size[1]

		src := make([]float32, rows*cols)
		for i := range src {
			src[i] = r.Float32()
		}

		want := make([]float32, rows*cols)
		got := make([]float32, rows*cols)

		useTranspose8 = false
		transposeBlock(rows, cols, src, cols, want, rows)
		useTranspose8 = true

		transposeBlock(rows, cols, src, cols, got, rows)

		for i, v := range want {
			if got[i] != v {
				t.Fatalf("%dx%d sample %d: got %v, want %v", rows, cols, i, got[i], v)
			}
		}
	}
}
