package jxl

import (
	"errors"
	"math"
	"math/bits"
	"math/rand/v2"
	"testing"
)

func TestBitReaderBits(t *testing.T) {
	r := newBitReader([]byte{0x00, 0x01})

	if v := r.bits(8); v != 0 {
		t.Errorf("bits(8) = %d, want 0", v)
	}
	if v := r.bits(4); v != 1 {
		t.Errorf("bits(4) = %d, want 1", v)
	}
	if v := r.bits(4); v != 0 {
		t.Errorf("bits(4) = %d, want 0", v)
	}
	if r.pos() != 16 {
		t.Errorf("pos = %d, want 16", r.pos())
	}
	if err := r.err(); err != nil {
		t.Fatalf("err = %v", err)
	}

	r.bits(1)

	if err := r.err(); !errors.Is(err, ErrInvalid) {
		t.Errorf("reading past the end gave %v, want ErrInvalid", err)
	}
}

func TestBitReaderRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))

	for iter := range 200 {
		var (
			w      bitWriter
			widths []int
			values []uint64
		)

		for range 1 + rng.IntN(64) {
			n := 1 + rng.IntN(maxBitsPerRead)
			v := rng.Uint64() >> (64 - n)

			widths = append(widths, n)
			values = append(values, v)
			w.bits(n, v)
		}

		r := newBitReader(w.bytes())

		for i, n := range widths {
			if v := r.bits(n); v != values[i] {
				t.Fatalf("iter %d field %d: bits(%d) = %#x, want %#x", iter, i, n, v, values[i])
			}
		}

		if err := r.err(); err != nil {
			t.Fatalf("iter %d: err = %v", iter, err)
		}
	}
}

func TestBitReaderU64(t *testing.T) {
	values := []uint64{0, 1, 2, 16, 17, 18, 272, 273, 274, 4095, 4096}
	for i := range 64 {
		values = append(values, 1<<i, 1<<i-1, 1<<i+1)
	}
	values = append(values, math.MaxUint64)

	for _, v := range values {
		var w bitWriter
		w.u64(v)

		r := newBitReader(w.bytes())

		if got := r.u64(); got != v {
			t.Errorf("u64 %#x round-tripped to %#x", v, got)
		}
		if err := r.err(); err != nil {
			t.Errorf("u64 %#x: err = %v", v, err)
		}
	}
}

func TestBitReaderU32(t *testing.T) {
	encs := [][]u32dist{
		{dVal(0), dVal(1), dVal(2), dVal(3)},
		{dBits(9), dBits(13), dBits(18), dBits(30)},
		{dVal(1), dBitsOff(2, 2), dBitsOff(4, 6), dBitsOff(8, 22)},
		{dBitsOff(3, 1), dBitsOff(6, 9), dBitsOff(10, 73), dBitsOff(30, 1097)},
	}

	rng := rand.New(rand.NewPCG(3, 4))

	for ei, enc := range encs {
		for range 2000 {
			v := rng.Uint32() >> rng.UintN(32)

			var w bitWriter

			w.u32(v, enc[0], enc[1], enc[2], enc[3])

			if w.err() != nil {
				continue
			}

			r := newBitReader(w.bytes())

			if got := r.u32(enc[0], enc[1], enc[2], enc[3]); got != v {
				t.Fatalf("enc %d: u32 %d round-tripped to %d", ei, v, got)
			}
			if err := r.err(); err != nil {
				t.Fatalf("enc %d: u32 %d: err = %v", ei, v, err)
			}
		}
	}
}

func TestBitReaderF16(t *testing.T) {
	for b := range 1 << 16 {
		r := newBitReader([]byte{byte(b), byte(b >> 8)})
		got := r.f16()

		sign, exp, mantissa := b>>15, (b>>10)&0x1f, b&0x3ff

		if exp == 31 {
			if !errors.Is(r.err(), ErrInvalid) {
				t.Fatalf("%#04x: infinity or NaN gave %v, want ErrInvalid", b, r.err())
			}

			continue
		}

		var want float64
		if exp == 0 {
			want = math.Ldexp(float64(mantissa)/1024, -14)
		} else {
			want = math.Ldexp(1+float64(mantissa)/1024, exp-15)
		}
		if sign != 0 {
			want = -want
		}

		if float64(got) != want {
			t.Fatalf("%#04x: f16 = %v, want %v", b, got, want)
		}
		if err := r.err(); err != nil {
			t.Fatalf("%#04x: err = %v", b, err)
		}
	}
}

func TestBitWriterF16(t *testing.T) {
	for b := range 1 << 16 {
		if exp := (b >> 10) & 0x1f; exp == 31 {
			continue
		}

		r := newBitReader([]byte{byte(b), byte(b >> 8)})

		v := r.f16()
		if err := r.err(); err != nil {
			t.Fatalf("%#04x: err = %v", b, err)
		}

		var w bitWriter

		w.f16(v)

		if err := w.err(); err != nil {
			t.Fatalf("%#04x: writing %v gave %v", b, v, err)
		}

		got := newBitReader(w.bytes()).f16()
		if math.Float32bits(got) != math.Float32bits(v) {
			t.Fatalf("%#04x: f16 %v round-tripped to %v", b, v, got)
		}
	}

	var w bitWriter

	w.f16(float32(math.Inf(1)))

	if !errors.Is(w.err(), ErrInvalid) {
		t.Errorf("writing an infinity gave %v, want ErrInvalid", w.err())
	}

	w = bitWriter{}
	w.f16(1e-9)

	if !errors.Is(w.err(), ErrInvalid) {
		t.Errorf("writing an unrepresentable value gave %v, want ErrInvalid", w.err())
	}
}

func TestBitReaderZeroPad(t *testing.T) {
	var w bitWriter
	w.bits(4, 5)

	r := newBitReader(w.bytes())
	r.bits(4)
	r.zeroPad()

	if r.pos() != 8 {
		t.Errorf("pos = %d, want 8", r.pos())
	}
	if err := r.err(); err != nil {
		t.Errorf("err = %v", err)
	}

	r = newBitReader([]byte{0xf0})
	r.bits(4)
	r.zeroPad()

	if err := r.err(); !errors.Is(err, ErrInvalid) {
		t.Errorf("nonzero padding gave %v, want ErrInvalid", err)
	}
}

func TestBitReaderSplit(t *testing.T) {
	for _, n := range []int{1, 4, 7, 8, 9, 16, 33} {
		data := make([]byte, n+16)
		for i := range data {
			data[i] = byte(i*37 + 11)
		}

		r := newBitReader(data)
		r.bits(4)
		r.zeroPad()

		sub := r.split(n)

		for i := range n {
			if v := byte(sub.bits(8)); v != data[1+i] {
				t.Fatalf("n=%d: sub byte %d = %#x, want %#x", n, i, v, data[1+i])
			}
		}

		sub.bits(8)

		if err := sub.err(); !errors.Is(err, ErrInvalid) {
			t.Errorf("n=%d: reading past the split gave %v, want ErrInvalid", n, err)
		}
		if v := byte(r.bits(8)); v != data[1+n] {
			t.Errorf("n=%d: parent resumed at %#x, want %#x", n, v, data[1+n])
		}
		if err := r.err(); err != nil {
			t.Errorf("n=%d: parent err = %v", n, err)
		}
	}
}

func TestBitReaderSkip(t *testing.T) {
	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	for _, n := range []int{0, 1, 7, 8, 9, 55, 56, 57, 64, 100, 511} {
		r := newBitReader(data)
		r.skip(n)

		if r.pos() != n {
			t.Errorf("skip(%d): pos = %d", n, r.pos())
		}

		want := newBitReader(data)
		for range n {
			want.bits(1)
		}

		if got, exp := r.bits(8), want.bits(8); got != exp {
			t.Errorf("skip(%d): next byte %#x, want %#x", n, got, exp)
		}
	}

	r := newBitReader(data)
	r.skip(len(data)*8 + 1)

	if err := r.err(); !errors.Is(err, ErrInvalid) {
		t.Errorf("skipping past the end gave %v, want ErrInvalid", err)
	}
}

func TestBitReaderTruncated(t *testing.T) {
	var w bitWriter
	for i := range 40 {
		n := 1 + i%17
		w.u64(uint64(i) * 7919)
		w.bits(n, uint64(i)&(1<<n-1))
	}

	full := w.bytes()

	for n := range len(full) {
		r := newBitReader(full[:n])

		for range 40 {
			r.u64()
			r.bits(9)
		}

		if err := r.err(); err == nil && r.pos() > r.avail {
			t.Errorf("prefix of %d/%d bytes read %d of %d bits without error",
				n, len(full), r.pos(), r.avail)
		}
	}

	r := newBitReader(full)
	for range 40 {
		r.u64()
		r.bits(9)
	}

	if err := r.err(); err != nil {
		t.Errorf("full stream: err = %v", err)
	}
}

func TestBitReaderLeft(t *testing.T) {
	r := newBitReader(make([]byte, 8))

	if got := r.left(); got != 64 {
		t.Fatalf("left = %d, want 64", got)
	}

	r.bits(1)

	if got := r.left(); got != 63 {
		t.Errorf("left = %d, want 63", got)
	}
}

func TestBitReaderRefillWindow(t *testing.T) {
	if maxBitsPerRead != 56 || bits.Len64(1<<maxBitsPerRead) > 64 {
		t.Fatalf("maxBitsPerRead = %d", maxBitsPerRead)
	}

	r := newBitReader(make([]byte, 16))
	r.peek(1)

	if r.nbits < maxBitsPerRead {
		t.Errorf("refill left %d bits, want at least %d", r.nbits, maxBitsPerRead)
	}
}
