package jxl

import (
	"math"
	"slices"
)

const (
	splineQuantAdjustmentContext = iota
	splineStartingPositionContext
	splineNumSplinesContext
	splineNumControlPointsContext
	splineControlPointsContext
	splineDCTContext
	numSplineContexts
)

const (
	maxNumControlPoints  = 1 << 20
	splineDeltaLimit     = 1 << 30
	splinePosLimit       = 1 << 23
	splineRenderDistance = 1
)

var splineChannelWeight = [4]float32{0.0042, 0.075, 0.07, 0.3333}

type point struct{ x, y float32 }

func (p point) add(o point) point   { return point{p.x + o.x, p.y + o.y} }
func (p point) sub(o point) point   { return point{p.x - o.x, p.y - o.y} }
func (p point) mul(s float32) point { return point{p.x * s, p.y * s} }
func (p point) div(s float32) point { return point{p.x * (1 / s), p.y * (1 / s)} }

func (p point) abs() float32 {
	return float32(math.Hypot(float64(p.x), float64(p.y)))
}

func fastCos(x float32) float32 {
	const (
		pi2    = math.Pi * 2
		pi2inv = 0.5 / math.Pi
	)

	npi2 := float32(math.Floor(float64(x*pi2inv))) * pi2
	xmod := x - npi2

	xpi := min(xmod, pi2-xmod)

	above := xpi >= math.Pi/2
	if above {
		xpi = math.Pi - xpi
	}

	xs := xpi * 0.25
	x2 := xs * xs
	x4 := x2 * x2

	c := x4*0.06960438 + (x2*-0.84087373 + 1.68179268)
	c = c*c - float32(math.Sqrt2)
	c = c*c - 1

	if above {
		return -c
	}

	return c
}

func fastErff(x float32) float32 {
	a := absF(x)

	d := a*7.77394369e-02 + 2.05260015e-04
	d = d*a + 2.32120216e-01
	d = d*a + 2.77820801e-01
	d = d*a + 1
	d *= d

	inv := 1 / d
	r := -inv*inv + 1

	if x < 0 {
		return -r
	}

	return r
}

type quantizedSpline struct {
	controlPoints [][2]int64
	colorDCT      [3][32]int32
	sigmaDCT      [32]int32
}

type spline struct {
	controlPoints []point
	colorDCT      [3][32]float32
	sigmaDCT      [32]float32
	estimatedArea uint64
}

type splineSegment struct {
	centerX, centerY float32
	maxDistance      float32
	invSigma         float32
	sigmaIntensity   float32
	color            [3]float32
}

type splines struct {
	quantAdjustment int32
	quantized       []quantizedSpline
	startingPoints  []point
	segments        []splineSegment
	segmentIndices  []int
	segmentYStart   []int
}

func splineAreaLimit(size uint64) uint64 {
	v := size * 1024
	if v/1024 != size {
		return 1 << 42
	}

	return min(v+(1<<32), 1<<42)
}

func invAdjustedQuant(adjustment int32) float32 {
	if adjustment >= 0 {
		return 1 / (1 + 0.125*float32(adjustment))
	}

	return 1 - 0.125*float32(adjustment)
}

func splinePosValid(x, y int32) bool {
	return x >= -splinePosLimit && x < splinePosLimit &&
		y >= -splinePosLimit && y < splinePosLimit
}

func (s *splines) read(r *bitReader, numPixels uint32) {
	*s = splines{}

	var h histograms

	h.read(r, numSplineContexts, true)

	if r.err() != nil {
		return
	}

	var sr symbolReader

	sr.init(&h, r, 0)

	num := sr.readUnsigned(&h, r, splineNumSplinesContext) + 1

	maxControl := uint32(maxNumControlPoints)
	if numPixels/2 < maxControl {
		maxControl = numPixels / 2
	}

	if num > maxControl {
		r.setErr(ErrInvalid)

		return
	}

	var lastX, lastY int64

	for i := range num {
		ux := sr.readUnsigned(&h, r, splineStartingPositionContext)
		uy := sr.readUnsigned(&h, r, splineStartingPositionContext)

		if r.err() != nil {
			return
		}

		x, y := int64(ux), int64(uy)
		if i != 0 {
			x = int64(unpackSigned(ux)) + lastX
			y = int64(unpackSigned(uy)) + lastY
		}

		if max(absInt64(x), absInt64(y)) >= splinePosLimit {
			r.setErr(ErrInvalid)

			return
		}

		s.startingPoints = append(s.startingPoints, point{float32(x), float32(y)})
		lastX, lastY = x, y
	}

	s.quantAdjustment = sr.readSigned(&h, r, splineQuantAdjustmentContext)

	total := uint32(0)

	for range num {
		var q quantizedSpline

		n := sr.readUnsigned(&h, r, splineNumControlPointsContext)

		total += n
		if total > maxControl {
			r.setErr(ErrInvalid)

			return
		}

		for range n {
			x := int64(sr.readSigned(&h, r, splineControlPointsContext))
			y := int64(sr.readSigned(&h, r, splineControlPointsContext))

			if max(absInt64(x), absInt64(y)) >= splineDeltaLimit {
				r.setErr(ErrInvalid)

				return
			}

			q.controlPoints = append(q.controlPoints, [2]int64{x, y})
		}

		for c := range 3 {
			for i := range q.colorDCT[c] {
				q.colorDCT[c][i] = sr.readSigned(&h, r, splineDCTContext)
			}
		}

		for i := range q.sigmaDCT {
			q.sigmaDCT[i] = sr.readSigned(&h, r, splineDCTContext)
		}

		if r.err() != nil {
			return
		}

		s.quantized = append(s.quantized, q)
	}

	if err := sr.checkFinalState(&h, r); err != nil {
		r.setErr(err)
	}
}

func (q *quantizedSpline) dequantize(start point, adjustment int32,
	ytox, ytob float32, imageArea uint64,
) (*spline, bool) {
	limit := splineAreaLimit(imageArea)

	out := &spline{}

	px := int32(math.Round(float64(start.x)))
	py := int32(math.Round(float64(start.y)))

	if !splinePosValid(px, py) {
		return nil, false
	}

	cx, cy := px, py
	out.controlPoints = append(out.controlPoints, point{float32(cx), float32(cy)})

	var dx, dy int32

	manhattan := uint64(0)

	for _, d := range q.controlPoints {
		dx += int32(d[0])
		dy += int32(d[1])

		if !splinePosValid(dx, dy) {
			return nil, false
		}

		manhattan += uint64(absInt32(dx)) + uint64(absInt32(dy))
		if manhattan > limit {
			return nil, false
		}

		cx += dx
		cy += dy

		if !splinePosValid(cx, cy) {
			return nil, false
		}

		out.controlPoints = append(out.controlPoints, point{float32(cx), float32(cy)})
	}

	for i := range len(out.controlPoints) - 1 {
		a, b := out.controlPoints[i], out.controlPoints[i+1]

		if absF(a.x-b.x) < 1e-3 && absF(a.y-b.y) < 1e-3 {
			return nil, false
		}
	}

	invQuant := invAdjustedQuant(adjustment)

	for c := range 3 {
		for i := range 32 {
			f := float32(1)
			if i == 0 {
				f = float32(1 / math.Sqrt2)
			}

			out.colorDCT[c][i] = float32(q.colorDCT[c][i]) * f * splineChannelWeight[c] * invQuant
		}
	}

	for i := range 32 {
		out.colorDCT[0][i] += ytox * out.colorDCT[1][i]
		out.colorDCT[2][i] += ytob * out.colorDCT[1][i]
	}

	var color [3]uint64

	for c := range 3 {
		for i := range 32 {
			color[c] += uint64(math.Ceil(float64(invQuant * float32(absInt32(q.colorDCT[c][i])))))
		}
	}

	color[0] += uint64(math.Ceil(float64(absF(ytox)))) * color[1]
	color[2] += uint64(math.Ceil(float64(absF(ytob)))) * color[1]

	logColor := max(1, ceilLog2(1+max(color[0], max(color[1], color[2]))))

	weightLimit := float32(math.Ceil(math.Sqrt(float64(
		(float32(limit) / float32(logColor)) / float32(max(manhattan, 1))))))

	var width uint64

	for i := range 32 {
		f := float32(1)
		if i == 0 {
			f = float32(1 / math.Sqrt2)
		}

		out.sigmaDCT[i] = float32(q.sigmaDCT[i]) * f * splineChannelWeight[3] * invQuant

		wf := float32(math.Ceil(float64(invQuant * float32(absInt32(q.sigmaDCT[i])))))
		w := uint64(min(weightLimit, max(wf, 1)))
		width += w * w * uint64(logColor)
	}

	out.estimatedArea = width * manhattan

	return out, true
}

func catmullRom(points []point) []point {
	if len(points) <= 1 {
		return points
	}

	const n = 16

	ext := make([]point, 0, len(points)+2)
	ext = append(ext, points[0].add(points[0].sub(points[1])))
	ext = append(ext, points...)
	ext = append(ext, points[len(points)-1].add(points[len(points)-1].sub(points[len(points)-2])))

	deltas := make([]float32, len(ext))
	for i := range ext {
		if i+1 < len(ext) {
			deltas[i] = float32(math.Sqrt(float64(ext[i+1].sub(ext[i]).abs())))
		}
	}

	out := make([]point, 0, (len(ext)-3)*n+1)

	for w := 0; w+3 < len(ext); w++ {
		p := ext[w : w+4]
		d := deltas[w : w+4]

		var t [4]float32
		for k := range 3 {
			t[k+1] = t[k] + d[k]
		}

		out = append(out, p[1])

		for i := 1; i < n; i++ {
			tt := d[0] + (float32(i)/n)*d[1]

			var a [3]point

			for k := range 3 {
				a[k] = p[k].add(p[k+1].sub(p[k]).mul((tt - t[k]) / d[k]))
			}

			var b [2]point

			for k := range 2 {
				b[k] = a[k].add(a[k+1].sub(a[k]).mul((tt - t[k]) / (d[k] + d[k+1])))
			}

			out = append(out, b[0].add(b[1].sub(b[0]).mul((tt-t[1])/d[1])))
		}
	}

	return append(out, points[len(points)-1])
}

func equallySpacedPoints(points []point, distance float32) ([]point, []float32) {
	if len(points) == 0 {
		return nil, nil
	}

	outP := []point{points[0]}
	outD := []float32{distance}

	if len(points) == 1 {
		return outP, outD
	}

	acc := float32(0)

	for i := range len(points) - 1 {
		cur, next := points[i], points[i+1]

		seg := next.sub(cur)
		length := seg.abs()
		step := seg.div(length)

		if acc+length >= distance {
			cur = cur.add(step.mul(distance - acc))

			outP = append(outP, cur)
			outD = append(outD, distance)

			acc -= distance
		}

		acc += length

		for acc >= distance {
			cur = cur.add(step.mul(distance))

			outP = append(outP, cur)
			outD = append(outD, distance)

			acc -= distance
		}
	}

	return append(outP, points[len(points)-1]), append(outD, acc)
}

func continuousIDCT(dct *[32]float32, cos *[32]float32) float32 {
	var sum float32

	for i := range 32 {
		sum += dct[i] * cos[i]
	}

	return sum * float32(math.Sqrt2)
}

func (s *splines) addSegment(center point, intensity float32, color [3]float32,
	sigma float32, byY *[][2]int,
) {
	if math.IsInf(float64(sigma), 0) || sigma == 0 ||
		math.IsInf(float64(1/sigma), 0) || math.IsInf(float64(intensity), 0) {
		return
	}

	maxColor := absF(0.01 * intensity)
	for _, c := range color {
		maxColor = max(maxColor, absF(c*intensity))
	}

	maxDistance := float32(math.Sqrt(float64(
		-2 * sigma * sigma * (float32(math.Log(0.1))*3 - float32(math.Log(float64(maxColor)))))))

	s.segments = append(s.segments, splineSegment{
		centerX:        center.x,
		centerY:        center.y,
		color:          color,
		invSigma:       1 / sigma,
		sigmaIntensity: 0.25 * sigma * intensity,
		maxDistance:    maxDistance,
	})

	y0 := int64(math.Round(float64(center.y - maxDistance)))
	y1 := int64(math.Round(float64(center.y+maxDistance))) + 1

	for y := max(y0, 0); y < y1; y++ {
		*byY = append(*byY, [2]int{int(y), len(s.segments) - 1})
	}
}

func (s *splines) initDrawCache(w, h int, cmap *colorCorrelation) bool {
	area := uint64(w) * uint64(h)
	limit := splineAreaLimit(area)

	ytox := cmap.yToX(cmap.ytoxLF)
	ytob := cmap.yToB(cmap.ytobLF)

	var total uint64

	s.segments = nil

	var byY [][2]int

	for i := range s.quantized {
		sp, ok := s.quantized[i].dequantize(s.startingPoints[i], s.quantAdjustment, ytox, ytob, area)
		if !ok {
			return false
		}

		total += sp.estimatedArea
		if total > limit {
			return false
		}

		pts, mults := equallySpacedPoints(catmullRom(sp.controlPoints), splineRenderDistance)

		length := float32(len(pts)-2)*splineRenderDistance + mults[len(mults)-1]
		if length <= 0 {
			continue
		}

		inv := 1 / length

		var cos [32]float32

		for k, p := range pts {
			t := 31 * min(float32(k)*splineRenderDistance*inv, 1)

			for i := range 32 {
				cos[i] = fastCos(float32(math.Pi) / 32 * float32(i) * (t + 0.5))
			}

			var color [3]float32

			for c := range 3 {
				color[c] = continuousIDCT(&sp.colorDCT[c], &cos)
			}

			s.addSegment(p, mults[k], color, continuousIDCT(&sp.sigmaDCT, &cos), &byY)
		}
	}

	sortSegmentsByY(byY)

	s.segmentIndices = make([]int, len(byY))
	s.segmentYStart = make([]int, h+1)

	for i, seg := range byY {
		s.segmentIndices[i] = seg[1]

		if seg[0] < h {
			s.segmentYStart[seg[0]+1]++
		}
	}

	for y := range h {
		s.segmentYStart[y+1] += s.segmentYStart[y]
	}

	return true
}

func sortSegmentsByY(v [][2]int) {
	slices.SortStableFunc(v, func(a, b [2]int) int { return a[0] - b[0] })
}

func (s *splines) draw(planes [][]float32, w, h int) {
	for y := range h {
		for i := s.segmentYStart[y]; i < s.segmentYStart[y+1]; i++ {
			s.drawSegment(planes, w, y, &s.segments[s.segmentIndices[i]])
		}
	}
}

func (s *splines) drawSegment(planes [][]float32, w, y int, seg *splineSegment) {
	x0 := max(0, int(math.Round(float64(seg.centerX-seg.maxDistance))))
	x1 := min(w, int(math.Round(float64(seg.centerX+seg.maxDistance)))+1)

	if x1 <= x0 {
		return
	}

	dy := float32(y) - seg.centerY
	dy2 := dy * dy

	row := y * w

	for x := x0; x < x1; x++ {
		dx := float32(x) - seg.centerX
		distance := float32(math.Sqrt(float64(dx*dx + dy2)))

		arg1 := (distance*0.5 + 0.353553_38) * seg.invSigma
		arg2 := (distance*0.5 - 0.353553_38) * seg.invSigma

		factor := fastErff(arg1) - fastErff(arg2)
		intensity := seg.sigmaIntensity * factor * factor

		for c := range 3 {
			planes[c][row+x] += seg.color[c] * intensity
		}
	}
}
