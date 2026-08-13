// Package jxl decodes JPEG XL images.
package jxl

import (
	"errors"
	"image"
	"io"
)

// ErrInvalid means the bitstream is malformed. ErrUnsupported means it is
// well-formed but uses a feature this decoder does not implement.
var (
	ErrInvalid     = errors.New("jxl: invalid bitstream")
	ErrUnsupported = errors.New("jxl: unsupported feature")
)

// DefaultLevel is the codestream level enforced when Options.Level is zero.
const DefaultLevel = 5

// Options configure a decode.
type Options struct {
	// Level is the codestream level to accept. Zero means DefaultLevel.
	Level int
	// Threads bounds the goroutines decoding one frame. Zero means GOMAXPROCS,
	// one decodes serially.
	Threads int
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
func DecodeAll(r io.Reader, opts ...Options) (*JXL, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return decodeAll(data, options(opts))
}

// DecodeFloat reads a JPEG XL file into the decoder's native float samples,
// without converting to an image.Image.
func DecodeFloat(r io.Reader, opts ...Options) (*Float, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return decodeFloat(data, options(opts))
}

// DecodeConfig reads the dimensions and color model without decoding pixels.
func DecodeConfig(r io.Reader) (image.Config, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return image.Config{}, err
	}

	return decodeConfig(data)
}

func decode(r io.Reader) (image.Image, error) { return Decode(r) }

func init() {
	image.RegisterFormat("jxl", sigCodestream, decode, DecodeConfig)
	image.RegisterFormat("jxl", sigContainer, decode, DecodeConfig)
}
