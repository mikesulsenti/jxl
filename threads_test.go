package jxl

import (
	"os"
	"path/filepath"
	"testing"
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

			want, err := decodeFloat(data, Options{Level: 10, Threads: 1})
			if err != nil {
				t.Fatal(err)
			}

			for _, n := range []int{2, 3, 8} {
				got, err := decodeFloat(data, Options{Level: 10, Threads: n})
				if err != nil {
					t.Fatalf("threads %d: %v", n, err)
				}

				if len(got.Frames) != len(want.Frames) {
					t.Fatalf("threads %d: %d frames, want %d",
						n, len(got.Frames), len(want.Frames))
				}

				for i := range want.Frames {
					a, b := want.Frames[i], got.Frames[i]

					if len(a) != len(b) {
						t.Fatalf("threads %d: frame %d has %d samples, want %d",
							n, i, len(b), len(a))
					}

					for j := range a {
						if a[j] != b[j] {
							t.Fatalf("threads %d: frame %d sample %d is %g, want %g",
								n, i, j, b[j], a[j])
						}
					}
				}
			}
		})
	}
}
