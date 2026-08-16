package jxl

import (
	"bytes"
	"cmp"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func planeRows(rgb [3][]float32, w int) rowSource {
	return func(y int, dst *[3][]float32) {
		for c := range 3 {
			copy(dst[c], rgb[c][y*w:(y+1)*w])
		}
	}
}

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

		data, err := encodeVarDCTImage(planeRows(rgb, w), w, h, encodeParams{distance: 0.1, strength: 1})
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

	data, err := encodeVarDCTImage(planeRows(rgb, w), w, h, encodeParams{distance: 1, strength: 1})
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

	data, err := encodeVarDCTImage(planeRows(rgb, w), w, h, encodeParams{distance: 0.1, strength: 1})
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

	if worst > 0.45 {
		t.Fatalf("reconstruction is %g from the source", worst)
	}
}

func TestEncodeAgainstLibjxl(t *testing.T) {
	djxl, err := exec.LookPath("djxl")
	if err != nil {
		t.Skip("djxl is not on the path")
	}

	dir := t.TempDir()

	for _, dims := range [][3]int{
		{64, 48, 0}, {17, 9, 0}, {120, 96, 0}, {257, 200, 0}, {300, 260, 0}, {512, 512, 0},
		{2100, 300, 0}, {96, 72, 1}, {300, 260, 1},
	} {
		w, h := dims[0], dims[1]

		var src image.Image = testImage(w, h)

		switch {
		case dims[2] == 1:
			src = alphaImage(w, h)
		case w > groupDim || h > groupDim:
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
				r1, g1, b1, a1 := ours.At(x, y).RGBA()
				r2, g2, b2, a2 := theirs.At(x, y).RGBA()

				for _, d := range [4]float64{
					math.Abs(float64(r1) - float64(r2)),
					math.Abs(float64(g1) - float64(g2)),
					math.Abs(float64(b1) - float64(b2)),
					math.Abs(float64(a1) - float64(a2)),
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

		if worst > 0.1 {
			t.Fatalf("%v: reconstruction is %g from the source", dims, worst)
		}
	}
}

func TestLinearFrom16MatchesDirect(t *testing.T) {
	for v := range uint32(65536) {
		want := srgbToLinear(float32(v) / 65535)

		if got := linearFrom16(v); got != want {
			t.Fatalf("linearFrom16(%d) is %v, want %v", v, got, want)
		}
	}
}

func TestEncodeThreadsAreDeterministic(t *testing.T) {
	for _, dims := range [][3]int{{300, 260, 0}, {512, 512, 0}, {600, 129, 0}, {300, 260, 1}} {
		var src image.Image = texturedImage(dims[0], dims[1])

		if dims[2] == 1 {
			src = alphaImage(dims[0], dims[1])
		}

		var serial bytes.Buffer

		if err := Encode(&serial, src, EncodeOptions{Threads: 1}); err != nil {
			t.Fatalf("%v: serial encode gave %v", dims, err)
		}

		for _, threads := range []int{0, 2, 3, 8} {
			for range 3 {
				var buf bytes.Buffer

				if err := Encode(&buf, src, EncodeOptions{Threads: threads}); err != nil {
					t.Fatalf("%v: %d threads gave %v", dims, threads, err)
				}

				if !bytes.Equal(buf.Bytes(), serial.Bytes()) {
					t.Fatalf("%v: %d threads gave %d bytes, serial gave %d",
						dims, threads, buf.Len(), serial.Len())
				}
			}
		}

		t.Logf("%v: %d bytes, identical across 1, 2, 3 and 8 threads", dims, serial.Len())
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

		flatWhole, _ := encodeQuality(t, src, DefaultQuality, 0)
		flatCrop, _ := encodeQuality(t, crop, DefaultQuality, 0)

		for y := range ch {
			for x := range cw {
				if flatWhole.At(x, y) != flatCrop.At(x, y) {
					t.Fatalf("%v: pixel %d,%d is %v alongside other groups, %v alone",
						dims, x, y, flatWhole.At(x, y), flatCrop.At(x, y))
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

func TestEncodeEffort(t *testing.T) {
	src := texturedImage(384, 256)

	sizes := make(map[int]int)
	errors := make(map[int]float64)

	for _, e := range []int{-3, 0, 1, 2, 3, 4, 5, 6, 7, 40} {
		var buf bytes.Buffer

		if err := Encode(&buf, src, EncodeOptions{Effort: e}); err != nil {
			t.Fatalf("effort %d: %v", e, err)
		}

		got, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("effort %d: decode gave %v", e, err)
		}

		worst := maxError(src, got)

		t.Logf("effort %d: %d bytes, worst error %.5f", e, buf.Len(), worst)

		sizes[e] = buf.Len()
		errors[e] = worst
	}

	if sizes[-3] != sizes[1] {
		t.Fatalf("effort -3 gave %d bytes, effort 1 gives %d", sizes[-3], sizes[1])
	}

	if sizes[40] != sizes[MaxEffort] {
		t.Fatalf("effort 40 gave %d bytes, effort %d gives %d", sizes[40], MaxEffort,
			sizes[MaxEffort])
	}

	if sizes[0] != sizes[DefaultEffort] {
		t.Fatalf("effort 0 gave %d bytes, effort %d gives %d", sizes[0], DefaultEffort,
			sizes[DefaultEffort])
	}

	for e := 1; e <= MaxEffort; e++ {
		if errors[e] > 2*errors[DefaultEffort] {
			t.Fatalf("effort %d is %.5f from the source, effort %d is %.5f",
				e, errors[e], DefaultEffort, errors[DefaultEffort])
		}
	}

	for _, e := range []int{2, 3, 4, 5, 6, 7} {
		if sizes[e] >= sizes[1] {
			t.Fatalf("effort %d gave %d bytes, no smaller than %d at effort 1",
				e, sizes[e], sizes[1])
		}
	}
}

func alphaImage(w, h int) *image.NRGBA {
	m := texturedImage(w, h)

	for y := range h {
		for x := range w {
			c := m.NRGBAAt(x, y)

			switch {
			case x < w/4:
				c.A = 0
			case x < w/2:
				c.A = uint8(255 * y / max(h-1, 1))
			case x < 3*w/4:
				c.A = 128
			}

			m.SetNRGBA(x, y, c)
		}
	}

	return m
}

func TestEncodeAlpha(t *testing.T) {
	for _, dims := range [][2]int{{64, 48}, {300, 260}, {600, 129}} {
		w, h := dims[0], dims[1]

		src := alphaImage(w, h)

		var buf bytes.Buffer

		if err := Encode(&buf, src); err != nil {
			t.Fatalf("%v: %v", dims, err)
		}

		got, err := Decode(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("%v: decode gave %v", dims, err)
		}

		worst := 0

		for y := range h {
			for x := range w {
				_, _, _, a := got.At(x, y).RGBA()

				d := int(a>>8) - int(src.NRGBAAt(x, y).A)
				if d < 0 {
					d = -d
				}

				worst = max(worst, d)
			}
		}

		t.Logf("%v: %d bytes, worst alpha error %d", dims, buf.Len(), worst)

		if worst != 0 {
			t.Fatalf("%v: alpha is %d off, it is coded losslessly", dims, worst)
		}
	}
}

func TestEncodeAlpha16(t *testing.T) {
	w, h := 200, 120

	base := texturedImage(w, h)
	src := image.NewNRGBA64(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			c := base.NRGBAAt(x, y)

			src.SetNRGBA64(x, y, color.NRGBA64{
				R: uint16(c.R) * 0x101,
				G: uint16(c.G) * 0x101,
				B: uint16(c.B) * 0x101,
				A: uint16(x*257 + y),
			})
		}
	}

	var buf bytes.Buffer

	if err := Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	got, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := got.(*image.NRGBA64); !ok {
		t.Fatalf("a 16 bit source decoded as %T", got)
	}

	for y := range h {
		for x := range w {
			_, _, _, a := got.At(x, y).RGBA()

			if want := uint32(src.NRGBA64At(x, y).A); a != want {
				t.Fatalf("alpha at %d,%d is %d, want %d", x, y, a, want)
			}
		}
	}

	t.Logf("%dx%d 16 bit alpha in %d bytes", w, h, buf.Len())
}

func encodedChannels(t *testing.T, src image.Image) int {
	t.Helper()

	var buf bytes.Buffer

	if err := Encode(&buf, src); err != nil {
		t.Fatal(err)
	}

	d, err := newDecoder(bytes.NewReader(buf.Bytes()), Options{Level: DefaultLevel})
	if err != nil {
		t.Fatal(err)
	}

	return len(d.hdr.meta.extraChannels)
}

func TestEncodeOpaqueDropsAlpha(t *testing.T) {
	if n := encodedChannels(t, texturedImage(96, 64)); n != 0 {
		t.Fatalf("an opaque image carries %d extra channels", n)
	}

	if n := encodedChannels(t, alphaImage(96, 64)); n != 1 {
		t.Fatalf("a transparent image carries %d extra channels", n)
	}

	opaque := alphaImage(96, 64)

	for y := range 64 {
		for x := range 96 {
			c := opaque.NRGBAAt(x, y)
			c.A = 255

			opaque.SetNRGBA(x, y, c)
		}
	}

	if n := encodedChannels(t, opaque); n != 0 {
		t.Fatalf("an image with a full alpha channel carries %d extra channels", n)
	}
}

func effortCurve(t *testing.T, tool, dir string, src image.Image, effort int) []ratePoint {
	t.Helper()

	var curve []ratePoint

	b := src.Bounds()

	for q := 40; q <= 95; q += 5 {
		data, err := encodeVarDCTImage(imageRows(src), b.Dx(), b.Dy(), encodeParams{
			distance: qualityToDistance(q),
			strength: adaptiveQuantStrength,
			effort:   effort,
		})
		if err != nil {
			t.Fatalf("effort %d: %v", effort, err)
		}

		got, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("effort %d: %v", effort, err)
		}

		curve = append(curve, ratePoint{len(data), ssimulacra2(t, tool, dir, src, got)})
	}

	return curve
}

// TestEncodeEffortQuality compares each effort against the default at matched
// size, which is the only comparison that says whether a wider search pays.
func TestEncodeEffortQuality(t *testing.T) {
	tool := ssimulacra2Path(t)
	dir := t.TempDir()

	corpus := aqCorpus()
	total := make([]float64, MaxEffort+1)

	for _, tc := range corpus {
		base := effortCurve(t, tool, dir, tc.img, DefaultEffort)

		for e := 1; e < DefaultEffort; e++ {
			mean, _, _ := aqDelta(base, effortCurve(t, tool, dir, tc.img, e))
			total[e] += mean
		}
	}

	for e := 1; e < DefaultEffort; e++ {
		mean := total[e] / float64(len(corpus))

		t.Logf("effort %d: %+.3f ssimulacra2 against effort %d at matched size",
			e, mean, DefaultEffort)

		if mean > 0.25 {
			t.Fatalf("effort %d beats effort %d by %+.3f", e, DefaultEffort, mean)
		}
	}

	if total[1]/float64(len(corpus)) > -0.5 {
		t.Fatalf("effort 1 costs only %+.3f, the ladder is inert",
			total[1]/float64(len(corpus)))
	}
}

func TestTransformCandidates(t *testing.T) {
	var prev []hfTransform

	for e := 1; e <= MaxEffort; e++ {
		got := transformCandidates(e)

		for _, t2 := range prev {
			if !slices.Contains(got, t2) {
				t.Fatalf("effort %d dropped %v", e, t2)
			}
		}

		prev = got
	}

	if len(prev) != len(transformLadder) {
		t.Fatalf("effort %d searches %d shapes, the ladder has %d",
			MaxEffort, len(prev), len(transformLadder))
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

func detailedImage(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewPCG(29, 31))

	noise := make([]float64, w*h)

	for i := range noise {
		noise[i] = rng.NormFloat64()
	}

	for y := range h {
		for x := range w {
			fx, fy := float64(x), float64(y)

			sky := 0.45 + 0.35*fy/float64(h)
			r, g, b := sky*0.8, sky*0.9, sky

			if fy > float64(h)*0.55 {
				n := noise[y*w+x]
				grass := 0.28 + 0.06*math.Sin(fx/2.7) + 0.05*math.Cos(fy/1.9) + 0.05*n

				r, g, b = grass*0.7, grass, grass*0.45
			}

			if d := math.Hypot(fx-float64(w)*0.3, fy-float64(h)*0.3); d < float64(h)*0.12 {
				r, g, b = 0.92, 0.86, 0.35
			}

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

func smoothImage(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)

			r := 0.35 + 0.5*fx
			g := 0.4 + 0.4*fy
			b := 0.6 - 0.3*fx*fy

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

func noiseImage(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewPCG(41, 43))

	for y := range h {
		for x := range w {
			m.SetNRGBA(x, y, color.NRGBA{
				R: uint8(60 + rng.IntN(160)),
				G: uint8(60 + rng.IntN(160)),
				B: uint8(60 + rng.IntN(160)),
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

// SSIMULACRA2_BIN, or PATH. Empty skips the tests that need it.
var ssimulacra2Bin = func() string {
	name := os.Getenv("SSIMULACRA2_BIN")
	if name == "" {
		name = "ssimulacra2"
	}

	bin, err := exec.LookPath(name)
	if err != nil {
		return ""
	}

	return bin
}()

func ssimulacra2Path(t *testing.T) string {
	t.Helper()

	if ssimulacra2Bin == "" {
		t.Skip("ssimulacra2 not found (set SSIMULACRA2_BIN)")
	}

	return ssimulacra2Bin
}

func ssimulacra2(t *testing.T, tool, dir string, src, got image.Image) float64 {
	t.Helper()

	paths := [2]string{filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")}

	for i, m := range [2]image.Image{src, got} {
		f, err := os.Create(paths[i])
		if err != nil {
			t.Fatal(err)
		}

		if err := png.Encode(f, m); err != nil {
			t.Fatal(err)
		}

		f.Close()
	}

	out, err := exec.Command(tool, paths[0], paths[1]).CombinedOutput()
	if err != nil {
		t.Fatalf("ssimulacra2 failed: %v\n%s", err, out)
	}

	var score float64

	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%g", &score); err != nil {
		t.Fatalf("ssimulacra2 printed %q", out)
	}

	return score
}

func encodeQuality(t *testing.T, src image.Image, q int, strength float32) (image.Image, int) {
	t.Helper()

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	data, err := encodeVarDCTImage(imageRows(src), w, h, encodeParams{
		distance: qualityToDistance(q),
		strength: strength,
	})
	if err != nil {
		t.Fatalf("quality %d: %v", q, err)
	}

	got, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("quality %d: decode gave %v", q, err)
	}

	return got, len(data)
}

type ratePoint struct {
	bytes int
	score float64
}

func interpolateScore(curve []ratePoint, size int) (float64, bool) {
	for i := 1; i < len(curve); i++ {
		lo, hi := curve[i-1], curve[i]

		if size < lo.bytes || size > hi.bytes {
			continue
		}

		if hi.bytes == lo.bytes {
			return lo.score, true
		}

		t := float64(size-lo.bytes) / float64(hi.bytes-lo.bytes)

		return lo.score + t*(hi.score-lo.score), true
	}

	return 0, false
}

func aqCurve(t *testing.T, tool, dir string, src image.Image, strength float32) []ratePoint {
	t.Helper()

	var curve []ratePoint

	for q := 30; q <= 95; q += 5 {
		got, size := encodeQuality(t, src, q, strength)
		curve = append(curve, ratePoint{size, ssimulacra2(t, tool, dir, src, got)})
	}

	slices.SortFunc(curve, func(a, b ratePoint) int { return cmp.Compare(a.bytes, b.bytes) })

	return curve
}

// rdFrontier keeps the points no other point beats on both size and score, so
// an interpolated comparison is against a monotone curve.
func rdFrontier(curve []ratePoint) []ratePoint {
	sorted := slices.Clone(curve)

	slices.SortFunc(sorted, func(a, b ratePoint) int {
		return cmp.Or(cmp.Compare(a.bytes, b.bytes), cmp.Compare(b.score, a.score))
	})

	var out []ratePoint

	best := math.Inf(-1)

	for _, p := range sorted {
		if p.score <= best {
			continue
		}

		best = p.score
		out = append(out, p)
	}

	return out
}

func aqDelta(flat, curve []ratePoint) (float64, int, int) {
	sum, wins, total := 0.0, 0, 0

	flat = rdFrontier(flat)

	for _, a := range rdFrontier(curve) {
		want, ok := interpolateScore(flat, a.bytes)
		if !ok {
			continue
		}

		total++
		sum += a.score - want

		if a.score > want {
			wins++
		}
	}

	if total == 0 {
		return 0, 0, 0
	}

	return sum / float64(total), wins, total
}

func aqCorpus() []struct {
	name string
	img  image.Image
} {
	return []struct {
		name string
		img  image.Image
	}{
		{"photo", detailedImage(512, 384)},
		{"textured", texturedImage(512, 512)},
		{"blocks", testImage(512, 384)},
		{"smooth", smoothImage(512, 384)},
		{"noise", noiseImage(384, 384)},
	}
}

// aqFloor is what adaptive quantisation may cost on any one corpus image.
const aqFloor = -0.35

func TestAdaptiveQuantCorpus(t *testing.T) {
	tool := ssimulacra2Path(t)
	dir := t.TempDir()

	total := 0.0

	for _, tc := range aqCorpus() {
		flat := aqCurve(t, tool, dir, tc.img, 0)
		mean, wins, n := aqDelta(flat, aqCurve(t, tool, dir, tc.img, adaptiveQuantStrength))

		total += mean

		t.Logf("%-9s %+.3f ssimulacra2 at matched size, wins %d of %d", tc.name, mean, wins, n)

		if mean < aqFloor {
			t.Fatalf("%s: adaptive quantisation costs %+.3f", tc.name, mean)
		}
	}

	mean := total / float64(len(aqCorpus()))

	t.Logf("corpus mean %+.3f at strength %.2f", mean, adaptiveQuantStrength)

	if mean <= 0 {
		t.Fatalf("adaptive quantisation scores %+.3f over the corpus", mean)
	}
}

func TestAdaptiveQuantStrengthSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("the sweep is slow")
	}

	tool := ssimulacra2Path(t)
	dir := t.TempDir()

	src := image.Image(detailedImage(320, 240))
	flat := aqCurve(t, tool, dir, src, 0)

	for _, strength := range []float32{0.15, 0.25, 0.35, 0.5, 1.0} {
		mean, wins, n := aqDelta(flat, aqCurve(t, tool, dir, src, strength))

		t.Logf("strength %.2f: %+.3f, wins %d of %d", strength, mean, wins, n)
	}
}

func TestRateAgainstCjxl(t *testing.T) {
	tool := ssimulacra2Path(t)

	cjxl, err := exec.LookPath("cjxl")
	if err != nil {
		t.Skip("cjxl is not on the path")
	}

	dir := t.TempDir()
	src := image.Image(detailedImage(640, 480))

	png0 := filepath.Join(dir, "src.png")

	f, err := os.Create(png0)
	if err != nil {
		t.Fatal(err)
	}

	if err := png.Encode(f, src); err != nil {
		t.Fatal(err)
	}

	f.Close()

	var theirs []ratePoint

	for _, d := range []string{"0.5", "1.0", "1.5", "2.0", "3.0", "4.0", "6.0"} {
		out := filepath.Join(dir, "cjxl-"+d+".jxl")

		if o, err := exec.Command(cjxl, png0, out, "-d", d, "--quiet").CombinedOutput(); err != nil {
			t.Fatalf("cjxl -d %s: %v\n%s", d, err, o)
		}

		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}

		got, err := Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("cjxl -d %s: %v", d, err)
		}

		theirs = append(theirs, ratePoint{len(data), ssimulacra2(t, tool, dir, src, got)})
	}

	slices.SortFunc(theirs, func(a, b ratePoint) int { return cmp.Compare(a.score, b.score) })

	worst := 0.0

	for q := 40; q <= 95; q += 5 {
		got, size := encodeQuality(t, src, q, adaptiveQuantStrength)
		score := ssimulacra2(t, tool, dir, src, got)

		want, ok := matchedSize(theirs, score)
		if !ok {
			t.Logf("q%d %6d bytes %.2f, outside the cjxl curve", q, size, score)

			continue
		}

		ratio := float64(size) / want

		t.Logf("q%d %6d bytes %.2f, cjxl needs %.0f (%.2fx)", q, size, score, want, ratio)

		if score >= 80 {
			worst = math.Max(worst, ratio)
		}
	}

	if worst > 1.15 {
		t.Fatalf("we are %.2fx cjxl above ssimulacra2 80, ratchet is 1.15x", worst)
	}
}

func matchedSize(curve []ratePoint, score float64) (float64, bool) {
	for i := 1; i < len(curve); i++ {
		lo, hi := curve[i-1], curve[i]

		if score < lo.score || score > hi.score {
			continue
		}

		r := 0.0
		if hi.score != lo.score {
			r = (score - lo.score) / (hi.score - lo.score)
		}

		return float64(lo.bytes) + r*float64(hi.bytes-lo.bytes), true
	}

	return 0, false
}
