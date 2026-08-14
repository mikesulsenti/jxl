package jxl

import (
	"encoding/binary"
	"math"
)

const maxBitsPerRead = 56

type bitReader struct {
	data  []byte
	buf   uint64
	nbits int
	read  int
	avail int
	fail  error
}

func newBitReader(data []byte) *bitReader {
	return &bitReader{data: data, avail: len(data) * 8}
}

func (r *bitReader) refill() {
	if len(r.data) >= 8 {
		r.buf |= binary.LittleEndian.Uint64(r.data) << r.nbits
		n := (63 - r.nbits) >> 3
		r.nbits |= maxBitsPerRead
		r.data = r.data[n:]

		return
	}

	for r.nbits < maxBitsPerRead && len(r.data) > 0 {
		r.buf |= uint64(r.data[0]) << r.nbits
		r.nbits += 8
		r.data = r.data[1:]
	}
}

func (r *bitReader) peek(n int) uint64 {
	if r.nbits < n {
		r.refill()
	}

	return r.buf & (1<<n - 1)
}

func (r *bitReader) consume(n int) {
	if r.nbits < n {
		r.setErr(ErrInvalid)
		r.read += r.nbits
		r.buf, r.nbits = 0, 0

		return
	}

	r.buf >>= n
	r.nbits -= n
	r.read += n
}

func (r *bitReader) bits(n int) uint64 {
	v := r.peek(n)
	r.consume(n)

	return v
}

func (r *bitReader) bit() bool { return r.bits(1) != 0 }

func (r *bitReader) consumeOpt(n int) {
	r.buf >>= n
	r.nbits = max(0, r.nbits-n)
	r.read += n
}

func (r *bitReader) bitsOpt(n int) uint64 {
	v := r.peek(n)
	r.consumeOpt(n)

	return v
}

func (r *bitReader) skip(n int) {
	if n <= r.nbits {
		r.buf >>= n
		r.nbits -= n
		r.read += n

		return
	}

	n -= r.nbits
	r.read += r.nbits
	r.buf, r.nbits = 0, 0

	if n > len(r.data)*8 {
		r.read += len(r.data) * 8
		r.data = nil
		r.setErr(ErrInvalid)

		return
	}

	r.read += n / 8 * 8
	r.data = r.data[n/8:]
	r.bits(n % 8)
}

func (r *bitReader) pos() int { return r.read }

func (r *bitReader) left() int { return len(r.data)*8 + r.nbits }

func (r *bitReader) setErr(err error) {
	if r.fail == nil {
		r.fail = err
	}
}

func (r *bitReader) err() error {
	if r.fail == nil && r.read > r.avail {
		return ErrInvalid
	}

	return r.fail
}

func (r *bitReader) toByte() int { return (8 - r.read%8) % 8 }

func (r *bitReader) zeroPad() {
	if r.bits(r.toByte()) != 0 {
		r.setErr(ErrInvalid)
	}
}

func (r *bitReader) split(n int) *bitReader {
	r.zeroPad()

	if n < 0 || n > r.left()/8 {
		r.setErr(ErrInvalid)

		return &bitReader{fail: ErrInvalid}
	}

	sub := &bitReader{data: r.data, buf: r.buf, nbits: r.nbits, avail: n * 8}

	r.skip(n * 8)

	if bytesInBuf := sub.nbits / 8; n > bytesInBuf {
		sub.data = sub.data[:n-bytesInBuf]
	} else {
		sub.nbits = n * 8
		sub.buf &= 1<<(n*8) - 1
		sub.data = nil
	}

	return sub
}

type u32dist struct {
	offset uint32
	bits   uint8
	direct bool
}

func dVal(v uint32) u32dist          { return u32dist{offset: v, direct: true} }
func dBitsOff(n, off uint32) u32dist { return u32dist{offset: off, bits: uint8(n)} }
func dBits(n uint32) u32dist         { return dBitsOff(n, 0) }

func (r *bitReader) u32(d0, d1, d2, d3 u32dist) uint32 {
	d := [4]u32dist{d0, d1, d2, d3}[r.bits(2)]
	if d.direct {
		return d.offset
	}

	return uint32(r.bits(int(d.bits))) + d.offset
}

func (r *bitReader) u64() uint64 {
	switch r.bits(2) {
	case 0:
		return 0
	case 1:
		return 1 + r.bits(4)
	case 2:
		return 17 + r.bits(8)
	}

	v := r.bits(12)

	for shift := 12; r.bit(); shift += 8 {
		if shift == 60 {
			v |= r.bits(4) << shift

			break
		}

		v |= r.bits(8) << shift
	}

	return v
}

func (r *bitReader) f16() float32 {
	b := uint32(r.bits(16))
	sign, exp, mantissa := b>>15, (b>>10)&0x1f, b&0x3ff

	switch exp {
	case 31:
		r.setErr(ErrInvalid)

		return 0
	case 0:
		v := float32(mantissa) * (1.0 / 1024) * (1.0 / 16384)
		if sign != 0 {
			v = -v
		}

		return v
	}

	return math.Float32frombits(sign<<31 | (exp+127-15)<<23 | mantissa<<13)
}
