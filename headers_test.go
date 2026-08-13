package jxl

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

			d, err := newDecoder(bytes.NewReader(data), Options{Level: 10})
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

func TestFileHeaderRoundTrip(t *testing.T) {
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

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		d, err := newDecoder(bytes.NewReader(data), Options{Level: 10})
		if err != nil {
			continue
		}

		var w bitWriter

		d.hdr.write(&w)

		if err := w.err(); err != nil {
			t.Fatalf("%s: writing the header gave %v", path, err)
		}

		var got fileHeader

		r := newBitReader(w.bytes())
		got.read(r)

		if err := r.err(); err != nil {
			t.Fatalf("%s: rereading the header gave %v", path, err)
		}

		if !reflect.DeepEqual(got, d.hdr) {
			t.Fatalf("%s: header round-tripped to\n%+v\nwant\n%+v", path, got, d.hdr)
		}

		checked++
	}

	if checked == 0 {
		t.Fatal("no inputs produced a header")
	}

	t.Logf("%d headers round-tripped", checked)
}

func TestFileHeaderWriteCoverage(t *testing.T) {
	base := fileHeader{
		size:      sizeHeader{xsize: 640, ysize: 480},
		meta:      defaultImageMetadata(),
		transform: defaultCustomTransformData(),
	}

	var cases []fileHeader

	add := func(f func(h *fileHeader)) {
		h := base
		h.meta.extraChannels = nil
		f(&h)
		cases = append(cases, h)
	}

	add(func(h *fileHeader) {})

	for _, s := range [][2]uint32{{1, 1}, {7, 13}, {640, 480}, {800, 800}, {1920, 1080},
		{4096, 4096}, {65535, 1}, {1, 65535}, {123456, 654321}} {
		add(func(h *fileHeader) { h.size = sizeHeader{xsize: s[0], ysize: s[1]} })
	}

	for o := orIdentity; o <= orRotate90CCW; o++ {
		add(func(h *fileHeader) { h.meta.orient = o })
	}

	for _, n := range []uint32{0, 1, 7, 8, 65535, 1 << 20} {
		add(func(h *fileHeader) {
			h.meta.haveAnimation = true
			h.meta.anim = animation{tpsNumerator: 100, tpsDenominator: 1, numLoops: n}
		})
	}

	add(func(h *fileHeader) {
		h.meta.haveAnimation = true
		h.meta.anim = animation{tpsNumerator: 77, tpsDenominator: 1001, numLoops: 3, haveTimecodes: true}
	})

	for _, p := range [][2]uint32{{128, 128}, {256, 256}, {200, 200}, {100, 60}, {33, 17},
		{1, 1}, {2560, 1440}} {
		add(func(h *fileHeader) {
			h.meta.havePreview = true
			h.meta.preview = previewHeader{xsize: p[0], ysize: p[1]}
		})
	}

	add(func(h *fileHeader) {
		h.meta.haveIntrinsicSize = true
		h.meta.intrinsicSize = sizeHeader{xsize: 320, ysize: 200}
	})

	for _, b := range []bitDepth{
		{bitsPerSample: 8}, {bitsPerSample: 10}, {bitsPerSample: 12},
		{bitsPerSample: 16}, {bitsPerSample: 31},
		{floatSample: true, bitsPerSample: 32, expBits: 8},
		{floatSample: true, bitsPerSample: 16, expBits: 5},
	} {
		add(func(h *fileHeader) { h.meta.depth = b })
	}

	for _, e := range []extraChannelInfo{
		defaultExtraChannelInfo(),
		{ecType: ecAlpha, depth: defaultBitDepth(), alphaAssociated: true, cfaChannel: 1},
		{ecType: ecDepth, depth: defaultBitDepth(), dimShift: 3, name: "depth", cfaChannel: 1},
		{ecType: ecDepth, depth: defaultBitDepth(), dimShift: 1, cfaChannel: 1},
		{ecType: ecDepth, depth: defaultBitDepth(), dimShift: 2, cfaChannel: 1},
		{ecType: ecSpotColor, depth: defaultBitDepth(), cfaChannel: 1,
			spotColor: [4]float32{0.5, 0.25, 0.125, 1}},
		{ecType: ecCFA, depth: defaultBitDepth(), cfaChannel: 19},
		{ecType: ecOptional, depth: bitDepth{bitsPerSample: 16}, name: "a name", cfaChannel: 1},
	} {
		add(func(h *fileHeader) { h.meta.extraChannels = []extraChannelInfo{e} })
	}

	add(func(h *fileHeader) {
		h.meta.extraChannels = []extraChannelInfo{
			defaultExtraChannelInfo(),
			{ecType: ecCFA, depth: defaultBitDepth(), cfaChannel: 4},
		}
	})

	for _, c := range []colorEncoding{
		defaultColorEncoding(),
		{space: csGray, white: wpD65, prim: prSRGB, tf: defaultCustomTF(), renderingIntent: riRelative},
		{space: csRGB, white: wpE, prim: prBT2100, tf: defaultCustomTF(), renderingIntent: riRelative},
		{space: csRGB, white: wpDCI, prim: prP3, tf: defaultCustomTF(), renderingIntent: riAbsolute},
		{space: csRGB, white: wpD65, prim: prSRGB, tf: customTF{gamma: gammaMul / 3, tf: tfPQ}, renderingIntent: riSaturation},
		{space: csRGB, white: wpD65, prim: prSRGB, tf: customTF{gamma: gammaMul / 3, tf: tfLinear}, renderingIntent: riPerceptual},
		{space: csRGB, white: wpD65, prim: prSRGB, tf: customTF{gamma: gammaMul / 3, tf: tfHLG}, renderingIntent: riRelative},
		{space: csRGB, white: wpD65, prim: prSRGB, tf: customTF{haveGamma: true, gamma: 2200000, tf: tfSRGB},
			renderingIntent: riRelative},
		{space: csRGB, white: wpCustom, whiteXY: customXY{x: 31271, y: 32902},
			prim: prCustom, customPrim: [3]customXY{{64000, 33000}, {30000, 60000}, {-15000, 6000}},
			tf: defaultCustomTF(), renderingIntent: riRelative},
		{wantICC: true, space: csRGB, white: wpD65, prim: prSRGB, tf: defaultCustomTF(),
			renderingIntent: riRelative},
	} {
		add(func(h *fileHeader) { h.meta.color = c })
	}

	add(func(h *fileHeader) {
		h.meta.tone = toneMapping{intensityTarget: 1000, minNits: 0.5,
			relativeToMaxDisplay: true, linearBelow: 0.25}
	})

	add(func(h *fileHeader) {
		for i := range h.transform.opsin.inverse {
			h.transform.opsin.inverse[i] = float32(i) * 0.5
		}

		for i := range h.transform.opsin.biases {
			h.transform.opsin.biases[i] = -0.25
		}

		for i := range h.transform.opsin.quant {
			h.transform.opsin.quant[i] = 0.75
		}
	})

	add(func(h *fileHeader) {
		for i := range h.transform.weights2 {
			h.transform.weights2[i] = float32(i) / 64
		}
	})

	add(func(h *fileHeader) {
		for i := range h.transform.weights4 {
			h.transform.weights4[i] = float32(i) / 128
		}

		for i := range h.transform.weights8 {
			h.transform.weights8[i] = -float32(i) / 256
		}
	})

	for i, want := range cases {
		var w bitWriter

		want.write(&w)

		if err := w.err(); err != nil {
			t.Fatalf("case %d: write gave %v (%+v)", i, err, want.meta)
			continue
		}

		var got fileHeader

		r := newBitReader(w.bytes())
		got.read(r)

		if err := r.err(); err != nil {
			t.Fatalf("case %d: read gave %v (%+v)", i, err, want.meta)
			continue
		}

		if len(want.meta.extraChannels) == 0 {
			want.meta.extraChannels = got.meta.extraChannels
		}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("case %d round-tripped to\n%+v\nwant\n%+v", i, got.meta, want.meta)
		}
	}

	t.Logf("%d synthetic headers round-tripped", len(cases))
}
