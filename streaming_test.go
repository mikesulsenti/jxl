package jxl

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestStreamMatchesFloat(t *testing.T) {
	paths, _ := filepath.Glob("testdata/streams/*.jxl")
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want, err := DecodeFloat(bytes.NewReader(data), Options{Level: 10, Threads: 1})
			if err != nil {
				t.Fatal(err)
			}
			for _, threads := range []int{1, 4} {
				for _, planar := range []bool{false, true} {
					seen := 0
					info, err := DecodeFloatFrames(bytes.NewReader(data), func(f *FloatFrame) error {
						if f.Width != want.Width || f.Height != want.Height || f.Channels != want.Channels || f.Index != seen {
							t.Fatal("frame shape/index mismatch")
						}
						samples := f.Samples
						if planar {
							samples = make([]float32, f.Width*f.Height*f.Channels)
							for c, p := range f.Planes {
								for i, v := range p {
									samples[i*f.Channels+c] = v
								}
							}
						}
						if seen >= len(want.Frames) || !slices.Equal(samples, want.Frames[seen]) {
							t.Fatal("frame samples differ")
						}
						seen++
						return nil
					}, Options{Level: 10, Threads: threads, PlanarOutput: planar, MaxFrames: 64, MaxInputBytes: len(data), MaxWidth: 4096, MaxHeight: 4096, MaxOutputBytes: 64 << 20, MaxICCBytes: 1 << 20})
					if err != nil {
						t.Fatal(err)
					}
					if seen != len(want.Frames) || info.Width != want.Width || info.Height != want.Height {
						t.Fatal("stream count/shape mismatch")
					}
				}
			}
		})
	}
}

func TestStreamCallbackStops(t *testing.T) {
	data, err := os.ReadFile("testdata/streams/rgb_lossless.jxl")
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("callback stopped")
	_, err = DecodeFloatFrames(bytes.NewReader(data), func(*FloatFrame) error { return want })
	if err != want {
		t.Fatalf("callback error = %v", err)
	}
}

func TestBoundedDecoder(t *testing.T) {
	data, err := os.ReadFile("testdata/streams/dct_epf3.jxl")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for name, o := range map[string]Options{
		"input":    {MaxInputBytes: len(data) - 1},
		"width":    {MaxWidth: cfg.Width - 1},
		"height":   {MaxHeight: cfg.Height - 1},
		"output":   {MaxOutputBytes: cfg.Width*cfg.Height*3*4 - 1},
		"canvas":   {FrameSizeLimit: cfg.Width*cfg.Height - 1},
		"negative": {MaxFrames: -1},
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			_, err := DecodeFloatFrames(bytes.NewReader(data), func(*FloatFrame) error { calls++; return nil }, o)
			if !errors.Is(err, ErrLimit) && !errors.Is(err, ErrFrameTooLarge) {
				t.Fatalf("error = %v", err)
			}
			if calls != 0 {
				t.Fatalf("delivered %d frames before rejected allocation", calls)
			}
		})
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("reader panic") }
func TestPublicPanicBoundary(t *testing.T) {
	if _, err := DecodeFloat(panicReader{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("DecodeFloat error = %v", err)
	}
	if _, err := DecodeAll(panicReader{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("DecodeAll error = %v", err)
	}
	if _, err := DecodeConfig(panicReader{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("DecodeConfig error = %v", err)
	}
	if _, err := DecodeFloatFrames(panicReader{}, func(*FloatFrame) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("DecodeFloatFrames error = %v", err)
	}
	if err := overTasks(4, 8, func(int, int) error { panic("worker panic") }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("worker error = %v", err)
	}
	func() {
		var err error
		defer func() {
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("row worker error = %v", err)
			}
		}()
		defer recoverDecode(&err)
		overRows(4, 128, func(int, int) { panic("row panic") })
	}()
}

func TestLevel10Config(t *testing.T) {
	data, err := os.ReadFile("testdata/streams/rgb_lossless.jxl")
	if err != nil {
		t.Fatal(err)
	}
	boxed := bytes.Join([][]byte{[]byte(sigContainer), ftyp(), box("jxll", []byte{10}), box("jxlc", data)}, nil)
	if _, err := DecodeConfig(bytes.NewReader(boxed)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("baseline level5 error = %v", err)
	}
	if _, err := DecodeConfigWithOptions(bytes.NewReader(boxed), Options{Level: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfigWithOptions(bytes.NewReader(boxed), Options{Level: 10, MaxInputBytes: len(boxed)}); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeConfigWithOptions(bytes.NewReader(boxed), Options{Level: 10, MaxInputBytes: 2}); !errors.Is(err, ErrLimit) {
		t.Fatalf("input error = %v", err)
	}
}

func FuzzBoundedStream(f *testing.F) {
	for _, src := range []image.Image{image.NewGray(image.Rect(0, 0, 1, 1)), image.NewNRGBA(image.Rect(0, 0, 17, 13))} {
		var b bytes.Buffer
		if err := Encode(&b, src, EncodeOptions{Lossless: true}); err != nil {
			f.Fatal(err)
		}
		f.Add(b.Bytes())
	}
	for _, path := range []string{"testdata/streams/dct_small.jxl", "testdata/streams/rgb_lossless.jxl"} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 16<<10 {
			return
		}
		o := Options{Level: 10, Threads: 1, FrameSizeLimit: 64 * 64, MaxWidth: 64, MaxHeight: 64, MaxFrames: 8, MaxInputBytes: 16 << 10, MaxICCBytes: 4096, MaxOutputBytes: 1 << 20}
		calls := 0
		_, err := DecodeFloatFrames(bytes.NewReader(data), func(fr *FloatFrame) error {
			calls++
			if calls > 8 || fr.Width > 64 || fr.Height > 64 || len(fr.Samples) > 64*64*259 {
				t.Fatal("output violated limits")
			}
			return nil
		}, o)
		if err == nil && calls == 0 {
			t.Fatal("success without frames")
		}
	})
}

func twoFrameAnimation(t *testing.T) []byte {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 8, 7))
	for y := 0; y < 7; y++ {
		for x := 0; x < 8; x++ {
			src.SetNRGBA(x, y, color.NRGBA{uint8(x * 31), uint8(y * 35), 127, 255})
		}
	}
	var b bytes.Buffer
	if err := Encode(&b, src, EncodeOptions{Lossless: true}); err != nil {
		t.Fatal(err)
	}
	d, err := newBytesDecoder(b.Bytes(), options(nil))
	if err != nil {
		t.Fatal(err)
	}
	r := newBitReader(d.src.window())
	r.skip(d.used)
	f, err := d.readFrameHeader(r)
	if err != nil {
		t.Fatal(err)
	}
	payload := d.src.window()[r.pos()/8 : r.pos()/8+int(f.toc.offsets[len(f.toc.offsets)-1])]
	d.hdr.meta.haveAnimation = true
	d.hdr.meta.anim = animation{tpsNumerator: 100, tpsDenominator: 1, numLoops: 2}
	d.hdr.meta.orient = orRotate90CW
	var w bitWriter
	d.hdr.write(&w)
	for i := 0; i < 2; i++ {
		f.hdr.isLast = i == 1
		f.hdr.duration = 2
		f.hdr.write(&w, &d.hdr.meta, d.hdr.size.xsize, d.hdr.size.ysize)
		f.toc.write(&w)
		w.writeBytes(payload)
	}
	if err := w.err(); err != nil {
		t.Fatal(err)
	}
	return w.bytes()
}

func TestAnimationStreamBoundsAndOrientation(t *testing.T) {
	data := twoFrameAnimation(t)
	want, err := DecodeFloat(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, planar := range []bool{false, true} {
		calls := 0
		info, err := DecodeFloatFrames(bytes.NewReader(data), func(f *FloatFrame) error {
			if f.Delay != 0.02 || f.Info.Loops != 2 || f.Width != 7 || f.Height != 8 {
				t.Fatal("animation metadata mismatch")
			}
			samples := f.Samples
			if planar {
				samples = make([]float32, f.Width*f.Height*f.Channels)
				for c, p := range f.Planes {
					for i, v := range p {
						samples[i*f.Channels+c] = v
					}
				}
			}
			if !slices.Equal(samples, want.Frames[calls]) {
				t.Fatal("oriented frame differs")
			}
			calls++
			return nil
		}, Options{PlanarOutput: planar, MaxFrames: 2, Threads: 4})
		if err != nil || calls != 2 || info.Loops != 2 {
			t.Fatalf("result calls%d info%+v err%v", calls, info, err)
		}
	}
	calls := 0
	_, err = DecodeFloatFrames(bytes.NewReader(data), func(*FloatFrame) error { calls++; return nil }, Options{MaxFrames: 1})
	if !errors.Is(err, ErrLimit) || calls != 1 {
		t.Fatalf("frame limit calls%d err%v", calls, err)
	}
}
