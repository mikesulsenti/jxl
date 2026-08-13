package jxl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func jxlinfoPath(t *testing.T) string {
	t.Helper()

	for _, p := range []string{
		"/temp/jxl/libjxl/build/tools/jxlinfo",
		"jxlinfo",
	} {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}

	t.Skip("jxlinfo not found")

	return ""
}

func jxlinfoFields(t *testing.T, bin, path string) map[string]string {
	t.Helper()

	out, err := exec.Command(bin, "-v", path).CombinedOutput()
	if err != nil {
		t.Fatalf("jxlinfo: %v\n%s", err, out)
	}

	fields := map[string]string{}
	prefix := ""

	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "Extra channel ") {
			prefix = strings.TrimSuffix(trimmed, ":") + " "

			continue
		}
		if trimmed != "" && !strings.HasPrefix(line, " ") {
			prefix = ""
		}
		for _, kind := range []string{"JPEG XL image, ", "JPEG XL animation, "} {
			if rest, ok := strings.CutPrefix(trimmed, kind); ok {
				fields["dimensions"] = strings.SplitN(rest, ", ", 2)[0]
			}
		}

		k, v, ok := strings.Cut(trimmed, ": ")
		if !ok {
			continue
		}

		fields[prefix+k] = v
	}

	return fields
}

var (
	colorSpaceNames = map[colorSpace]string{
		csRGB: "RGB", csGray: "Grayscale", csXYB: "XYB", csUnknown: "Unknown",
	}
	whitePointNames = map[whitePoint]string{
		wpD65: "D65", wpCustom: "Custom", wpE: "E", wpDCI: "DCI",
	}
	primariesNames = map[primaries]string{
		prSRGB: "sRGB", prCustom: "Custom", prBT2100: "Rec.2100", prP3: "P3",
	}
	transferNames = map[transferFunction]string{
		tfBT709: "709", tfUnknown: "Unknown", tfLinear: "Linear",
		tfSRGB: "sRGB", tfPQ: "PQ", tfDCI: "DCI", tfHLG: "HLG",
	}
	intentNames = map[renderingIntent]string{
		riPerceptual: "Perceptual", riRelative: "Relative",
		riSaturation: "Saturation", riAbsolute: "Absolute",
	}
	extraChannelNames = map[extraChannelType]string{
		ecAlpha: "Alpha", ecDepth: "Depth", ecSpotColor: "Spotcolor",
		ecSelectionMask: "Selection", ecBlack: "Black", ecCFA: "CFA",
		ecThermal: "Thermal", ecUnknown: "Unknown", ecOptional: "Optional",
	}
)

func TestHeaderCorpus(t *testing.T) {
	root := conformanceRoot(t)
	bin := jxlinfoPath(t)

	paths, err := filepath.Glob(filepath.Join(root, "testcases", "*", "input.jxl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no inputs found")
	}

	var checked int

	for _, path := range paths {
		name := filepath.Base(filepath.Dir(path))

		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			d, err := newDecoder(data, Options{Level: 10})
			if err != nil {
				t.Fatal(err)
			}

			want := jxlinfoFields(t, bin, path)
			m := &d.hdr.meta

			eq := func(key, got string) {
				t.Helper()

				w, ok := want[key]
				if !ok {
					t.Errorf("jxlinfo reported no %q", key)

					return
				}
				if w != got {
					t.Errorf("%s = %q, jxlinfo says %q", key, got, w)
				}
			}

			eq("dimensions", fmt.Sprintf("%dx%d", d.hdr.size.xsize, d.hdr.size.ysize))
			eq("Number of color channels", strconv.Itoa(m.colorChannels()))
			eq("Number of extra channels", strconv.Itoa(len(m.extraChannels)))
			eq("Have preview", boolField(m.havePreview))
			eq("Have animation", boolField(m.haveAnimation))
			eq("Orientation", fmt.Sprintf("%d (%s)", m.orient, orientationName(m.orient)))

			if !m.color.wantICC {
				eq("Color space", colorSpaceNames[m.color.space])
				eq("Rendering intent", intentNames[m.color.renderingIntent])

				if m.color.space != csXYB {
					eq("White point", whitePointNames[m.color.white])
				}
				if m.color.space != csXYB && m.color.space != csGray {
					eq("Primaries", primariesNames[m.color.prim])
				}
				if m.color.tf.haveGamma {
					eq("Transfer function", fmt.Sprintf("gamma(%f)", m.color.tf.gammaValue()))
				} else {
					eq("Transfer function", transferNames[m.color.tf.tf])
				}
			}

			for i, ec := range m.extraChannels {
				p := fmt.Sprintf("Extra channel %d ", i)

				eq(p+"type", extraChannelNames[ec.ecType])
				eq(p+"bits per sample", strconv.Itoa(int(ec.depth.bitsPerSample)))

				if ec.ecType == ecAlpha {
					eq(p+"alpha premultiplied", fmt.Sprintf("%d (%s)",
						btoi(ec.alphaAssociated), premultipliedName(ec.alphaAssociated)))
				}
			}

			if m.haveAnimation {
				eq("Ticks per second (numerator / denominator)",
					fmt.Sprintf("%d / %d", m.anim.tpsNumerator, m.anim.tpsDenominator))
				eq("Num loops", strconv.Itoa(int(m.anim.numLoops)))
				eq("Have timecodes", boolField(m.anim.haveTimecodes))
			}

			checked++
		})
	}

	t.Logf("%d of %d inputs checked against jxlinfo", checked, len(paths))
}

func boolField(b bool) string { return strconv.Itoa(btoi(b)) }

func btoi(b bool) int {
	if b {
		return 1
	}

	return 0
}

func premultipliedName(b bool) string {
	if b {
		return "Premultiplied"
	}

	return "Non-premultiplied"
}

func orientationName(o orientation) string {
	return map[orientation]string{
		orIdentity: "Normal", orFlipHorizontal: "Flipped horizontally",
		orRotate180: "Upside down", orFlipVertical: "Flipped vertically",
		orTranspose: "Transposed", orRotate90CW: "90 degrees clockwise",
		orAntiTranspose: "Anti-Transposed",
		orRotate90CCW:   "90 degrees counter-clockwise",
	}[o]
}
