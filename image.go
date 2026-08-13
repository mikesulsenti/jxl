package jxl

import (
	"image"
	"math"
)

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}

	return v
}

func (d *decoder) renderSpot(f *Float, frame int) {
	m := &d.hdr.meta
	color := m.colorChannels()

	d.renderSpot(f, frame)
	src := f.Frames[frame]

	for i, e := range m.extraChannels {
		if e.ecType != ecSpotColor {
			continue
		}

		scale := e.spotColor[3]

		overRows(d.threads, f.Height, func(y0, y1 int) {
			for j := y0 * f.Width; j < y1*f.Width; j++ {
				o := j * f.Channels
				mix := scale * src[o+color+i]

				for c := range min(color, 3) {
					src[o+c] = mix*e.spotColor[c] + (1-mix)*src[o+c]
				}
			}
		})
	}
}

func (d *decoder) toImage(f *Float, frame int) (image.Image, error) {
	m := &d.hdr.meta
	gray := m.color.space == csGray
	alpha := m.alphaIndex()
	color := m.colorChannels()

	deep := m.depth.bitsPerSample > 8 || m.depth.floatSample

	rect := image.Rect(0, 0, f.Width, f.Height)
	src := f.Frames[frame]

	at := func(x, y, c int) float32 {
		return clamp01(src[(y*f.Width+x)*f.Channels+c])
	}

	alphaAt := func(x, y int) float32 {
		if alpha < 0 {
			return 1
		}

		return at(x, y, color+alpha)
	}

	switch {
	case gray && alpha < 0 && !deep:
		img := image.NewGray(rect)

		for y := range f.Height {
			for x := range f.Width {
				img.Pix[y*img.Stride+x] = uint8(math.Round(float64(at(x, y, 0)) * 255))
			}
		}

		return img, nil

	case gray && alpha < 0:
		img := image.NewGray16(rect)

		for y := range f.Height {
			for x := range f.Width {
				v := uint16(math.Round(float64(at(x, y, 0)) * 65535))
				img.Pix[y*img.Stride+x*2] = uint8(v >> 8)
				img.Pix[y*img.Stride+x*2+1] = uint8(v)
			}
		}

		return img, nil

	case !deep:
		img := image.NewNRGBA(rect)

		for y := range f.Height {
			for x := range f.Width {
				p := img.Pix[y*img.Stride+x*4:]

				for c := range 3 {
					s := c
					if gray {
						s = 0
					}

					p[c] = uint8(math.Round(float64(at(x, y, s)) * 255))
				}

				p[3] = uint8(math.Round(float64(alphaAt(x, y)) * 255))
			}
		}

		return img, nil
	}

	img := image.NewNRGBA64(rect)

	for y := range f.Height {
		for x := range f.Width {
			p := img.Pix[y*img.Stride+x*8:]

			for c := range 3 {
				s := c
				if gray {
					s = 0
				}

				v := uint16(math.Round(float64(at(x, y, s)) * 65535))
				p[c*2] = uint8(v >> 8)
				p[c*2+1] = uint8(v)
			}

			v := uint16(math.Round(float64(alphaAt(x, y)) * 65535))
			p[6] = uint8(v >> 8)
			p[7] = uint8(v)
		}
	}

	return img, nil
}
