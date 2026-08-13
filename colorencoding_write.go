package jxl

func packSigned(v int32) uint32 {
	return uint32(v)<<1 ^ uint32(v>>31)
}

func (c *customXY) write(w *bitWriter) {
	d := [4]u32dist{
		dBits(19), dBitsOff(19, 524288), dBitsOff(20, 1048576), dBitsOff(21, 2097152),
	}

	w.u32(packSigned(c.x), d[0], d[1], d[2], d[3])
	w.u32(packSigned(c.y), d[0], d[1], d[2], d[3])
}

func (c *customTF) write(w *bitWriter, cs colorSpace) {
	if cs == csXYB {
		return
	}

	w.bit(c.haveGamma)

	if c.haveGamma {
		w.bits(24, uint64(c.gamma))

		return
	}

	w.enum(uint32(c.tf))
}

func (c *colorEncoding) write(w *bitWriter) {
	if *c == defaultColorEncoding() {
		w.bit(true)

		return
	}

	w.bit(false)
	w.bit(c.wantICC)
	w.enum(uint32(c.space))

	if !c.wantICC && c.space != csXYB {
		w.enum(uint32(c.white))
	}

	if c.white == wpCustom {
		c.whiteXY.write(w)
	}

	if !c.wantICC && c.space != csXYB && c.space != csGray {
		w.enum(uint32(c.prim))
	}

	if c.prim == prCustom {
		for i := range c.customPrim {
			c.customPrim[i].write(w)
		}
	}

	if !c.wantICC {
		c.tf.write(w, c.space)
		w.enum(uint32(c.renderingIntent))
	}
}
