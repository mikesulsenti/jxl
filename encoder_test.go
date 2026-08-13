package jxl

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gradientRGB(w, h int) [3][]float32 {
	var rgb [3][]float32

	for c := range 3 {
		rgb[c] = make([]float32, w*h)
	}

	for y := range h {
		for x := range w {
			i := y*w + x

			rgb[0][i] = 0.2 + 0.7*float32(x)/float32(max(w-1, 1))
			rgb[1][i] = 0.2 + 0.7*float32(y)/float32(max(h-1, 1))
			rgb[2][i] = 0.5
		}
	}

	return rgb
}

func TestEncodeDecodesBack(t *testing.T) {
	for _, dims := range [][2]int{{64, 48}, {17, 9}, {8, 8}} {
		w, h := dims[0], dims[1]

		rgb := gradientRGB(w, h)

		data, err := encodeVarDCTImage(rgb, w, h, 200)
		if err != nil {
			t.Fatalf("%v: encode gave %v", dims, err)
		}

		got, err := DecodeFloat(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%v: decode gave %v (%d bytes)", dims, err, len(data))
		}

		if got.Width != w || got.Height != h {
			t.Fatalf("%v: decoded %dx%d", dims, got.Width, got.Height)
		}

		var worst float64

		for y := range h {
			for x := range w {
				for c := range 3 {
					want := linearToSRGB(rgb[c][y*w+x])
					have := got.Frames[0][(y*w+x)*got.Channels+c]
					worst = math.Max(worst, math.Abs(float64(have-want)))
				}
			}
		}

		t.Logf("%v: %d bytes, worst error %.5f", dims, len(data), worst)

		if worst > 0.015 {
			t.Fatalf("%v: reconstruction is %g from the source", dims, worst)
		}
	}
}

func blockyRGB(rng *rand.Rand, w, h int) [3][]float32 {
	var rgb [3][]float32

	for c := range 3 {
		rgb[c] = make([]float32, w*h)
	}

	bw := (w + 7) / 8

	tone := make([]float32, 3*bw*((h+7)/8))

	for i := range tone {
		tone[i] = float32(0.2 + 0.7*rng.Float64())
	}

	for y := range h {
		for x := range w {
			b := (y/8)*bw + x/8

			for c := range 3 {
				rgb[c][y*w+x] = tone[b*3+c]
			}
		}
	}

	return rgb
}

func TestEncodeBlocky(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 5))

	w, h := 64, 48

	rgb := blockyRGB(rng, w, h)

	data, err := encodeVarDCTImage(rgb, w, h, 200)
	if err != nil {
		t.Fatalf("encode gave %v", err)
	}

	got, err := DecodeFloat(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode gave %v", err)
	}

	var worst float64

	for i := range w * h {
		for c := range 3 {
			d := math.Abs(float64(got.Frames[0][i*got.Channels+c] - linearToSRGB(rgb[c][i])))
			worst = math.Max(worst, d)
		}
	}

	t.Logf("%d bytes for %dx%d blocks, worst error %.5f", len(data), w, h, worst)

	if worst > 0.012 {
		t.Fatalf("reconstruction is %g from the source", worst)
	}
}

func TestEncodeNoise(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 13))

	w, h := 40, 24

	var rgb [3][]float32

	for c := range 3 {
		rgb[c] = make([]float32, w*h)

		for i := range rgb[c] {
			rgb[c][i] = float32(0.2 + 0.7*rng.Float64())
		}
	}

	data, err := encodeVarDCTImage(rgb, w, h, 250)
	if err != nil {
		t.Fatalf("encode gave %v", err)
	}

	got, err := DecodeFloat(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode gave %v", err)
	}

	var worst float64

	for i := range w * h {
		for c := range 3 {
			d := math.Abs(float64(got.Frames[0][i*got.Channels+c] - linearToSRGB(rgb[c][i])))
			worst = math.Max(worst, d)
		}
	}

	t.Logf("%d bytes for %dx%d noise, worst error %.5f", len(data), w, h, worst)

	if worst > 0.04 {
		t.Fatalf("reconstruction is %g from the source", worst)
	}
}

func TestEncodeAgainstLibjxl(t *testing.T) {
	djxl, err := exec.LookPath("djxl")
	if err != nil {
		t.Skip("djxl is not on the path")
	}

	dir := t.TempDir()

	for _, dims := range [][2]int{{64, 48}, {17, 9}, {120, 96}, {257, 200}, {300, 260}, {512, 512}, {2100, 300}} {
		w, h := dims[0], dims[1]

		var src image.Image = testImage(w, h)

		if w > groupDim || h > groupDim {
			src = texturedImage(w, h)
		}

		var buf bytes.Buffer

		if err := Encode(&buf, src); err != nil {
			t.Fatalf("%v: encode gave %v", dims, err)
		}

		data := buf.Bytes()

		jxlPath := filepath.Join(dir, "ours.jxl")
		dst := filepath.Join(dir, "ours.png")

		if err := os.WriteFile(jxlPath, data, 0o600); err != nil {
			t.Fatal(err)
		}

		if out, err := exec.Command(djxl, jxlPath, dst).CombinedOutput(); err != nil {
			t.Fatalf("%v: djxl refused our file: %v\n%s", dims, err, out)
		}

		ours, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%v: our decoder gave %v", dims, err)
		}

		f, err := os.Open(dst)
		if err != nil {
			t.Fatal(err)
		}

		theirs, err := png.Decode(f)
		f.Close()

		if err != nil {
			t.Fatalf("%v: djxl output gave %v", dims, err)
		}

		if theirs.Bounds() != ours.Bounds() {
			t.Fatalf("%v: djxl gave %v, we gave %v", dims, theirs.Bounds(), ours.Bounds())
		}

		var worst float64

		b := ours.Bounds()

		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r1, g1, b1, _ := ours.At(x, y).RGBA()
				r2, g2, b2, _ := theirs.At(x, y).RGBA()

				for _, d := range [3]float64{
					math.Abs(float64(r1) - float64(r2)),
					math.Abs(float64(g1) - float64(g2)),
					math.Abs(float64(b1) - float64(b2)),
				} {
					worst = math.Max(worst, d/65535)
				}
			}
		}

		t.Logf("%v: %d bytes, libjxl differs by %.6f", dims, len(data), worst)

		if worst > 0.005 {
			t.Fatalf("%v: libjxl and we disagree by %g", dims, worst)
		}
	}
}

func testImage(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			m.SetNRGBA(x, y, color.NRGBA{
				R: uint8(60 + 150*x/max(w-1, 1)),
				G: uint8(60 + 150*y/max(h-1, 1)),
				B: uint8(128 + 60*((x/8+y/8)%2)),
				A: 255,
			})
		}
	}

	return m
}

func TestEncodeImage(t *testing.T) {
	for _, dims := range [][2]int{{64, 48}, {31, 17}, {8, 8}} {
		w, h := dims[0], dims[1]

		src := testImage(w, h)

		var buf bytes.Buffer

		if err := Encode(&buf, src); err != nil {
			t.Fatalf("%v: encode gave %v", dims, err)
		}

		got, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("%v: decode gave %v", dims, err)
		}

		if got.Bounds() != src.Bounds() {
			t.Fatalf("%v: decoded %v", dims, got.Bounds())
		}

		var worst float64

		for y := range h {
			for x := range w {
				r1, g1, b1, _ := src.At(x, y).RGBA()
				r2, g2, b2, _ := got.At(x, y).RGBA()

				for _, d := range [3]float64{
					math.Abs(float64(r1) - float64(r2)),
					math.Abs(float64(g1) - float64(g2)),
					math.Abs(float64(b1) - float64(b2)),
				} {
					worst = math.Max(worst, d/65535)
				}
			}
		}

		t.Logf("%v: %d bytes, worst error %.5f", dims, buf.Len(), worst)

		if worst > 0.03 {
			t.Fatalf("%v: reconstruction is %g from the source", dims, worst)
		}
	}
}

func encodeDecode(t *testing.T, src image.Image) (image.Image, int) {
	t.Helper()

	var buf bytes.Buffer

	if err := Encode(&buf, src); err != nil {
		t.Fatalf("%v: encode gave %v", src.Bounds(), err)
	}

	got, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("%v: decode gave %v", src.Bounds(), err)
	}

	if got.Bounds() != src.Bounds() {
		t.Fatalf("%v: decoded %v", src.Bounds(), got.Bounds())
	}

	return got, buf.Len()
}

func TestEncodeMultiGroup(t *testing.T) {
	for _, dims := range [][2]int{{257, 200}, {300, 260}, {512, 512}, {600, 129}, {2100, 300}} {
		w, h := dims[0], dims[1]

		src := texturedImage(w, h)
		got, size := encodeDecode(t, src)

		cw, ch := min(w, groupDim), min(h, groupDim)
		crop := texturedImage(cw, ch)
		alone, _ := encodeDecode(t, crop)

		worst, single := maxError(src, got), maxError(crop, alone)

		t.Logf("%v: %d bytes, worst error %.5f against %.5f for one group",
			dims, size, worst, single)

		if worst > single*1.5 {
			t.Fatalf("%v: reconstruction is %g from the source, %g on one group",
				dims, worst, single)
		}

		for y := range ch {
			for x := range cw {
				if got.At(x, y) != alone.At(x, y) {
					t.Fatalf("%v: pixel %d,%d is %v alongside other groups, %v alone",
						dims, x, y, got.At(x, y), alone.At(x, y))
				}
			}
		}
	}
}

func TestEncodeQualityAffectsSize(t *testing.T) {
	src := testImage(64, 48)

	var prev int

	for _, q := range []int{20, 50, 90, 100} {
		var buf bytes.Buffer

		if err := Encode(&buf, src, EncodeOptions{Quality: q}); err != nil {
			t.Fatalf("quality %d: %v", q, err)
		}

		if _, err := Decode(bytes.NewReader(buf.Bytes())); err != nil {
			t.Fatalf("quality %d: decode gave %v", q, err)
		}

		t.Logf("quality %d: %d bytes", q, buf.Len())

		if buf.Len() <= prev {
			t.Fatalf("quality %d gave %d bytes, not more than %d", q, buf.Len(), prev)
		}

		prev = buf.Len()
	}
}

func texturedImage(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			fx, fy := float64(x), float64(y)

			r := 0.5 + 0.3*math.Sin(fx/7.3) + 0.15*math.Sin(fy/3.1+1)
			g := 0.5 + 0.25*math.Sin((fx+fy)/5.7) + 0.2*math.Cos(fx/11.9)
			b := 0.5 + 0.3*math.Cos(fy/4.3) + 0.15*math.Sin((fx-fy)/9.1)

			m.SetNRGBA(x, y, color.NRGBA{
				R: uint8(math.Round(min(max(r, 0), 1) * 255)),
				G: uint8(math.Round(min(max(g, 0), 1) * 255)),
				B: uint8(math.Round(min(max(b, 0), 1) * 255)),
				A: 255,
			})
		}
	}

	return m
}

func maxError(a, b image.Image) float64 {
	worst := 0.0
	r := a.Bounds()

	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()

			for _, d := range [3]float64{
				math.Abs(float64(r1) - float64(r2)),
				math.Abs(float64(g1) - float64(g2)),
				math.Abs(float64(b1) - float64(b2)),
			} {
				worst = math.Max(worst, d/65535)
			}
		}
	}

	return worst
}

func TestEncodeAgainstCjxl(t *testing.T) {
	cjxl, err := exec.LookPath("cjxl")
	if err != nil {
		t.Skip("cjxl is not on the path")
	}

	dir := t.TempDir()
	src := texturedImage(128, 96)

	png0 := filepath.Join(dir, "src.png")

	f, err := os.Create(png0)
	if err != nil {
		t.Fatal(err)
	}

	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}

	f.Close()

	type point struct {
		bytes int
		err   float64
	}

	var (
		curve []point
		worst float64
	)

	for q := 5; q <= 100; q += 5 {
		var buf bytes.Buffer

		if err := Encode(&buf, src, EncodeOptions{Quality: q}); err != nil {
			t.Fatalf("quality %d: %v", q, err)
		}

		got, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("quality %d: %v", q, err)
		}

		curve = append(curve, point{buf.Len(), maxError(src, got)})
	}

	match := func(target float64) point {
		best := curve[0]

		for _, p := range curve {
			if p.err <= target && p.bytes < best.bytes {
				best = p
			}

			if best.err > target && p.err < best.err {
				best = p
			}
		}

		return best
	}

	for _, dist := range []string{"1.0", "3.0", "6.0"} {
		out := filepath.Join(dir, "cjxl-"+dist+".jxl")

		cmd := exec.Command(cjxl, png0, out, "-d", dist, "--quiet")

		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cjxl -d %s failed: %v\n%s", dist, err, o)
		}

		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}

		theirs, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("we could not decode cjxl -d %s: %v", dist, err)
		}

		e := maxError(src, theirs)
		ours := match(e)
		ratio := float64(ours.bytes) / float64(len(data))

		t.Logf("cjxl -d %s: %d bytes at %.5f; ours %d bytes at %.5f (%.1fx)",
			dist, len(data), e, ours.bytes, ours.err, ratio)

		worst = math.Max(worst, ratio)
	}

	if worst > 8 {
		t.Fatalf("we are %.1fx larger than cjxl at matched distortion, ratchet is 8x", worst)
	}
}
