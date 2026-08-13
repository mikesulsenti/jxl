package jxl

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/iotest"
)

func TestThreadsDeterministic(t *testing.T) {
	paths, err := filepath.Glob("testdata/*/*.jxl")
	if err != nil {
		t.Fatal(err)
	}

	if len(paths) == 0 {
		t.Skip("no streams in testdata")
	}

	for _, path := range paths {
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			want, err := decodeFloat(bytes.NewReader(data), Options{Level: 10, Threads: 1})
			if err != nil {
				t.Fatal(err)
			}

			for _, tc := range []struct {
				name    string
				threads int
				stream  bool
				chunk   int
			}{
				{"threads 2", 2, false, 0},
				{"threads 3", 3, false, 0},
				{"threads 8", 8, false, 0},
				{"64 byte window", 1, true, 64},
				{"1 byte window", 1, true, 1},
			} {
				var r io.Reader = bytes.NewReader(data)

				if tc.stream {
					r = iotest.OneByteReader(r)

					defer func(n int) { streamChunk = n }(streamChunk)

					streamChunk = tc.chunk
				}

				got, err := decodeFloat(r, Options{Level: 10, Threads: tc.threads})
				if err != nil {
					t.Fatalf("%s: %v", tc.name, err)
				}

				if len(got.Frames) != len(want.Frames) {
					t.Fatalf("%s: %d frames, want %d",
						tc.name, len(got.Frames), len(want.Frames))
				}

				for i := range want.Frames {
					a, b := want.Frames[i], got.Frames[i]

					if len(a) != len(b) {
						t.Fatalf("%s: frame %d has %d samples, want %d",
							tc.name, i, len(b), len(a))
					}

					for j := range a {
						if a[j] != b[j] {
							t.Fatalf("%s: frame %d sample %d is %g, want %g",
								tc.name, i, j, b[j], a[j])
						}
					}
				}
			}
		})
	}
}
