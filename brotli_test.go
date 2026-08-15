package jxl

import (
	"bytes"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// brotliCompress runs the reference compressor, which is the oracle for every
// test here. Without it on the path the tests skip.
func brotliCompress(t *testing.T, data []byte, quality int) []byte {
	t.Helper()

	bin, err := exec.LookPath("brotli")
	if err != nil {
		t.Skip("brotli is not on the path")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "in")
	dst := filepath.Join(dir, "in.br")

	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "-q", itoa(quality), "-o", dst, src)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("brotli -q %d: %v\n%s", quality, err, out)
	}

	out, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}

	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}

	var b []byte

	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}

	return string(b)
}

func checkBrotli(t *testing.T, name string, want []byte, quality int) {
	t.Helper()

	got, err := brotliDecode(brotliCompress(t, want, quality), len(want)+1)
	if err != nil {
		t.Fatalf("%s at q%d: %v", name, quality, err)
	}

	if !bytes.Equal(got, want) {
		n := len(got)
		if len(want) < n {
			n = len(want)
		}

		at := n

		for i := range n {
			if got[i] != want[i] {
				at = i

				break
			}
		}

		t.Fatalf("%s at q%d: %d bytes, want %d, first difference at %d",
			name, quality, len(got), len(want), at)
	}
}

func TestBrotliRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(17, 19))

	text := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:creator><rdf:Seq><rdf:li>A Photographer</rdf:li></rdf:Seq></dc:creator>
</rdf:Description></rdf:RDF></x:xmpmeta>`)

	noise := make([]byte, 40000)
	for i := range noise {
		noise[i] = byte(rng.UintN(256))
	}

	runs := make([]byte, 30000)
	for i := range runs {
		runs[i] = byte(i / 700)
	}

	mixed := append(append(append([]byte{}, text...), noise[:2000]...), text...)

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"one byte", []byte{7}},
		{"text", text},
		{"english", bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog. "), 40)},
		{"html", []byte("<html><head><title>a</title></head><body><div class=\"x\">hello</div></body></html>")},
		{"zeros", make([]byte, 70000)},
		{"runs", runs},
		{"noise", noise},
		{"mixed", mixed},
	}

	for _, tc := range cases {
		for _, q := range []int{0, 1, 5, 9, 11} {
			checkBrotli(t, tc.name, tc.data, q)
		}
	}
}

// TestBrotliDictionary compresses the words of the static dictionary so the
// stream has to reach for dictionary references and the transforms.
func TestBrotliDictionary(t *testing.T) {
	rng := rand.New(rand.NewPCG(23, 29))

	var buf bytes.Buffer

	for range 400 {
		length := 4 + rng.IntN(20)
		bits := brotliDictBits[length]

		if bits == 0 {
			continue
		}

		word := rng.UintN(1 << bits)
		off := brotliDictOffset[length] + uint32(word)*uint32(length)

		buf.WriteString(brotliDictionary[off : off+uint32(length)])
		buf.WriteByte(' ')
	}

	for _, q := range []int{5, 9, 11} {
		checkBrotli(t, "dictionary words", buf.Bytes(), q)
	}
}

func TestBrotliLimit(t *testing.T) {
	data := bytes.Repeat([]byte("limit me "), 1000)
	src := brotliCompress(t, data, 9)

	if _, err := brotliDecode(src, len(data)-1); err == nil {
		t.Fatal("a stream past the limit was accepted")
	}

	if _, err := brotliDecode(src, len(data)); err != nil {
		t.Fatalf("a stream at the limit gave %v", err)
	}
}

func TestBrotliRejectsGarbage(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 37))

	good := brotliCompress(t, []byte("a short string to corrupt, with words"), 9)

	for range 2000 {
		bad := append([]byte(nil), good...)
		bad[rng.IntN(len(bad))] ^= 1 << rng.UintN(8)

		if out, err := brotliDecode(bad, 1<<20); err == nil && len(out) > 1<<20 {
			t.Fatal("corrupt stream produced more than the limit")
		}
	}

	for range 2000 {
		n := 1 + rng.IntN(64)
		bad := make([]byte, n)

		for i := range bad {
			bad[i] = byte(rng.UintN(256))
		}

		brotliDecode(bad, 1<<16)
	}
}

// TestBrotliCorpus decompresses every brob box the corpus carries and checks
// the bytes against the reference decompressor.
func TestBrotliCorpus(t *testing.T) {
	root := corpusRoot(t)

	bin, err := exec.LookPath("brotli")
	if err != nil {
		t.Skip("brotli is not on the path")
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "box.br")
	dst := filepath.Join(dir, "box")

	boxes, total := 0, 0

	for _, sub := range corpusSubdirs {
		files, err := filepath.Glob(filepath.Join(root, sub, "*.jxl"))
		if err != nil {
			t.Fatal(err)
		}

		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			c, err := readContainer(data[len(sigContainer):], false)
			if err != nil {
				continue
			}

			for typ, body := range c.compressed {
				name := filepath.Base(path) + " " + typ

				got, err := brotliDecode(body, maxMetadataSize)
				if err != nil {
					t.Errorf("%s: %v", name, err)

					continue
				}

				if err := os.WriteFile(src, body, 0o600); err != nil {
					t.Fatal(err)
				}

				os.Remove(dst)

				if o, err := exec.Command(bin, "-d", "-o", dst, src).CombinedOutput(); err != nil {
					t.Fatalf("%s: reference gave %v\n%s", name, err, o)
				}

				want, err := os.ReadFile(dst)
				if err != nil {
					t.Fatal(err)
				}

				if !bytes.Equal(got, want) {
					t.Errorf("%s: %d bytes, reference gives %d", name, len(got), len(want))

					continue
				}

				boxes++
				total += len(want)
			}
		}
	}

	if boxes == 0 {
		t.Skip("no compressed boxes in the corpus")
	}

	t.Logf("%d compressed boxes match the reference, %d bytes", boxes, total)
}

func FuzzBrotli(f *testing.F) {
	f.Add([]byte{0x03})
	f.Add([]byte{0x0b, 0x02, 0x80, 'a', 0x03})
	f.Add([]byte{0x1b, 0x0e, 0x00, 0x00, 0x00, 0x00, 0x80, 0xe3, 0xb4, 0x0d})

	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := brotliDecode(data, 1<<20)

		if err == nil && len(out) > 1<<20 {
			t.Fatalf("%d bytes past the limit", len(out))
		}
	})
}
