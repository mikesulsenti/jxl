package jxl

type canvas struct {
	w, h     int
	channels int
	planes   [][]float32
}

func newCanvas(w, h, channels int, a *arena) *canvas {
	c := &canvas{w: w, h: h, channels: channels, planes: make([][]float32, channels)}
	for i := range c.planes {
		c.planes[i] = a.get(w * h)
	}

	return c
}

func maybeClamp(v float32, clamp bool) float32 {
	if !clamp {
		return v
	}

	return min(max(v, 0), 1)
}

type blendSpec struct {
	mode         blendMode
	alphaChannel int
	clamp        bool
}

func allReplace(color blendSpec, ec []blendSpec) bool {
	if color.mode != bmReplace {
		return false
	}

	for _, s := range ec {
		if s.mode != bmReplace {
			return false
		}
	}

	return true
}

func specOf(b blendingInfo) blendSpec {
	return blendSpec{mode: b.mode, alphaChannel: int(b.alphaChannel), clamp: b.clamp}
}

type blender struct {
	nc      int
	numEC   int
	alphaOf []bool
	assoc   []bool
}

func (d *decoder) newBlender() blender {
	m := &d.hdr.meta
	b := blender{nc: m.colorChannels(), numEC: len(m.extraChannels)}

	b.alphaOf = make([]bool, b.numEC)
	b.assoc = make([]bool, b.numEC)

	for i, ec := range m.extraChannels {
		b.alphaOf[i] = ec.ecType == ecAlpha
		b.assoc[i] = ec.alphaAssociated
	}

	return b
}

func (b *blender) hasAlpha() bool {
	for _, a := range b.alphaOf {
		if a {
			return true
		}
	}

	return false
}

func (b *blender) blend(out, ref *canvas, frame [][]float32, fw, fh, x0, y0 int,
	color blendSpec, ec []blendSpec,
) {
	for i := range out.planes {
		copy(out.planes[i], ref.planes[i])
	}

	topAlpha := make([]float32, fw)

	for y := range fh {
		dy := y0 + y
		if dy < 0 || dy >= out.h {
			continue
		}

		for x := range fw {
			topAlpha[x] = 0
		}

		for i := range b.numEC {
			if !b.alphaOf[i] {
				continue
			}

			for x := range fw {
				topAlpha[x] = frame[b.nc+i][y*fw+x]
			}

			break
		}

		for x := range fw {
			dx := x0 + x
			if dx < 0 || dx >= out.w {
				continue
			}

			di, si := dy*out.w+dx, y*fw+x

			for i := range b.numEC {
				b.blendEC(out, ref, frame, di, si, i, ec[i], topAlpha[x])
			}

			b.blendColor(out, ref, frame, di, si, color, topAlpha[x])
		}
	}
}

func (b *blender) blendEC(out, ref *canvas, frame [][]float32, di, si, i int,
	spec blendSpec, topA float32,
) {
	c := b.nc + i
	top := frame[c][si]
	bottom := ref.planes[c][di]
	a := spec.alphaChannel

	switch spec.mode {
	case bmReplace:
		out.planes[c][di] = top
	case bmAdd:
		out.planes[c][di] = top + bottom
	case bmMul:
		out.planes[c][di] = top * maybeClamp(bottom, spec.clamp)
	case bmAlphaWeightedAdd:
		if a >= b.numEC {
			return
		}

		if i == a {
			out.planes[c][di] = bottom

			return
		}

		out.planes[c][di] = bottom + top*maybeClamp(topA, spec.clamp)
	case bmBlend:
		if a >= b.numEC {
			return
		}

		ta := maybeClamp(topA, spec.clamp)
		ba := ref.planes[b.nc+a][di]

		if i == a {
			out.planes[c][di] = 1 - (1-ta)*(1-ba)

			return
		}

		if b.assoc[a] {
			out.planes[c][di] = top + bottom*(1-ta)

			return
		}

		newA := 1 - (1-ta)*(1-ba)

		recip := float32(0)
		if newA > 0 {
			recip = 1 / newA
		}

		out.planes[c][di] = (top*ta + bottom*ba*(1-ta)) * recip
	}
}

func (b *blender) blendColor(out, ref *canvas, frame [][]float32, di, si int,
	spec blendSpec, topA float32,
) {
	a := spec.alphaChannel

	switch spec.mode {
	case bmReplace:
		for c := range b.nc {
			out.planes[c][di] = frame[c][si]
		}
	case bmAdd:
		for c := range b.nc {
			out.planes[c][di] = frame[c][si] + ref.planes[c][di]
		}
	case bmMul:
		for c := range b.nc {
			out.planes[c][di] = frame[c][si] * maybeClamp(ref.planes[c][di], spec.clamp)
		}
	case bmAlphaWeightedAdd:
		for c := range b.nc {
			if !b.hasAlpha() || a >= b.numEC {
				out.planes[c][di] = frame[c][si] + ref.planes[c][di]

				continue
			}

			out.planes[c][di] = ref.planes[c][di] + frame[c][si]*maybeClamp(topA, spec.clamp)
		}
	case bmBlend:
		if !b.hasAlpha() || a >= b.numEC {
			for c := range b.nc {
				out.planes[c][di] = frame[c][si]
			}

			return
		}

		ta := maybeClamp(topA, spec.clamp)
		ba := ref.planes[b.nc+a][di]
		newA := 1 - (1-ta)*(1-ba)

		recip := float32(0)
		if newA > 0 {
			recip = 1 / newA
		}

		for c := range b.nc {
			top, bottom := frame[c][si], ref.planes[c][di]

			if b.assoc[a] {
				out.planes[c][di] = top + bottom*(1-ta)
			} else {
				out.planes[c][di] = (top*ta + bottom*ba*(1-ta)) * recip
			}
		}

		out.planes[b.nc+a][di] = newA
	}
}
