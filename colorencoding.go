package jxl

type colorSpace uint32

const (
	csRGB colorSpace = iota
	csGray
	csXYB
	csUnknown
)

type whitePoint uint32

const (
	wpD65    whitePoint = 1
	wpCustom whitePoint = 2
	wpE      whitePoint = 10
	wpDCI    whitePoint = 11
)

type primaries uint32

const (
	prSRGB   primaries = 1
	prCustom primaries = 2
	prBT2100 primaries = 9
	prP3     primaries = 11
)

type transferFunction uint32

const (
	tfBT709   transferFunction = 1
	tfUnknown transferFunction = 2
	tfLinear  transferFunction = 8
	tfSRGB    transferFunction = 13
	tfPQ      transferFunction = 16
	tfDCI     transferFunction = 17
	tfHLG     transferFunction = 18
)

type renderingIntent uint32

const (
	riPerceptual renderingIntent = iota
	riRelative
	riSaturation
	riAbsolute
)

const gammaMul = 10000000

type customXY struct {
	x int32
	y int32
}

func (c *customXY) read(r *bitReader) {
	d := [4]u32dist{
		dBits(19), dBitsOff(19, 524288), dBitsOff(20, 1048576), dBitsOff(21, 2097152),
	}

	c.x = unpackSigned(r.u32(d[0], d[1], d[2], d[3]))
	c.y = unpackSigned(r.u32(d[0], d[1], d[2], d[3]))
}

type customTF struct {
	haveGamma bool
	gamma     uint32
	tf        transferFunction
}

func defaultCustomTF() customTF {
	return customTF{gamma: gammaMul / 3, tf: tfSRGB}
}

func (c *customTF) read(r *bitReader, cs colorSpace) {
	*c = defaultCustomTF()

	if cs == csXYB {
		c.haveGamma = true

		return
	}

	c.haveGamma = r.bit()

	if c.haveGamma {
		c.gamma = uint32(r.bits(24))

		if c.gamma > gammaMul || uint64(c.gamma)*maxGamma < gammaMul {
			r.setErr(ErrInvalid)
		}

		return
	}

	c.tf = transferFunction(r.enum())

	switch c.tf {
	case tfBT709, tfUnknown, tfLinear, tfSRGB, tfPQ, tfDCI, tfHLG:
	default:
		r.setErr(ErrInvalid)
	}
}

const maxGamma = 8192

func (c customTF) gammaValue() float32 { return float32(c.gamma) * 1e-7 }

type colorEncoding struct {
	wantICC         bool
	space           colorSpace
	white           whitePoint
	whiteXY         customXY
	prim            primaries
	customPrim      [3]customXY
	tf              customTF
	renderingIntent renderingIntent
}

func defaultColorEncoding() colorEncoding {
	return colorEncoding{
		space:           csRGB,
		white:           wpD65,
		prim:            prSRGB,
		tf:              defaultCustomTF(),
		renderingIntent: riRelative,
	}
}

func (c *colorEncoding) read(r *bitReader) {
	*c = defaultColorEncoding()

	if r.bit() {
		return
	}

	c.wantICC = r.bit()
	c.space = colorSpace(r.enum())

	switch c.space {
	case csRGB, csGray, csXYB, csUnknown:
	default:
		r.setErr(ErrInvalid)

		return
	}

	if !c.wantICC && c.space != csXYB {
		c.white = whitePoint(r.enum())

		switch c.white {
		case wpD65, wpCustom, wpE, wpDCI:
		default:
			r.setErr(ErrInvalid)

			return
		}
	}

	if c.white == wpCustom {
		c.whiteXY.read(r)
	}

	if !c.wantICC && c.space != csXYB && c.space != csGray {
		c.prim = primaries(r.enum())

		switch c.prim {
		case prSRGB, prCustom, prBT2100, prP3:
		default:
			r.setErr(ErrInvalid)

			return
		}
	}

	if c.prim == prCustom {
		for i := range c.customPrim {
			c.customPrim[i].read(r)
		}
	}

	if !c.wantICC {
		c.tf.read(r, c.space)
		c.renderingIntent = renderingIntent(r.enum())

		if c.renderingIntent > riAbsolute {
			r.setErr(ErrInvalid)
		}
	}
}

func (c *colorEncoding) check(r *bitReader) {
	if c.space == csUnknown || c.space == csXYB || c.tf.tf == tfUnknown {
		r.setErr(ErrInvalid)
	}
}
