package jxl

import (
	"bytes"
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

		if worst > 0.012 {
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

	for _, dims := range [][2]int{{64, 48}, {17, 9}, {120, 96}} {
		w, h := dims[0], dims[1]

		data, err := encodeVarDCTImage(gradientRGB(w, h), w, h, 200)
		if err != nil {
			t.Fatalf("%v: encode gave %v", dims, err)
		}

		src := filepath.Join(dir, "ours.jxl")
		dst := filepath.Join(dir, "ours.png")

		if err := os.WriteFile(src, data, 0o600); err != nil {
			t.Fatal(err)
		}

		if out, err := exec.Command(djxl, src, dst).CombinedOutput(); err != nil {
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
