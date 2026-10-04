package jxl

import (
	"image"
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
