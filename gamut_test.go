package jxl

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGamutRec2020(t *testing.T) {
	c := defaultColorEncoding()
	c.prim = prBT2100
	m, identity, ok := c.srgbGamut()
	if !ok || identity {
		t.Fatal("missing gamut matrix")
	}
	r, g, b := m.apply(1, 0, 0)
	for i, got := range []float32{r, g, b} {
		want := []float64{0.627403895934699, 0.069097289358232, 0.0163914388751503}[i]
		if math.Abs(float64(got)-want) > 1e-7 {
			t.Errorf("red primary channel%d=%g want%g", i, got, want)
		}
	}
	r, g, b = m.apply(1, 1, 1)
	for _, v := range []float32{r, g, b} {
		if math.Abs(float64(v)-1) > 1e-7 {
			t.Fatal("white was not preserved")
		}
	}
	c.prim = prCustom
	if _, _, ok := c.srgbGamut(); ok {
		t.Fatal("singular primaries accepted")
	}
}

// Optional real camera regression. Tiles carry Rec.2020 linear but XYB's
// inverse opsin initially produces linear sRGB. Before the gamut fix these
// differed from djxl by up to 0.14 despite reporting Rec.2020 metadata.
func TestCameraXYBGamut(t *testing.T) {
	root := os.Getenv("JXL_CAMERA_CORPUS")
	if root == "" {
		t.Skip("set JXL_CAMERA_CORPUS to extracted DNG tiles")
	}
	if _, err := exec.LookPath("djxl"); err != nil {
		t.Skip("djxl unavailable")
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.jxl"))
	if err != nil || len(paths) == 0 {
		t.Fatal("camera tiles unavailable")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got, err := DecodeFloat(bytes.NewReader(data), Options{Level: 10, Threads: 1})
			if err != nil {
				t.Fatal(err)
			}
			refpath := filepath.Join(t.TempDir(), "oracle.npy")
			if b, err := exec.Command("djxl", path, refpath, "--quiet", "--num_threads=0").CombinedOutput(); err != nil {
				t.Fatalf("djxl:%v:%s", err, b)
			}
			ref, err := readNPY(refpath)
			if err != nil {
				t.Fatal(err)
			}
			if got.Width != ref.width || got.Height != ref.height || got.Channels != ref.channels || len(got.Frames) != ref.frames {
				t.Fatal("shape mismatch")
			}
			for i, frame := range got.Frames {
				peak, rms := compareFrame(frame, ref.frame(i), ref.channels)
				if peak > 1e-5 || rms > 3e-6 {
					t.Fatalf("peak%g rms%g", peak, rms)
				}
				t.Logf("peak=%g max-channel RMS=%g", peak, rms)
			}
		})
	}
}
