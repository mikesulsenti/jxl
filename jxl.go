// Package jxl decodes and encodes JPEG XL images.
//
// Decoding covers both Modular and VarDCT codestreams. Encoding writes a
// single frame from sRGB input, lossy VarDCT by default and Modular when
// [EncodeOptions.Lossless] is set. An alpha channel is always kept losslessly.
//
// [DecodeExif] parses the Exif metadata of a file, while [RawExif] and
// [RawXMP] return the payloads whole. All three report [ErrNoExif] or
// [ErrNoXMP] when the file carries no such box, and [ErrUnsupported] when it
// carries one Brotli compressed.
package jxl

import (
	"errors"
	"fmt"
	"image"
	"io"
	"math"
)

// ErrInvalid means the bitstream is malformed. ErrUnsupported means it is
// well-formed but uses a feature this decoder does not implement.
var (
	ErrInvalid     = errors.New("jxl: invalid bitstream")
	ErrUnsupported = errors.New("jxl: unsupported feature")
)

// ErrFrameTooLarge is returned when a frame's area exceeds
// Options.FrameSizeLimit.
var ErrFrameTooLarge = errors.New("jxl: frame exceeds size limit")

// ErrLimit means a caller-supplied input, frame, dimension or output bound
// was exceeded. FrameSizeLimit retains its existing ErrFrameTooLarge result.
var ErrLimit = errors.New("jxl: decode limit exceeded")

// DefaultLevel is the codestream level enforced when Options.Level is zero.
const DefaultLevel = 5

// DefaultFrameSizeLimit is the frame area, in pixels, accepted when
// Options.FrameSizeLimit is zero.
const DefaultFrameSizeLimit = min(16384*16384, math.MaxInt>>6)

// Options configure a decode.
type Options struct {
	// Level is the codestream level to accept. Zero means DefaultLevel.
	Level int
	// Threads bounds the goroutines decoding one frame. Zero means GOMAXPROCS,
	// one decodes serially.
	Threads int
	// FrameSizeLimit bounds a frame's area in pixels, so that a corrupt or
	// hostile header cannot ask for an unbounded allocation. Zero means
	// DefaultFrameSizeLimit; a negative value removes the limit.
	FrameSizeLimit int
	// MaxWidth and MaxHeight bound both the canvas and internal frame sizes.
	// Zero leaves these bounds disabled; negative values are invalid.
	MaxWidth, MaxHeight int
	// MaxFrames bounds all decoded frames, including invisible references and
	// low-frequency frames. Zero leaves the bound disabled.
	MaxFrames int
	// MaxInputBytes bounds compressed bytes read. Zero leaves it disabled.
	MaxInputBytes int
	// MaxICCBytes bounds both the decoded and entropy-coded ICC data before
	// allocating them. Zero leaves this caller bound disabled.
	MaxICCBytes int
	// MaxOutputBytes bounds cumulative displayed float samples, including
	// callbacks, before allocating another frame. Zero leaves it disabled.
	MaxOutputBytes int
}

func options(opts []Options) Options {
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Level == 0 {
		o.Level = DefaultLevel
	}

	return o
}

// ColorInfo reports the color space of the decoded samples, as CICP code
// points or as the ICC profile the codestream carried.
type ColorInfo struct {
	Primaries uint16
	Transfer  uint16
	Matrix    uint16
	ICCP      []byte
}

// JXL is every frame of a file, with the delay of each in seconds.
type JXL struct {
	Image []image.Image
	Delay []float64
	Loops int
	Color ColorInfo
}

// Float is the decoder's native output: interleaved float32 samples per frame,
// in the image's own color space.
type Float struct {
	Width    int
	Height   int
	Channels int
	// Frames each hold Width*Height*Channels samples, row-major.
	Frames [][]float32
	Color  ColorInfo
}

// StreamInfo describes the samples delivered by DecodeFloatFrames. For XYB
// with an embedded ICC, samples are linear sRGB; for non-XYB ICC they are in
// the profile's encoded sample space. The profile is not applied here.
type StreamInfo struct {
	Width, Height, Channels int
	BitDepth, AlphaChannel  int
	AlphaPremultiplied      bool
	XYBEncoded              bool
	IntensityTarget         float32
	WhitePoint              uint32
	WhiteXY                 [2]float64
	PrimariesXY             [3][2]float64
	Gamma                   float64
	Loops                   int
	Color                   ColorInfo
}

// FloatFrame is one composited, orientation-correct displayed frame.
// Samples are borrowed, read-only, and valid only until the callback returns.
type FloatFrame struct {
	Width, Height, Channels int
	Samples                 []float32
	Delay                   float64
	Index                   int
	Info                    StreamInfo
}

// DecodeFloatFrames decodes displayed frames one at a time. It retains only
// format-required reference frames, not every displayed frame. Callback
// errors stop decoding and are returned unchanged.
func DecodeFloatFrames(r io.Reader, emit func(*FloatFrame) error, opts ...Options) (info *StreamInfo, err error) {
	defer recoverDecode(&err)
	if emit == nil {
		return nil, fmt.Errorf("%w: nil frame callback", ErrInvalid)
	}
	d, err := newDecoder(r, options(opts))
	if err != nil {
		return nil, err
	}
	d.emit = emit
	_, _, _, err = d.decodeFrames()
	if err != nil {
		return nil, err
	}
	v := d.streamInfo()
	return &v, nil
}

// Worker panics are propagated to the calling goroutine by threading helpers,
// so this boundary also contains bounds-check panics from parallel decoding.
func recoverDecode(err *error) {
	if recover() != nil {
		*err = fmt.Errorf("%w: decoder panic", ErrInvalid)
	}
}

// Decode reads a JPEG XL image as *image.NRGBA, or *image.NRGBA64 above 8
// bits. Grayscale without alpha gives *image.Gray or *image.Gray16.
func Decode(r io.Reader, opts ...Options) (image.Image, error) {
	img, _, err := DecodeColor(r, opts...)

	return img, err
}

// DecodeColor reads a JPEG XL image and reports the color space it came from.
func DecodeColor(r io.Reader, opts ...Options) (image.Image, ColorInfo, error) {
	a, err := DecodeAll(r, opts...)
	if err != nil {
		return nil, ColorInfo{}, err
	}

	return a.Image[0], a.Color, nil
}

// DecodeAll reads every frame of a JPEG XL file. A still image gives one frame.
func DecodeAll(r io.Reader, opts ...Options) (out *JXL, err error) {
	defer recoverDecode(&err)
	return decodeAll(r, options(opts))
}

// DecodeFloat reads a JPEG XL file into the decoder's native float samples,
// without converting to an image.Image.
func DecodeFloat(r io.Reader, opts ...Options) (out *Float, err error) {
	defer recoverDecode(&err)
	return decodeFloat(r, options(opts))
}

const configChunk = 1 << 12

// DecodeConfig reads the dimensions and color model without decoding pixels.
func DecodeConfig(r io.Reader) (cfg image.Config, err error) {
	return DecodeConfigWithOptions(r)
}

// DecodeConfigWithOptions reads a header with the caller's codestream level
// and limits. DecodeConfig retains its existing default level-5 behaviour.
func DecodeConfigWithOptions(r io.Reader, opts ...Options) (cfg image.Config, err error) {
	defer recoverDecode(&err)
	o := options(opts)
	if o.MaxInputBytes > 0 {
		r = &inputLimitedReader{r: r, remaining: int64(o.MaxInputBytes)}
	}
	var buf []byte

	for size := configChunk; ; size *= 4 {
		have := len(buf)
		if o.MaxInputBytes > 0 {
			size = min(size, o.MaxInputBytes)
			if size <= have {
				return image.Config{}, fmt.Errorf("%w: configuration header", ErrLimit)
			}
		}

		buf = append(buf, make([]byte, size-have)...)
		n, err := io.ReadFull(r, buf[have:])
		buf = buf[:have+n]

		cfg, cerr := decodeConfigOptions(buf, o)
		if cerr == nil {
			return cfg, nil
		}

		if err != nil {
			return image.Config{}, cerr
		}
	}
}

func decode(r io.Reader) (image.Image, error) { return Decode(r) }

func init() {
	image.RegisterFormat("jxl", sigCodestream, decode, DecodeConfig)
	image.RegisterFormat("jxl", sigContainer, decode, DecodeConfig)
}
