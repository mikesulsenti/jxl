package jxl

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type npy struct {
	frames   int
	height   int
	width    int
	channels int
	data     []float32
}

func readNPY(path string) (*npy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < 10 || string(b[:6]) != "\x93NUMPY" {
		return nil, errors.New("not a npy file")
	}

	var hlen int
	switch major := b[6]; major {
	case 1:
		hlen = int(binary.LittleEndian.Uint16(b[8:10]))
		b = b[10:]
	case 2, 3:
		hlen = int(binary.LittleEndian.Uint32(b[8:12]))
		b = b[12:]
	default:
		return nil, fmt.Errorf("npy version %d", major)
	}

	if len(b) < hlen {
		return nil, errors.New("npy header truncated")
	}

	header, body := string(b[:hlen]), b[hlen:]

	if descr := npyField(header, "descr"); descr != "<f4" {
		return nil, fmt.Errorf("npy descr %q, want <f4", descr)
	}
	if order := npyField(header, "fortran_order"); order != "False" {
		return nil, fmt.Errorf("npy fortran_order %q", order)
	}

	shape, err := npyShape(header)
	if err != nil {
		return nil, err
	}
	if len(shape) != 4 {
		return nil, fmt.Errorf("npy shape %v, want 4 dimensions", shape)
	}

	n := shape[0] * shape[1] * shape[2] * shape[3]
	if len(body) < n*4 {
		return nil, fmt.Errorf("npy has %d bytes, want %d", len(body), n*4)
	}

	data := make([]float32, n)
	for i := range data {
		data[i] = math.Float32frombits(binary.LittleEndian.Uint32(body[i*4:]))
	}

	return &npy{shape[0], shape[1], shape[2], shape[3], data}, nil
}

func (a *npy) frame(i int) []float32 {
	n := a.height * a.width * a.channels

	return a.data[i*n : (i+1)*n]
}

func npyField(header, key string) string {
	i := strings.Index(header, "'"+key+"':")
	if i < 0 {
		return ""
	}

	v := strings.TrimSpace(header[i+len(key)+3:])
	if strings.HasPrefix(v, "'") {
		if j := strings.Index(v[1:], "'"); j >= 0 {
			return v[1 : 1+j]
		}

		return ""
	}

	if j := strings.IndexAny(v, ",}"); j >= 0 {
		return strings.TrimSpace(v[:j])
	}

	return ""
}

func npyShape(header string) ([]int, error) {
	i := strings.Index(header, "'shape':")
	if i < 0 {
		return nil, errors.New("npy header has no shape")
	}

	v := header[i+len("'shape':"):]

	open := strings.Index(v, "(")
	close := strings.Index(v, ")")
	if open < 0 || close < open {
		return nil, errors.New("npy shape is not a tuple")
	}

	var shape []int
	for _, f := range strings.Split(v[open+1:close], ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}

		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("npy shape %q: %w", f, err)
		}

		shape = append(shape, n)
	}

	return shape, nil
}

func TestNPY(t *testing.T) {
	root := conformanceRoot(t)

	dirs, err := filepath.Glob(filepath.Join(root, "testcases", "*", "reference_image.npy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no references found")
	}

	for _, path := range dirs {
		name := filepath.Base(filepath.Dir(path))

		t.Run(name, func(t *testing.T) {
			a, err := readNPY(path)
			if err != nil {
				t.Fatal(err)
			}

			b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "test.json"))
			if err != nil {
				t.Fatal(err)
			}

			var desc conformanceCase
			if err := json.Unmarshal(b, &desc); err != nil {
				t.Fatal(err)
			}

			if a.frames != len(desc.Frames) {
				t.Errorf("%d frames, test.json describes %d", a.frames, len(desc.Frames))
			}

			if want := len(desc.BitsPerSample) + 2; a.channels > want {
				t.Errorf("%d channels, test.json allows at most %d", a.channels, want)
			}
			if a.channels < len(desc.ExtraChannelType)+1 {
				t.Errorf("%d channels, fewer than the %d extra channels plus colour",
					a.channels, len(desc.ExtraChannelType))
			}

			if n := a.frames * a.height * a.width * a.channels; len(a.data) != n {
				t.Errorf("%d samples, want %d", len(a.data), n)
			}
			for i, v := range a.data {
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					t.Fatalf("sample %d is %v", i, v)
				}
			}
		})
	}
}
