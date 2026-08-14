package jxl

import "math"

type bitWriter struct {
	data  []byte
	buf   uint64
	nbits int
	fail  error
}

func (w *bitWriter) setErr(err error) {
	if w.fail == nil {
		w.fail = err
	}
}

func (w *bitWriter) err() error { return w.fail }

func (w *bitWriter) flush() {
	for w.nbits >= 8 {
		w.data = append(w.data, byte(w.buf))
		w.buf >>= 8
		w.nbits -= 8
	}
}

func (w *bitWriter) bits(n int, v uint64) {
	if n < 0 || n > maxBitsPerRead {
		w.setErr(ErrInvalid)

		return
	}

	if n < 64 && v>>n != 0 {
		w.setErr(ErrInvalid)

		return
	}

	w.buf |= v << w.nbits
	w.nbits += n

	w.flush()
}

func (w *bitWriter) reserve(n int) {
	if cap(w.data)-len(w.data) >= n {
		return
	}

	data := make([]byte, len(w.data), len(w.data)+n)
	copy(data, w.data)
	w.data = data
}

func (w *bitWriter) writeBytes(b []byte) {
	if w.nbits == 0 {
		w.data = append(w.data, b...)

		return
	}

	for _, v := range b {
		w.bits(8, uint64(v))
	}
}

func (w *bitWriter) bit(b bool) {
	if b {
		w.bits(1, 1)

		return
	}

	w.bits(1, 0)
}

func (w *bitWriter) zeroPad() {
	if n := (8 - w.nbits%8) % 8; n != 0 {
		w.bits(n, 0)
	}
}

func (w *bitWriter) pos() int { return len(w.data)*8 + w.nbits }

func (w *bitWriter) append(src *bitWriter) {
	if src.fail != nil {
		w.setErr(src.fail)

		return
	}

	w.reserve(len(src.data) + 1)
	w.writeBytes(src.data)

	if src.nbits > 0 {
		w.bits(src.nbits, src.buf&(1<<src.nbits-1))
	}
}

func (w *bitWriter) bytes() []byte {
	w.zeroPad()

	return w.data
}

func (w *bitWriter) u32(v uint32, d0, d1, d2, d3 u32dist) {
	best, total := -1, 0

	for i, d := range [4]u32dist{d0, d1, d2, d3} {
		if d.direct {
			if d.offset != v {
				continue
			}

			w.bits(2, uint64(i))

			return
		}

		if uint64(v) < uint64(d.offset) || uint64(v) >= uint64(d.offset)+1<<d.bits {
			continue
		}

		if n := int(d.bits); best < 0 || n < total {
			best, total = i, n
		}
	}

	if best < 0 {
		w.setErr(ErrInvalid)

		return
	}

	w.bits(2, uint64(best))
	w.bits(total, uint64(v)-uint64([4]u32dist{d0, d1, d2, d3}[best].offset))
}

func (w *bitWriter) u64(v uint64) {
	switch {
	case v == 0:
		w.bits(2, 0)
	case v <= 16:
		w.bits(2, 1)
		w.bits(4, v-1)
	case v <= 272:
		w.bits(2, 2)
		w.bits(8, v-17)
	default:
		w.bits(2, 3)
		w.bits(12, v&0xfff)

		v >>= 12

		for shift := 12; ; shift += 8 {
			if v == 0 {
				w.bit(false)

				return
			}

			w.bit(true)

			if shift == 60 {
				w.bits(4, v&0xf)

				return
			}

			w.bits(8, v&0xff)
			v >>= 8
		}
	}
}

func (w *bitWriter) f16(v float32) {
	b := math.Float32bits(v)
	sign := uint64(b >> 31)
	exp := int32((b>>23)&0xff) - 127
	frac := b & 0x7fffff

	if b<<1 == 0 {
		w.bits(16, sign<<15)

		return
	}

	if exp == 128 || exp > 15 {
		w.setErr(ErrInvalid)

		return
	}

	if exp >= -14 {
		if frac&0x1fff != 0 {
			w.setErr(ErrInvalid)

			return
		}

		w.bits(16, sign<<15|uint64(exp+15)<<10|uint64(frac>>13))

		return
	}

	shift := -exp - 1
	sig := uint32(1)<<23 | frac

	if shift >= 32 || sig&(1<<shift-1) != 0 || sig>>shift == 0 {
		w.setErr(ErrInvalid)

		return
	}

	w.bits(16, sign<<15|uint64(sig>>shift))
}
