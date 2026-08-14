package jxl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The corpora are large and live outside the repository. CONFORMANCE_DIR is a
// colon separated list of them, and each test takes the entries it recognises.
func conformanceDirs(t *testing.T) []string {
	t.Helper()

	env := os.Getenv("CONFORMANCE_DIR")
	if env == "" {
		t.Skip("set CONFORMANCE_DIR")
	}

	return strings.Split(env, ":")
}

// conformanceRoot is the official suite, which keeps its case lists in
// testcases and its references in a tree beside them.
func conformanceRoot(t *testing.T) string {
	t.Helper()

	for _, dir := range conformanceDirs(t) {
		if _, err := os.Stat(filepath.Join(dir, "testcases")); err == nil {
			return dir
		}
	}

	t.Skip("no conformance suite in CONFORMANCE_DIR")

	return ""
}

var updateRatchet = flag.Bool("conformance.update", false,
	"rewrite testdata/conformance.txt from this run")

type conformanceSuite struct {
	name     string
	level    int
	baseline int
}

var conformanceSuites = []conformanceSuite{
	{"main_level5", 5, 22},
	{"main_level10", 10, 26},
}

type conformanceCase struct {
	Frames []struct {
		Name string  `json:"name"`
		RMS  float64 `json:"rms_error"`
		Peak float64 `json:"peak_error"`
	} `json:"frames"`
	BitsPerSample    []int    `json:"bits_per_sample"`
	ExpBitsPerSample []int    `json:"exp_bits_per_sample"`
	ExtraChannelType []string `json:"extra_channel_type"`
	IntensityTarget  float64  `json:"intensity_target"`
	OriginalICC      string   `json:"original_icc"`
	ReconstructedJPG string   `json:"reconstructed_jpeg"`
}

func (s conformanceSuite) cases(t *testing.T, root string) []string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(root, "testcases", s.name+".txt"))
	if err != nil {
		t.Skipf("no corpus list for %s", s.name)
	}

	var names []string
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}

	return names
}

type conformanceResult struct {
	verdict string
	peak    float64
	rms     float64
	note    string
}

func TestConformance(t *testing.T) {
	root := conformanceRoot(t)
	ratchet := readRatchet(t)
	fresh := map[string]conformanceResult{}

	for _, s := range conformanceSuites {
		names := s.cases(t, root)
		counts := map[string]int{}

		for _, name := range names {
			key := s.name + "/" + name
			res := runConformanceCase(t, root, name, s.level, ratchet[key])
			counts[res.verdict]++
			fresh[key] = res

			switch res.verdict {
			case "pass":
			case "drift":
				t.Errorf("%s: %s", key, res.note)
			default:
				t.Logf("%s: %s: %s", key, res.verdict, res.note)
			}
		}

		t.Logf("%s: %d/%d pass, %d drift, %d fail, %d error", s.name,
			counts["pass"], len(names), counts["drift"], counts["fail"], counts["error"])

		if got := counts["pass"]; got < s.baseline {
			t.Errorf("%s: %d pass, baseline is %d", s.name, got, s.baseline)
		}
	}

	if *updateRatchet {
		writeRatchet(t, fresh)
	}
}

func runConformanceCase(t *testing.T, root, name string, level int, prev conformanceResult) conformanceResult {
	t.Helper()

	dir := filepath.Join(root, "testcases", name)

	var desc conformanceCase
	b, err := os.ReadFile(filepath.Join(dir, "test.json"))
	if err != nil {
		return conformanceResult{verdict: "error", note: err.Error()}
	}
	if err := json.Unmarshal(b, &desc); err != nil {
		return conformanceResult{verdict: "error", note: err.Error()}
	}

	ref, err := readNPY(filepath.Join(dir, "reference_image.npy"))
	if err != nil {
		return conformanceResult{verdict: "error", note: err.Error()}
	}

	in, err := os.Open(filepath.Join(dir, "input.jxl"))
	if err != nil {
		return conformanceResult{verdict: "error", note: err.Error()}
	}
	defer in.Close()

	got, err := DecodeFloat(in, Options{Level: level})
	if err != nil {
		return conformanceResult{verdict: "error", note: err.Error()}
	}

	if len(got.Frames) != ref.frames || got.Height != ref.height ||
		got.Width != ref.width || got.Channels != ref.channels {
		return conformanceResult{verdict: "fail", note: fmt.Sprintf(
			"shape %dx%dx%dx%d, want %dx%dx%dx%d",
			len(got.Frames), got.Height, got.Width, got.Channels,
			ref.frames, ref.height, ref.width, ref.channels)}
	}
	if len(desc.Frames) != ref.frames {
		return conformanceResult{verdict: "error", note: fmt.Sprintf(
			"test.json has %d frames, reference has %d", len(desc.Frames), ref.frames)}
	}

	res := conformanceResult{verdict: "pass"}

	for i, limit := range desc.Frames {
		peak, rms := compareFrame(got.Frames[i], ref.frame(i), ref.channels)
		res.peak = math.Max(res.peak, peak)
		res.rms = math.Max(res.rms, rms)

		if res.verdict == "pass" {
			if rms > limit.RMS {
				res.verdict = "fail"
				res.note = fmt.Sprintf("frame %d: rms %g > %g", i, rms, limit.RMS)
			} else if peak > limit.Peak {
				res.verdict = "fail"
				res.note = fmt.Sprintf("frame %d: peak %g > %g", i, peak, limit.Peak)
			}
		}
	}

	if res.verdict != "pass" {
		return res
	}

	if prev.verdict == "pass" {
		if grew, was, now := ratchetGrew(prev, res); grew {
			res.verdict = "drift"
			res.note = fmt.Sprintf("error grew: %g, recorded %g", now, was)
		}
	}

	return res
}

func ratchetGrew(prev, res conformanceResult) (bool, float64, float64) {
	const band, floor = 1.25, 1e-7

	if res.peak > prev.peak*band+floor {
		return true, prev.peak, res.peak
	}
	if res.rms > prev.rms*band+floor {
		return true, prev.rms, res.rms
	}

	return false, 0, 0
}

func compareFrame(got, want []float32, channels int) (peak, rms float64) {
	sum := make([]float64, channels)
	n := len(want) / channels

	for i, w := range want {
		d := math.Abs(float64(got[i]) - float64(w))
		if d > peak {
			peak = d
		}
		sum[i%channels] += d * d
	}

	for _, s := range sum {
		if r := math.Sqrt(s / float64(n)); r > rms {
			rms = r
		}
	}

	return peak, rms
}

const ratchetPath = "testdata/conformance.txt"

func readRatchet(t *testing.T) map[string]conformanceResult {
	t.Helper()

	m := map[string]conformanceResult{}

	f, err := os.Open(ratchetPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}

		return m
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if i := strings.Index(text, "#"); i >= 0 {
			text = text[:i]
		}

		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 3 {
			t.Fatalf("%s:%d: want 'case peak rms'", ratchetPath, line)
		}

		peak, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			t.Fatalf("%s:%d: %v", ratchetPath, line, err)
		}

		rms, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			t.Fatalf("%s:%d: %v", ratchetPath, line, err)
		}

		m[fields[0]] = conformanceResult{verdict: "pass", peak: peak, rms: rms}
	}

	return m
}

func writeRatchet(t *testing.T, res map[string]conformanceResult) {
	t.Helper()

	keys := make([]string, 0, len(res))
	for k, v := range res {
		if v.verdict == "pass" || v.verdict == "drift" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# case peak rms, from go test -run TestConformance -conformance.update\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s %g %g\n", k, res[k].peak, res[k].rms)
	}

	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ratchetPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Logf("wrote %s with %d cases", ratchetPath, len(keys))
}

func TestConformanceICC(t *testing.T) {
	root := conformanceRoot(t)

	paths, err := filepath.Glob(filepath.Join(root, "testcases", "*", "original.icc"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Skip("no original.icc in the corpus")
	}

	for _, path := range paths {
		dir := filepath.Dir(path)

		t.Run(filepath.Base(dir), func(t *testing.T) {
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			data, err := os.ReadFile(filepath.Join(dir, "input.jxl"))
			if err != nil {
				t.Fatal(err)
			}

			d, err := newDecoder(bytes.NewReader(data), Options{Level: 10})
			if err != nil {
				t.Fatal(err)
			}

			if !d.hdr.meta.color.wantICC {
				t.Skip("enumerated colour encoding: the ICC is generated, not decoded")
			}

			if len(d.iccp) != len(want) {
				t.Fatalf("decoded %d bytes, want %d", len(d.iccp), len(want))
			}
			for i := range want {
				if d.iccp[i] != want[i] {
					t.Fatalf("byte %d is %#02x, want %#02x", i, d.iccp[i], want[i])
				}
			}

			t.Logf("%d bytes, byte-exact", len(want))
		})
	}
}

// corpusRoot is the image corpus, which keeps libjxl's own decodes in reference.
func corpusRoot(t *testing.T) string {
	t.Helper()

	for _, dir := range conformanceDirs(t) {
		if _, err := os.Stat(filepath.Join(dir, "reference")); err == nil {
			return dir
		}
	}

	t.Skip("no image corpus in CONFORMANCE_DIR")

	return ""
}

// corpusUnsupported are the corpus files this decoder knowingly refuses, with
// the reason. Every other file has to decode, so the set only ever shrinks.
var corpusUnsupported = map[string]string{
	"features/ec_resampling_2x.jxl": "extra channel upsampled independently of the colour channels",
	"features/ec_resampling_4x.jxl": "extra channel upsampled independently of the colour channels",
	"photographic/candle.jxl":       "float16 samples",
}

// corpusTolerance is the largest difference from libjxl's own output this
// corpus reaches, in fractions of full scale. Two lossy files with alpha reach
// 2/255 because a one step alpha difference is amplified by the premultiply in
// color.Color.RGBA; every other file is within one step.
const corpusTolerance = 0.01

var corpusSubdirs = []string{"conformance", "features", "photographic", "edge-cases"}

// TestCorpus decodes an external corpus and compares every file against the
// reference outputs libjxl produced from it. Set CONFORMANCE_DIR to run it.
func TestCorpus(t *testing.T) {
	root := corpusRoot(t)

	var exact, near, refused, total int

	for _, sub := range corpusSubdirs {
		files, err := filepath.Glob(filepath.Join(root, sub, "*.jxl"))
		if err != nil {
			t.Fatal(err)
		}

		for _, path := range files {
			name := sub + "/" + filepath.Base(path)
			total++

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			got, err := Decode(bytes.NewReader(data), Options{Level: 10})
			if err != nil {
				why, known := corpusUnsupported[name]
				if !known {
					t.Errorf("%s: %v", name, err)

					continue
				}

				refused++

				t.Logf("%s: refused, %s", name, why)

				continue
			}

			ref := filepath.Join(root, "reference", sub,
				strings.TrimSuffix(filepath.Base(path), ".jxl")+".png")

			worst, err := peakAgainstPNG(got, ref)
			if err != nil {
				t.Logf("%s: no reference (%v)", name, err)

				continue
			}

			switch {
			case worst == 0:
				exact++
			case worst <= corpusTolerance:
				near++
			default:
				t.Errorf("%s: differs from libjxl by %g", name, worst)
			}
		}
	}

	if total == 0 {
		t.Skip("no corpus files present")
	}

	t.Logf("%d files: %d exact, %d within %g, %d knowingly refused",
		total, exact, near, corpusTolerance, refused)

	if refused != len(corpusUnsupported) {
		t.Errorf("%d files refused, the known set has %d", refused, len(corpusUnsupported))
	}
}
