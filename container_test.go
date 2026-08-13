package jxl

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func box(typ string, body ...[]byte) []byte {
	content := bytes.Join(body, nil)
	b := make([]byte, 8, 8+len(content))

	binary.BigEndian.PutUint32(b, uint32(8+len(content)))
	copy(b[4:], typ)

	return append(b, content...)
}

func boxLarge(typ string, body ...[]byte) []byte {
	content := bytes.Join(body, nil)
	b := make([]byte, 16, 16+len(content))

	binary.BigEndian.PutUint32(b, 1)
	copy(b[4:], typ)
	binary.BigEndian.PutUint64(b[8:], uint64(16+len(content)))

	return append(b, content...)
}

func ftyp() []byte {
	return box("ftyp", []byte("jxl "), []byte{0, 0, 0, 0}, []byte("jxl "))
}

func ftypV1() []byte {
	return box("ftyp", []byte("jxl "), []byte{0, 0, 0, 1}, []byte("jxl "))
}

func jxlp(index uint32, last bool, body []byte) []byte {
	if last {
		index |= 0x80000000
	}

	idx := make([]byte, 4)
	binary.BigEndian.PutUint32(idx, index)

	return box("jxlp", idx, body)
}

func TestParseContainer(t *testing.T) {
	stream := []byte("\xff\x0acodestream")

	tests := []struct {
		name string
		in   []byte
		want []byte
		err  bool
	}{
		{
			name: "jxlc",
			in:   bytes.Join([][]byte{ftyp(), box("jxlc", stream)}, nil),
			want: stream,
		},
		{
			name: "jxlc with 64-bit size",
			in:   bytes.Join([][]byte{ftyp(), boxLarge("jxlc", stream)}, nil),
			want: stream,
		},
		{
			name: "jxlp in order",
			in: bytes.Join([][]byte{ftyp(),
				jxlp(0, false, stream[:5]), jxlp(1, false, stream[5:9]),
				jxlp(2, true, stream[9:])}, nil),
			want: stream,
		},
		{
			name: "jxlp out of order, version 1",
			in: bytes.Join([][]byte{ftypV1(),
				jxlp(2, true, stream[9:]), jxlp(0, false, stream[:5]),
				jxlp(1, false, stream[5:9])}, nil),
			want: stream,
		},
		{
			name: "metadata between codestream boxes",
			in: bytes.Join([][]byte{ftyp(),
				jxlp(0, false, stream[:5]), box("Exif", []byte("exifdata")),
				box("xml ", []byte("<x/>")), jxlp(1, true, stream[5:])}, nil),
			want: stream,
		},
		{
			name: "unknown box is skipped",
			in: bytes.Join([][]byte{ftyp(), box("zzzz", []byte("whatever")),
				box("jxlc", stream)}, nil),
			want: stream,
		},
		{
			name: "jbrd is skipped, not refused",
			in: bytes.Join([][]byte{ftyp(), box("jxlc", stream),
				box("jbrd", []byte("reconstruction"))}, nil),
			want: stream,
		},

		{
			name: "no ftyp",
			in:   box("jxlc", stream),
			err:  true,
		},
		{
			name: "ftyp is not first",
			in:   bytes.Join([][]byte{box("jxlc", stream), ftyp()}, nil),
			err:  true,
		},
		{
			name: "wrong brand",
			in: bytes.Join([][]byte{box("ftyp", []byte("avif"), []byte{0, 0, 0, 0}),
				box("jxlc", stream)}, nil),
			err: true,
		},
		{
			name: "future ftyp version",
			in: bytes.Join([][]byte{box("ftyp", []byte("jxl "), []byte{0, 0, 0, 2}),
				box("jxlc", stream)}, nil),
			err: true,
		},
		{
			name: "two ftyp",
			in:   bytes.Join([][]byte{ftyp(), ftyp(), box("jxlc", stream)}, nil),
			err:  true,
		},
		{
			name: "no codestream",
			in:   bytes.Join([][]byte{ftyp(), box("Exif", []byte("x"))}, nil),
			err:  true,
		},
		{
			name: "jxlc after jxlp",
			in: bytes.Join([][]byte{ftyp(), jxlp(0, false, stream[:5]),
				box("jxlc", stream)}, nil),
			err: true,
		},
		{
			name: "jxlp after jxlc",
			in: bytes.Join([][]byte{ftyp(), box("jxlc", stream),
				jxlp(0, true, stream)}, nil),
			err: true,
		},
		{
			name: "jxlp after the last one",
			in: bytes.Join([][]byte{ftyp(), jxlp(0, true, stream),
				jxlp(1, true, stream)}, nil),
			err: true,
		},
		{
			name: "jxlp with a repeated index",
			in: bytes.Join([][]byte{ftypV1(), jxlp(0, false, stream[:5]),
				jxlp(0, true, stream[5:])}, nil),
			err: true,
		},
		{
			name: "jxlp with a gap",
			in: bytes.Join([][]byte{ftypV1(), jxlp(0, false, stream[:5]),
				jxlp(2, true, stream[5:])}, nil),
			err: true,
		},
		{
			name: "jxlp out of order, version 0",
			in: bytes.Join([][]byte{ftyp(), jxlp(1, true, stream[5:]),
				jxlp(0, false, stream[:5])}, nil),
			err: true,
		},
		{
			name: "jxlp never marked last",
			in: bytes.Join([][]byte{ftyp(), jxlp(0, false, stream[:5]),
				jxlp(1, false, stream[5:])}, nil),
			err: true,
		},
		{
			name: "box size below its header",
			in:   bytes.Join([][]byte{ftyp(), {0, 0, 0, 4, 'j', 'x', 'l', 'c'}}, nil),
			err:  true,
		},
		{
			name: "box size past the end",
			in:   bytes.Join([][]byte{ftyp(), {0, 0, 0xff, 0, 'j', 'x', 'l', 'c'}}, nil),
			err:  true,
		},
		{
			name: "truncated box header",
			in:   bytes.Join([][]byte{ftyp(), {0, 0, 0}}, nil),
			err:  true,
		},
		{
			name: "64-bit size announced but absent",
			in:   bytes.Join([][]byte{ftyp(), {0, 0, 0, 1, 'j', 'x', 'l', 'c'}}, nil),
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseContainer(tt.in)

			if tt.err {
				if err == nil {
					t.Fatal("no error")
				}

				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(c.codestream, tt.want) {
				t.Errorf("codestream = %q, want %q", c.codestream, tt.want)
			}
		})
	}
}

func TestParseContainerSizeZero(t *testing.T) {
	stream := []byte("\xff\x0acodestream")

	in := append(ftyp(), append([]byte{0, 0, 0, 0, 'j', 'x', 'l', 'c'}, stream...)...)

	c, err := parseContainer(in)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(c.codestream, stream) {
		t.Errorf("codestream = %q, want %q", c.codestream, stream)
	}
}

func TestParseContainerMetadata(t *testing.T) {
	stream := []byte("\xff\x0acodestream")

	in := bytes.Join([][]byte{ftyp(),
		box("jxll", []byte{10}),
		box("Exif", []byte("exifdata")),
		box("xml ", []byte("<x/>")),
		box("jumb", []byte("jumbf")),
		box("brob", []byte("xml "), []byte("compressed")),
		box("jxlc", stream),
	}, nil)

	c, err := parseContainer(in)
	if err != nil {
		t.Fatal(err)
	}

	if c.level != 10 {
		t.Errorf("level = %d, want 10", c.level)
	}
	if string(c.exif) != "exifdata" {
		t.Errorf("exif = %q", c.exif)
	}
	if string(c.xmp) != "<x/>" {
		t.Errorf("xmp = %q", c.xmp)
	}
	if string(c.jumb) != "jumbf" {
		t.Errorf("jumb = %q", c.jumb)
	}
	if len(c.compressed) != 1 || c.compressed[0] != "xml " {
		t.Errorf("compressed = %q, want [\"xml \"]", c.compressed)
	}
}

func TestContainerCorpus(t *testing.T) {
	root := conformanceRoot(t)

	paths, err := filepath.Glob(filepath.Join(root, "testcases", "*", "input.jxl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no inputs found")
	}

	var boxed int

	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path))

		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			isContainer, skip, err := signature(data)
			if err != nil {
				t.Fatal(err)
			}
			if !isContainer {
				return
			}

			boxed++

			c, err := parseContainer(data[skip:])
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(c.codestream, []byte(sigCodestream)) {
				t.Fatalf("extracted codestream starts %x, want %x",
					c.codestream[:min(2, len(c.codestream))], sigCodestream)
			}
			if len(c.compressed) > 0 {
				t.Logf("brotli-compressed boxes left undecoded: %q", c.compressed)
			}
		})
	}

	if boxed == 0 {
		t.Error("no container-format inputs in the corpus")
	}

	t.Logf("%d of %d inputs use the container format", boxed, len(paths))
}

type shortReader struct {
	data []byte
	pos  int
	read int
}

func (s *shortReader) Read(p []byte) (int, error) {
	if s.pos >= len(s.data) {
		return 0, io.EOF
	}

	n := copy(p, s.data[s.pos:])
	s.pos += n
	s.read += n

	return n, nil
}

// TestDecodeConfigReadsAHeader checks that DecodeConfig answers from a prefix
// rather than the whole file, and that the answer is the one the whole file
// gives.
func TestDecodeConfigReadsAHeader(t *testing.T) {
	var paths []string

	for _, g := range []string{
		filepath.Join("testdata", "*", "*.jxl"),
		filepath.Join(conformanceRoot(t), "testcases", "*", "input.jxl"),
	} {
		m, err := filepath.Glob(g)
		if err != nil {
			t.Fatal(err)
		}

		paths = append(paths, m...)
	}

	if len(paths) == 0 {
		t.Fatal("no inputs found")
	}

	checked := 0

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}

		want, werr := decodeConfig(data)

		r := &shortReader{data: data}

		got, err := DecodeConfig(r)
		if !errors.Is(err, werr) {
			t.Fatalf("%s: streamed error %v, whole file %v", p, err, werr)
		}
		if werr != nil {
			continue
		}
		if got != want {
			t.Fatalf("%s: streamed %v, whole file %v", p, got, want)
		}

		checked++

		if len(data) > 4*configChunk && r.read > configChunk {
			t.Errorf("%s: read %d of %d bytes for a header", p, r.read, len(data))
		}
	}

	if checked == 0 {
		t.Fatal("no inputs produced a config")
	}

	t.Logf("%d inputs, each answered from at most %d bytes", checked, configChunk)
}

func FuzzContainer(f *testing.F) {
	stream := []byte("\xff\x0acodestream")

	f.Add(bytes.Join([][]byte{ftyp(), box("jxlc", stream)}, nil))
	f.Add(bytes.Join([][]byte{ftypV1(), jxlp(1, true, stream[5:]),
		jxlp(0, false, stream[:5])}, nil))
	f.Add(bytes.Join([][]byte{ftyp(), box("brob", []byte("Exif"), []byte("x")),
		boxLarge("jxlc", stream)}, nil))

	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := parseContainer(data)
		if err != nil {
			return
		}
		if c.codestream == nil {
			t.Fatal("no error but no codestream")
		}
	})
}
