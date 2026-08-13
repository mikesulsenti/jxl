package jxl

import (
	"errors"
	"io"
)

var streamChunk = 1 << 16

type source struct {
	r    io.Reader
	buf  []byte
	off  int
	eof  bool
	fail error
}

func newSource(r io.Reader, head []byte) *source {
	s := &source{r: r, buf: head}
	if r == nil {
		s.eof = true
	}

	return s
}

func (s *source) window() []byte { return s.buf[s.off:] }

func (s *source) avail() int { return len(s.buf) - s.off }

func (s *source) more() bool {
	if s.eof {
		return false
	}

	have := len(s.buf)
	s.buf = append(s.buf, make([]byte, streamChunk)...)

	n, err := io.ReadFull(s.r, s.buf[have:])
	s.buf = s.buf[:have+n]

	if err != nil {
		s.eof = true

		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			s.fail = err
		}
	}

	return n > 0
}

func (s *source) need(n int) bool {
	for s.avail() < n {
		if !s.more() {
			return false
		}
	}

	return true
}

func (s *source) advance(n int) {
	s.off += n

	if s.off >= streamChunk {
		s.buf = append(s.buf[:0], s.buf[s.off:]...)
		s.off = 0
	}
}

func (s *source) all() ([]byte, error) {
	for s.more() {
	}

	return s.window(), s.fail
}
