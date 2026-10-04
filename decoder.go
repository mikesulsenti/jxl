package jxl

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"slices"
)

type refFrame struct {
	w, h     int
	planes   [][]float32
	beforeCT bool
}

type decoder struct {
	opts     Options
	threads  int
	hdr      fileHeader
	iccp     []byte
	arena    arena
	src      *source
	used     int
	refs     [4]*canvas
	preCT    [4]*refFrame
	lfFrames [4]*refFrame

	visible       int
	nonvisible    int
	preview       bool
	emit          func(*FloatFrame) error
	streamSamples []float32
	streamPlanes  [][]float32
	decodedFrames int
	outputBytes   uint64
}

func newDecoder(r io.Reader, o Options) (*decoder, error) {
	if o.MaxWidth < 0 || o.MaxHeight < 0 || o.MaxFrames < 0 || o.MaxInputBytes < 0 || o.MaxICCBytes < 0 || o.MaxOutputBytes < 0 {
		return nil, fmt.Errorf("%w: negative decode limit", ErrLimit)
	}
	if o.MaxInputBytes > 0 {
		r = &inputLimitedReader{r: r, remaining: int64(o.MaxInputBytes)}
	}
	return newSourceDecoder(newSource(r, nil), o, false)
}

type inputLimitedReader struct {
	r         io.Reader
	remaining int64
}

func (r *inputLimitedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.r.Read(extra[:])
		if n > 0 {
			return 0, fmt.Errorf("%w: compressed input", ErrLimit)
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.r.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func newBytesDecoder(data []byte, o Options) (*decoder, error) {
	return newSourceDecoder(newSource(nil, data), o, false)
}

func newSourceDecoder(s *source, o Options, partial bool) (*decoder, error) {
	s.need(len(sigContainer))

	if s.fail != nil {
		return nil, s.fail
	}

	boxed, skip, err := signature(s.window())
	if err != nil {
		return nil, err
	}

	d := &decoder{opts: o, threads: numThreads(o.Threads)}

	level := 5

	if boxed {
		data, err := s.all()
		if err != nil {
			return nil, err
		}

		c, err := readContainer(data[skip:], partial)
		if err != nil {
			return nil, err
		}

		level = c.level
		s = newSource(nil, c.codestream)
	}

	if level > d.opts.Level {
		return nil, ErrUnsupported
	}

	for {
		br := newBitReader(s.window())

		var hdr fileHeader

		hdr.read(br)

		err := br.err()
		if err == nil {
			d.hdr = hdr
			if !partial {
				if err := d.checkFrameSize(hdr.size.xsize, hdr.size.ysize); err != nil {
					return nil, err
				}
			}

			if !partial && hdr.meta.color.wantICC {
				encoded := readICCStreamLimited(br, d.opts.MaxICCBytes)

				if err = br.err(); err == nil {
					if d.opts.MaxICCBytes > 0 {
						s := iccStream{data: encoded}
						n, ok := s.varint()
						if ok && n > uint64(d.opts.MaxICCBytes) {
							return nil, fmt.Errorf("%w: ICC profile", ErrLimit)
						}
					}
					profile, ok := decodeICC(encoded)
					if !ok {
						return nil, ErrInvalid
					}

					d.iccp = profile
				}
			}
		}

		if err == nil {
			if !levelAllows(d.opts.Level, &d.hdr, len(d.iccp)) {
				return nil, ErrUnsupported
			}

			d.src = s
			d.used = br.pos()

			return d, nil
		}

		if !s.more() {
			if s.fail != nil {
				return nil, s.fail
			}

			return nil, err
		}
	}
}

func (d *decoder) checkFrameSize(w, h uint32) error {
	if d.opts.MaxWidth > 0 && uint64(w) > uint64(d.opts.MaxWidth) || d.opts.MaxHeight > 0 && uint64(h) > uint64(d.opts.MaxHeight) {
		return fmt.Errorf("%w: dimensions", ErrLimit)
	}
	channels := d.hdr.meta.colorChannels() + len(d.hdr.meta.extraChannels)
	if channels > 0 && uint64(w)*uint64(h) > uint64(math.MaxInt)/uint64(channels)/4 {
		return fmt.Errorf("%w: sample allocation", ErrLimit)
	}
	if d.opts.MaxOutputBytes > 0 && uint64(w)*uint64(h)*uint64(channels)*4 > uint64(d.opts.MaxOutputBytes) {
		return fmt.Errorf("%w: float frame size", ErrLimit)
	}
	lim := int64(d.opts.FrameSizeLimit)

	if lim == 0 {
		lim = DefaultFrameSizeLimit
	}

	if lim < 0 {
		return nil
	}

	if int64(w)*int64(h) > lim {
		return ErrFrameTooLarge
	}

	return nil
}

func levelAllows(level int, hdr *fileHeader, iccSize int) bool {
	x, y := uint64(hdr.size.xsize), uint64(hdr.size.ysize)
	icc := uint64(iccSize)
	channels := len(hdr.meta.extraChannels)

	if x > 1<<30 || y > 1<<30 || x*y > 1<<40 || icc > 1<<28 || channels > 256 {
		return false
	}

	if level >= 10 {
		return true
	}

	if x > 1<<18 || y > 1<<18 || x*y > 1<<28 || icc > 1<<22 || channels > 4 {
		return false
	}

	for i := range hdr.meta.extraChannels {
		if hdr.meta.extraChannels[i].ecType == ecBlack {
			return false
		}
	}

	return true
}

func decodeAll(r io.Reader, o Options) (*JXL, error) {
	d, err := newDecoder(r, o)
	if err != nil {
		return nil, err
	}

	fl, delays, loops, err := d.decodeFrames()
	if err != nil {
		return nil, err
	}

	out := &JXL{Delay: delays, Loops: loops, Color: d.colorInfo()}

	for i := range fl.Frames {
		img, err := d.toImage(fl, i)
		if err != nil {
			return nil, err
		}

		out.Image = append(out.Image, img)
	}

	return out, nil
}

func (d *decoder) colorInfo() ColorInfo {
	c := &d.hdr.meta.color

	info := ColorInfo{
		Primaries: uint16(c.prim),
		Transfer:  uint16(c.tf.tf),
		Matrix:    0,
		ICCP:      d.iccp,
	}

	if c.space == csGray {
		info.Matrix = 2
	}

	return info
}

func (d *decoder) streamInfo() StreamInfo {
	m := &d.hdr.meta
	w, h := int(d.hdr.size.xsize), int(d.hdr.size.ysize)
	if m.orient.transposing() {
		w, h = h, w
	}
	a := m.alphaIndex()
	assoc := false
	if a >= 0 {
		assoc = m.extraChannels[a].alphaAssociated
		a += m.colorChannels()
	}
	loops := 0
	if m.haveAnimation {
		loops = int(m.anim.numLoops)
	}
	info := StreamInfo{Width: w, Height: h, Channels: m.colorChannels() + len(m.extraChannels), BitDepth: int(m.depth.bitsPerSample), AlphaChannel: a, AlphaPremultiplied: assoc, XYBEncoded: m.xybEncoded, IntensityTarget: m.tone.intensityTarget, Loops: loops, Color: d.colorInfo(), WhitePoint: uint32(m.color.white)}
	if ai := m.alphaIndex(); ai >= 0 {
		info.BitDepth = max(info.BitDepth, int(m.extraChannels[ai].depth.bitsPerSample))
	}
	info.WhiteXY = [2]float64{float64(m.color.whiteXY.x) * 1e-6, float64(m.color.whiteXY.y) * 1e-6}
	for i, p := range m.color.customPrim {
		info.PrimariesXY[i] = [2]float64{float64(p.x) * 1e-6, float64(p.y) * 1e-6}
	}
	if m.color.tf.haveGamma {
		info.Gamma = float64(m.color.tf.gamma) * 1e-7
	}
	return info
}

func decodeFloat(r io.Reader, o Options) (*Float, error) {
	d, err := newDecoder(r, o)
	if err != nil {
		return nil, err
	}

	return d.floats()
}

func decodeFloatBytes(data []byte, o Options) (*Float, error) {
	d, err := newBytesDecoder(data, o)
	if err != nil {
		return nil, err
	}

	return d.floats()
}

func (d *decoder) floats() (*Float, error) {
	fl, _, _, err := d.decodeFrames()
	if err != nil {
		return nil, err
	}

	fl.Color = d.colorInfo()

	return fl, nil
}

func (d *decoder) skipPreview() error {
	d.preview = true
	defer func() { d.preview = false }()

	for {
		r := newBitReader(d.src.window())
		r.skip(d.used)

		f, err := d.readFrameHeader(r)
		if err == nil {
			end := r.pos()/8 + int(f.toc.offsets[len(f.toc.offsets)-1])

			if d.src.avail() >= end {
				d.used = end * 8
				d.src.advance(end)
				d.used = 0

				return nil
			}

			err = ErrInvalid
		}

		if !d.src.more() {
			if d.src.fail != nil {
				return d.src.fail
			}

			return err
		}
	}
}

func (d *decoder) decodeFrames() (*Float, []float64, int, error) {
	m := &d.hdr.meta

	if m.havePreview {
		if err := d.skipPreview(); err != nil {
			return nil, nil, 0, err
		}
	}
	channels := m.colorChannels() + len(m.extraChannels)
	w, h := int(d.hdr.size.xsize), int(d.hdr.size.ysize)

	b := d.newBlender()

	ow, oh := w, h
	if m.orient.transposing() {
		ow, oh = h, w
	}

	out := &Float{Width: ow, Height: oh, Channels: channels}

	var (
		delays []float64
		loops  int
	)

	if m.haveAnimation {
		loops = int(m.anim.numLoops)
	}

	for {
		if d.opts.MaxFrames > 0 && d.decodedFrames >= d.opts.MaxFrames {
			return nil, nil, 0, fmt.Errorf("%w: frame count", ErrLimit)
		}
		f, err := d.nextFrame()
		if err != nil {
			return nil, nil, 0, err
		}
		d.decodedFrames++

		if f.hdr.isVisible() {
			d.visible++
			d.nonvisible = 0
		} else {
			d.nonvisible++
		}

		planes, err := d.renderFrame(f, channels)
		if err != nil {
			return nil, nil, 0, err
		}

		if f.hdr.ftype == ftLF {
			continue
		}

		fw, fh := f.hdr.sizeUpsampled()

		colorSpec := specOf(f.hdr.blending)

		ecSpecs := make([]blendSpec, len(f.hdr.ecBlending))
		for i, e := range f.hdr.ecBlending {
			ecSpecs[i] = specOf(e)
		}

		var result *canvas

		if fw == w && fh == h && f.hdr.x0 == 0 && f.hdr.y0 == 0 &&
			allReplace(colorSpec, ecSpecs) {
			result = &canvas{w: w, h: h, channels: channels, planes: planes}
		} else {
			ref := d.refs[f.hdr.blending.source]
			blank := ref == nil

			if blank {
				ref = newCanvas(w, h, channels, &d.arena)
			}

			result = newCanvas(w, h, channels, &d.arena)

			b.blend(result, ref, planes, fw, fh,
				int(f.hdr.x0), int(f.hdr.y0), colorSpec, ecSpecs)

			if blank {
				d.arena.putAll(ref.planes)
			}

			d.arena.putAll(planes)
		}

		if f.hdr.canBeReferenced {
			old := d.refs[f.hdr.saveAsReference]
			d.refs[f.hdr.saveAsReference] = result
			d.release(old, result)
		}

		if f.hdr.isLast || f.hdr.duration > 0 {
			bytes := uint64(w) * uint64(h) * uint64(channels) * 4
			if d.opts.MaxOutputBytes > 0 && (bytes > uint64(d.opts.MaxOutputBytes) || d.outputBytes > uint64(d.opts.MaxOutputBytes)-bytes) {
				return nil, nil, 0, fmt.Errorf("%w: float output", ErrLimit)
			}
			d.outputBytes += bytes
			var interleaved []float32
			var displayPlanes [][]float32
			if d.emit != nil && d.opts.PlanarOutput {
				displayPlanes = result.planes
				if m.orient != orIdentity {
					if d.streamPlanes == nil {
						d.streamPlanes = make([][]float32, channels)
					}
					for c := range channels {
						if d.streamPlanes[c] == nil {
							d.streamPlanes[c] = make([]float32, w*h)
						}
					}
					displayPlanes = d.streamPlanes
					overRows(d.threads, h, func(y0, y1 int) {
						for y := y0; y < y1; y++ {
							for x := range w {
								ox, oy := m.orient.displayPixel(x, y, w, h)
								for c := range channels {
									displayPlanes[c][oy*ow+ox] = result.planes[c][y*w+x]
								}
							}
						}
					})
				}
			} else {
				if d.emit != nil {
					if cap(d.streamSamples) < w*h*channels {
						d.streamSamples = make([]float32, w*h*channels)
					}
					interleaved = d.streamSamples[:w*h*channels]
				} else {
					interleaved = make([]float32, w*h*channels)
				}

				if m.orient == orIdentity {
					overRows(d.threads, h, func(y0, y1 int) {
						lo, hi := y0*w, y1*w

						if channels == 1 {
							copy(interleaved[lo:hi], result.planes[0][lo:hi])

							return
						}

						for i := lo; i < hi; i++ {
							dst := interleaved[i*channels : (i+1)*channels]

							for c, p := range result.planes {
								dst[c] = p[i]
							}
						}
					})
				} else {
					overRows(d.threads, h, func(y0, y1 int) {
						for y := y0; y < y1; y++ {
							for x := range w {
								ox, oy := m.orient.displayPixel(x, y, w, h)
								dst := (oy*ow + ox) * channels

								for c := range channels {
									interleaved[dst+c] = result.planes[c][y*w+x]
								}
							}
						}
					})
				}
			}

			delay := f.hdr.delay(&m.anim)
			if d.emit != nil {
				frame := &FloatFrame{Width: ow, Height: oh, Channels: channels, Samples: interleaved, Planes: displayPlanes, Delay: delay, Index: len(delays), Info: d.streamInfo()}
				if err := d.emit(frame); err != nil {
					return nil, nil, 0, err
				}
			} else {
				out.Frames = append(out.Frames, interleaved)
			}
			delays = append(delays, delay)
		}

		d.release(result, nil)

		if f.hdr.isLast {
			break
		}
	}

	if len(delays) == 0 {
		return nil, nil, 0, ErrInvalid
	}

	return out, delays, loops, nil
}

func (d *decoder) nextFrame() (*frame, error) {
	var (
		f   *frame
		r   *bitReader
		err error
	)

	for {
		r = newBitReader(d.src.window())
		r.skip(d.used)

		if f, err = d.readFrameHeader(r); err == nil {
			if end := r.pos()/8 + int(f.toc.offsets[len(f.toc.offsets)-1]); d.src.avail() >= end {
				break
			}

			err = ErrInvalid
		}

		if !d.src.more() {
			if d.src.fail != nil {
				return nil, d.src.fail
			}

			return nil, err
		}
	}

	if err := d.decodeFrame(f, r); err != nil {
		return nil, err
	}

	d.used = r.pos()
	d.src.advance(d.used / 8)
	d.used %= 8

	return f, nil
}

func (d *decoder) release(c, keep *canvas) {
	if c == nil || c == keep {
		return
	}

	for _, r := range d.refs {
		if r == c {
			return
		}
	}

	d.arena.putAll(c.planes)
}

func (d *decoder) colorPlanes(f *frame) int {
	if d.hdr.meta.xybEncoded || f.vardct != nil {
		return 3
	}

	return d.hdr.meta.colorChannels()
}

func (d *decoder) renderFrame(f *frame, channels int) ([][]float32, error) {
	m := &d.hdr.meta
	cw, ch := f.hdr.size()
	w, h := f.hdr.sizeUpsampled()

	planes, err := d.toPlanes(f, cw, ch)
	if err != nil {
		return nil, err
	}

	color := d.colorPlanes(f)

	if g := &f.hdr.filter; g.gab {
		weights := [3][2]float32{
			{g.gabXWeight1, g.gabXWeight2},
			{g.gabYWeight1, g.gabYWeight2},
			{g.gabBWeight1, g.gabBWeight2},
		}

		scratch := d.arena.getDirty(cw * ch)

		for c := range min(color, 3) {
			out := gaborish(planes[c], scratch, cw, ch, d.threads, weights[c][0], weights[c][1])

			scratch, planes[c] = planes[c], out
		}

		d.arena.put(scratch)
	}

	if err := d.applyEPF(f, planes, cw, ch, color); err != nil {
		return nil, err
	}

	d.applyPatches(f, planes, cw, ch)

	if f.hdr.flags&flagSplines != 0 {
		f.splines.draw(planes, cw, ch)
	}

	if f.hdr.upsampling > 1 {
		planes = d.upsamplePlanes(f, planes, cw, ch, w, h)
	}

	if f.hdr.flags&flagNoise != 0 {
		d.addNoise(f, planes, w, h)
	}

	if f.hdr.ftype == ftLF {
		lf := &refFrame{w: w, h: h, planes: make([][]float32, 3)}

		for i := range 3 {
			lf.planes[i] = planes[i]
		}

		d.lfFrames[f.hdr.lfLevel-1] = lf

		return nil, nil
	}

	if f.hdr.canBeReferenced && f.hdr.saveBeforeCT {
		ref := &refFrame{w: w, h: h, beforeCT: true, planes: make([][]float32, len(planes))}

		for i, p := range planes {
			ref.planes[i] = make([]float32, len(p))
			copy(ref.planes[i], p)
		}

		d.preCT[f.hdr.saveAsReference] = ref
	}

	switch {
	case f.hdr.doYCbCr:
		ycbcrToRGB(planes, w, h, d.threads)
	case m.xybEncoded:
		if err := d.xybToRGB(f, planes, w, h); err != nil {
			return nil, err
		}
	}

	if len(planes) != channels {
		planes = append(planes[:m.colorChannels()], planes[color:]...)
	}

	return planes, nil
}

func (d *decoder) toPlanes(f *frame, w, h int) ([][]float32, error) {
	m := &d.hdr.meta
	color := d.colorPlanes(f)

	planes := make([][]float32, color+len(m.extraChannels))

	adopted := 0
	if f.vardct != nil {
		adopted = 3
	}

	for i := adopted; i < len(planes); i++ {
		planes[i] = d.arena.getDirty(w * h)
	}

	switch {
	case f.vardct != nil:
		v := f.vardct

		for c := range 3 {
			src := v.pixels[c]
			sw, sh := v.bw*blockDim>>v.hs[c], v.bh*blockDim>>v.vs[c]

			for range v.hs[c] {
				src = chromaUpsampleH(src, sw, sh)
				sw *= 2
			}

			for range v.vs[c] {
				src = chromaUpsampleV(src, sw, sh)
				sh *= 2
			}

			if sw != w {
				for y := 1; y < h; y++ {
					copy(src[y*w:(y+1)*w], src[y*sw:])
				}
			}

			planes[c] = src[:w*h]

			if &src[0] != &v.pixels[c][0] {
				d.arena.put(v.pixels[c])
			}

			v.pixels[c] = nil
		}
	case m.xybEncoded:
		if len(f.modular.channels) < 3 {
			return nil, ErrUnsupported
		}

		cy, cx, cb := f.modular.channels[0], f.modular.channels[1], f.modular.channels[2]
		if cy.w < w || cy.h < h || !cx.equivalent(cy) || !cb.equivalent(cy) {
			return nil, ErrUnsupported
		}

		s := f.lfQuant.factors

		overRows(d.threads, h, func(y0, y1 int) {
			for y := y0; y < y1; y++ {
				ry, rx, rb := cy.row(y), cx.row(y), cb.row(y)

				for x := range w {
					i := y*w + x
					planes[0][i] = float32(rx[x]) * s[0]
					planes[1][i] = float32(ry[x]) * s[1]
					planes[2][i] = (float32(rb[x]) + float32(ry[x])) * s[2]
				}
			}
		})
	}

	mbase := color
	if f.vardct != nil {
		mbase = 0
	}

	for c := range planes {
		var (
			ch    *modChannel
			depth bitDepth
		)

		if c < color {
			if f.vardct != nil || m.xybEncoded {
				continue
			}

			ch, depth = f.modular.channels[c], m.depth
		} else {
			ch, depth = f.modular.channels[mbase+c-color], m.extraChannels[c-color].depth
		}

		if ch.w < w || ch.h < h {
			return nil, ErrUnsupported
		}

		if depth.floatSample {
			if depth.bitsPerSample != 32 || m.xybEncoded {
				return nil, ErrUnsupported
			}

			overRows(d.threads, h, func(y0, y1 int) {
				for y := y0; y < y1; y++ {
					row := ch.row(y)

					for x := range w {
						planes[c][y*w+x] = math.Float32frombits(uint32(row[x]))
					}
				}
			})

			continue
		}

		scale := float32(1) / float32(uint64(1)<<depth.bitsPerSample-1)

		overRows(d.threads, h, func(y0, y1 int) {
			for y := y0; y < y1; y++ {
				row := ch.row(y)

				for x := range w {
					planes[c][y*w+x] = float32(row[x]) * scale
				}
			}
		})
	}

	return planes, nil
}

func (d *decoder) applyEPF(f *frame, planes [][]float32, w, h, color int) error {
	rf := &f.hdr.filter

	if rf.epfIters == 0 {
		return nil
	}

	e := epfPlanes{w: w, h: h, threads: d.threads, arena: &d.arena}

	switch {
	case color >= 3:
		e.planes = slices.Clone(planes[:3])
		e.scales = rf.epfChannelScale[:]
	case color == 1:
		e.planes = slices.Clone(planes[:1])
		e.scales = []float32{
			rf.epfChannelScale[0] + rf.epfChannelScale[1] + rf.epfChannelScale[2],
		}
	default:
		return ErrUnsupported
	}

	if f.vardct != nil {
		e.sigma, e.sw = f.vardct.sigmaImage(rf)
	} else {
		e.sw = divCeil(w, blockDim)
		e.sigma = make([]float32, e.sw*divCeil(h, blockDim))

		for i := range e.sigma {
			e.sigma[i] = epfInvSigmaNum / rf.epfSigmaForModular
		}
	}

	e.init()

	if rf.epfIters >= 3 {
		e.pass0(rf.epfPass0SigmaScale, rf.epfBorderSadMul)
	}

	e.pass1(1, rf.epfBorderSadMul)

	if rf.epfIters >= 2 {
		e.pass2(rf.epfPass2SigmaScale, rf.epfBorderSadMul)
	}

	copy(planes, e.planes)
	d.arena.putAll(e.out)

	return nil
}

func ycbcrToRGB(planes [][]float32, w, h, threads int) {
	const (
		c128  = 128.0 / 255
		crToR = 1.402
		crToG = -0.299 * 1.402 / 0.587
		cbToG = -0.114 * 1.772 / 0.587
		cbToB = 1.772
	)

	overRows(threads, h, func(y0, y1 int) {
		for i := y0 * w; i < y1*w; i++ {
			cb, y, cr := planes[0][i], planes[1][i]+c128, planes[2][i]

			planes[0][i] = cr*crToR + y
			planes[1][i] = cr*crToG + (cb*cbToG + y)
			planes[2][i] = cb*cbToB + y
		}
	})
}

func (d *decoder) xybToRGB(f *frame, planes [][]float32, w, h int) error {
	p := newXYBParams(&d.hdr.transform.opsin, d.hdr.meta.tone.intensityTarget)
	c := &d.hdr.meta.color
	gamut, identity, ok := c.srgbGamut()
	if !ok {
		return ErrUnsupported
	}

	n := w * h
	x, y, b := planes[0][:n], planes[1][:n], planes[2][:n]

	srgb := identity && !c.wantICC && !c.tf.haveGamma && c.tf.tf == tfSRGB

	var k *[29][8]float32

	var k512 *[29][16]float32

	switch {
	case srgb && xybSRGB512Asm != nil:
		k512 = p.srgbConsts512()
	case srgb && xybSRGBAsm != nil:
		k = p.srgbConsts()
	}

	overRows(d.threads, h, func(y0, y1 int) {
		lo, hi := y0*w, y1*w

		if k512 != nil {
			if m := (hi - lo) &^ 15; m > 0 {
				xybSRGB512Asm(&x[lo], &y[lo], &b[lo], m, k512)

				lo += m
			}
		}

		if k != nil {
			if m := (hi - lo) &^ 7; m > 0 {
				xybSRGBAsm(&x[lo], &y[lo], &b[lo], m, k)

				lo += m
			}
		}

		switch {
		case c.wantICC:
			for i := lo; i < hi; i++ {
				x[i], y[i], b[i] = p.apply(x[i], y[i], b[i])
			}
		case srgb:
			for i := lo; i < hi; i++ {
				r, g, bb := p.apply(x[i], y[i], b[i])

				x[i] = linearToSRGB(r)
				y[i] = linearToSRGB(g)
				b[i] = linearToSRGB(bb)
			}
		default:
			for i := lo; i < hi; i++ {
				r, g, bb := p.apply(x[i], y[i], b[i])
				if !identity {
					r, g, bb = gamut.apply(r, g, bb)
				}

				x[i] = c.fromLinear(r)
				y[i] = c.fromLinear(g)
				b[i] = c.fromLinear(bb)
			}
		}
	})
	return nil
}

func decodeConfig(data []byte) (image.Config, error) {
	return decodeConfigOptions(data, options(nil))
}

func decodeConfigOptions(data []byte, o Options) (image.Config, error) {
	d, err := newSourceDecoder(newSource(nil, data), o, true)
	if err != nil {
		return image.Config{}, err
	}
	if err := d.checkFrameSize(d.hdr.size.xsize, d.hdr.size.ysize); err != nil {
		return image.Config{}, err
	}

	w, h := int(d.hdr.size.xsize), int(d.hdr.size.ysize)
	if d.hdr.meta.orient.transposing() {
		w, h = h, w
	}

	model := color.Model(color.NRGBAModel)
	if d.hdr.meta.depth.bitsPerSample > 8 || d.hdr.meta.depth.floatSample {
		model = color.NRGBA64Model
	}

	return image.Config{Width: w, Height: h, ColorModel: model}, nil
}
