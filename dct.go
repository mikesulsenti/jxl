package jxl

import (
	"math"
	"math/bits"
)

const (
	blockSize      = blockDim * blockDim
	maxCoeffBlocks = 32
	maxBlockDim    = blockDim * maxCoeffBlocks
	maxCoeffArea   = maxBlockDim * maxBlockDim
	dctSqrt2       = 1.41421356237
)

var (
	dctMultipliers [9][]float32
	dctResample    [6][]float32
	idctConsts     [31][8]float32
)

func init() {
	for l := 1; l <= 8; l++ {
		n := 1 << l
		m := make([]float32, n/2)

		for i := range m {
			m[i] = float32(1 / (2 * math.Cos((float64(i)+0.5)*math.Pi/float64(n))))
		}

		dctMultipliers[l] = m
	}

	for l := 0; l <= 5; l++ {
		n := 1 << l
		s := make([]float32, n)

		for i := range s {
			f := float64(i) * math.Pi / float64(n)
			s[i] = float32(1 / (math.Cos(f/16) * math.Cos(f/8) * math.Cos(f/4)))
		}

		dctResample[l] = s
	}

	c := append([]float32{dctSqrt2}, dctMultipliers[2]...)
	c = append(c, dctMultipliers[3]...)
	c = append(c, dctMultipliers[4]...)
	c = append(c, dctMultipliers[5]...)

	for i, v := range c {
		for j := range idctConsts[i] {
			idctConsts[i][j] = v
		}
	}
}

func idct4(a0, a1, a2, a3 float32) (float32, float32, float32, float32) {
	m := dctMultipliers[2]

	e0, e1 := a0+a2, a0-a2

	t3 := a3 + a1
	t2 := a1 * dctSqrt2

	o0, o1 := t2+t3, t2-t3

	p0, p1 := m[0]*o0, m[1]*o1

	return e0 + p0, e1 + p1, e1 - p1, e0 - p0
}

func idct8(from []float32, fromStride int, to []float32, toStride int) {
	m := dctMultipliers[3]

	e0, e1, e2, e3 := idct4(from[0], from[2*fromStride], from[4*fromStride], from[6*fromStride])

	t4 := from[fromStride]
	t5 := from[3*fromStride]
	t6 := from[5*fromStride]
	t7 := from[7*fromStride]

	t7 += t6
	t6 += t5
	t5 += t4
	t4 *= dctSqrt2

	o0, o1, o2, o3 := idct4(t4, t5, t6, t7)

	p0, p1 := m[0]*o0, m[1]*o1
	p2, p3 := m[2]*o2, m[3]*o3

	to[0] = e0 + p0
	to[7*toStride] = e0 - p0
	to[toStride] = e1 + p1
	to[6*toStride] = e1 - p1
	to[2*toStride] = e2 + p2
	to[5*toStride] = e2 - p2
	to[3*toStride] = e3 + p3
	to[4*toStride] = e3 - p3
}

func idct1d(n int, from []float32, fromStride int, to []float32, toStride int, tmp []float32) {
	switch n {
	case 1:
		to[0] = from[0]

		return
	case 2:
		a, b := from[0], from[fromStride]
		to[0] = a + b
		to[toStride] = a - b

		return
	case 4:
		a, b, c, d := idct4(from[0], from[fromStride], from[2*fromStride], from[3*fromStride])
		to[0], to[toStride], to[2*toStride], to[3*toStride] = a, b, c, d

		return
	case 8:
		idct8(from, fromStride, to, toStride)

		return
	}

	h := n / 2

	for i := range h {
		tmp[i] = from[2*i*fromStride]
	}

	for i := h; i < n; i++ {
		tmp[i] = from[(2*(i-h)+1)*fromStride]
	}

	idct1d(h, tmp, 1, tmp, 1, tmp[n:])

	for i := n - 1; i > h; i-- {
		tmp[i] += tmp[i-1]
	}

	tmp[h] *= dctSqrt2

	idct1d(h, tmp[h:], 1, tmp[h:], 1, tmp[n:])

	mul := dctMultipliers[bits.TrailingZeros(uint(n))]

	for i := range h {
		in1, in2 := tmp[i], tmp[h+i]
		to[i*toStride] = mul[i]*in2 + in1
		to[(n-1-i)*toStride] = in1 - mul[i]*in2
	}
}

func dct1d(n int, mem, tmp []float32) {
	switch n {
	case 1:
		return
	case 2:
		a, b := mem[0], mem[1]
		mem[0] = a + b
		mem[1] = a - b

		return
	}

	h := n / 2

	for i := range h {
		tmp[i] = mem[i] + mem[n-1-i]
		tmp[h+i] = mem[i] - mem[n-1-i]
	}

	dct1d(h, tmp, tmp[n:])

	mul := dctMultipliers[bits.TrailingZeros(uint(n))]

	for i := range h {
		tmp[h+i] *= mul[i]
	}

	dct1d(h, tmp[h:], tmp[n:])

	tmp[h] = tmp[h]*dctSqrt2 + tmp[h+1]

	for i := 1; i+1 < h; i++ {
		tmp[h+i] += tmp[h+i+1]
	}

	for i := range h {
		mem[2*i] = tmp[i]
		mem[2*i+1] = tmp[h+i]
	}
}

func idctColumns(n, m int, src []float32, srcStride int, dst []float32, dstStride int, tmp []float32) {
	j := 0

	for ; j+8 <= m; j += 8 {
		if !idctCols8(n, src[j:], srcStride, dst[j:], dstStride, tmp) {
			break
		}
	}

	for ; j < m; j++ {
		idct1d(n, src[j:], srcStride, dst[j:], dstStride, tmp)
	}
}

func dctColumns(n, m int, src []float32, srcStride int, dst []float32, dstStride int, tmp []float32) {
	scale := dctResample[bits.TrailingZeros(uint(n))]
	inv := 1 / float32(n)

	for j := range m {
		for i := range n {
			tmp[i] = src[i*srcStride+j]
		}

		dct1d(n, tmp, tmp[n:])

		for i := range n {
			dst[i*dstStride+j] = tmp[i] * inv * scale[i]
		}
	}
}

func transposeBlock(rows, cols int, src []float32, srcStride int, dst []float32, dstStride int) {
	if useTranspose8 && rows&7 == 0 && cols&7 == 0 {
		for y := 0; y < rows; y += 8 {
			for x := 0; x < cols; x += 8 {
				transpose8(src[y*srcStride+x:], srcStride, dst[x*dstStride+y:], dstStride)
			}
		}

		return
	}

	for y := range rows {
		row := src[y*srcStride : y*srcStride+cols]

		for x, v := range row {
			dst[x*dstStride+y] = v
		}
	}
}

func idct2d(rows, cols int, data, scratch []float32) {
	if rows == blockDim && cols == blockDim && idct8x8(data) {
		return
	}

	n := rows * cols
	block := scratch[:n]
	tmp := scratch[n:]

	if rows < cols {
		transposeBlock(rows, cols, data, cols, block, rows)
		idctColumns(cols, rows, block, rows, data, rows, tmp)
		transposeBlock(cols, rows, data, rows, block, cols)
		idctColumns(rows, cols, block, cols, data, cols, tmp)

		return
	}

	idctColumns(cols, rows, data, rows, block, rows, tmp)
	transposeBlock(cols, rows, block, rows, data, cols)
	idctColumns(rows, cols, data, cols, data, cols, tmp)
}

func fdctColumns(n, m int, src []float32, srcStride int, dst []float32, dstStride int, tmp []float32) {
	inv := 1 / float32(n)

	for j := range m {
		for i := range n {
			tmp[i] = src[i*srcStride+j]
		}

		dct1d(n, tmp, tmp[n:])

		for i := range n {
			dst[i*dstStride+j] = tmp[i] * inv
		}
	}
}

func fdct2d(rows, cols int, data, scratch []float32) {
	n := rows * cols
	block := scratch[:n]
	tmp := scratch[n:]

	if rows < cols {
		fdctColumns(rows, cols, data, cols, block, cols, tmp)
		transposeBlock(rows, cols, block, cols, data, rows)
		fdctColumns(cols, rows, data, rows, block, rows, tmp)
		transposeBlock(cols, rows, block, rows, data, cols)

		return
	}

	fdctColumns(rows, cols, data, cols, data, cols, tmp)
	transposeBlock(rows, cols, data, cols, block, rows)
	fdctColumns(cols, rows, block, rows, data, rows, tmp)
}

func unscaledColumns(n, m int, src []float32, srcStride int, dst []float32, dstStride int, tmp []float32) {
	scale := dctResample[bits.TrailingZeros(uint(n))]

	for j := range m {
		for i := range n {
			tmp[i] = src[i*srcStride+j] / scale[i]
		}

		idct1d(n, tmp, 1, tmp[n:], 1, tmp[2*n:])

		for i := range n {
			dst[i*dstStride+j] = tmp[n+i]
		}
	}
}

func unscaledDCT2D(rows, cols int, in []float32, inStride int, out []float32, scratch []float32) {
	n := rows * cols
	a, b := scratch[:n], scratch[n:2*n]
	tmp := scratch[2*n:]

	lo, hi := min(rows, cols), max(rows, cols)

	src := a
	if rows < cols {
		src = b
	}

	for y := range lo {
		copy(src[y*hi:(y+1)*hi], in[y*inStride:])
	}

	if rows < cols {
		transposeBlock(rows, cols, b, cols, a, rows)
	}

	unscaledColumns(cols, rows, a, rows, b, rows, tmp)
	transposeBlock(cols, rows, b, rows, a, cols)
	unscaledColumns(rows, cols, a, cols, out, cols, tmp)
}

func scaledDCT2D(rows, cols int, in, out []float32, outStride int, scratch []float32) {
	n := rows * cols
	a, b := scratch[:n], scratch[n:2*n]
	tmp := scratch[2*n:]

	dctColumns(rows, cols, in, cols, a, cols, tmp)
	transposeBlock(rows, cols, a, cols, b, rows)
	dctColumns(cols, rows, b, rows, a, rows, tmp)

	src, srcStride := a, rows
	if rows < cols {
		transposeBlock(cols, rows, a, rows, b, cols)

		src, srcStride = b, cols
	}

	for y := range min(rows, cols) {
		copy(out[y*outStride:y*outStride+max(rows, cols)], src[y*srcStride:])
	}
}
