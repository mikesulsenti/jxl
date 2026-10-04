package jxl

import (
	"bytes"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// RGB and alpha have independent precisions in a codestream. One 16-bit alpha
// level must survive even when the RGB channels are only 8 bit.
func TestIndependentAlphaPrecision(t *testing.T) {
	d := decoder{threads: 1}
	d.hdr.meta = imageMetadata{depth: defaultBitDepth(), color: defaultColorEncoding(), extraChannels: []extraChannelInfo{{ecType: ecAlpha, depth: bitDepth{bitsPerSample: 16}}}}
	f := Float{Width: 1, Height: 1, Channels: 4, Frames: [][]float32{{1, 0.5, 0, 1.0 / 65535}}}
	img, err := d.toImage(&f, 0)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := img.(*image.NRGBA64)
	if !ok {
		t.Fatalf("output type %T loses alpha precision", img)
	}
	if got := p.NRGBA64At(0, 0).A; got != 1 {
		t.Fatalf("alpha=%d want1", got)
	}
}

// The conformance file's associated float reference must be unassociated
// exactly once when converting to Go's straight-alpha image types.
func TestAssociatedAlphaImageReference(t *testing.T) {
	root := conformanceRoot(t)
	dir := filepath.Join(root, "testcases", "alpha_premultiplied")
	data, err := os.ReadFile(filepath.Join(dir, "input.jxl"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := readNPY(filepath.Join(dir, "reference_image.npy"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := Decode(bytes.NewReader(data), Options{Level: 10, Threads: 1})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := img.(*image.NRGBA64)
	if !ok {
		t.Fatalf("output type%T", img)
	}
	for y := range ref.height {
		for x := range ref.width {
			i := (y*ref.width + x) * ref.channels
			rgba := p.NRGBA64At(x, y)
			wantAlpha := ref.data[i+3]
			if math.Abs(float64(rgba.A)/65535-float64(wantAlpha)) > 1.0/65535 {
				t.Fatal("alpha mismatch")
			}
			if wantAlpha < 0.1 {
				continue
			} // float reconstruction error grows on division.
			for c, got := range []uint16{rgba.R, rgba.G, rgba.B} {
				want := clamp01(ref.data[i+c] / wantAlpha)
				if math.Abs(float64(got)/65535-float64(want)) > 6e-5 {
					t.Fatalf("pixel(%d,%d) channel%d got%g want%g", x, y, c, float64(got)/65535, want)
				}
			}
		}
	}
}
