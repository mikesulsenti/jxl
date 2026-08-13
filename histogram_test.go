package jxl

import (
	"math/rand/v2"
	"testing"
)

func hybridConfigs(logAlphaSize int) []hybridUint {
	var out []hybridUint

	for se := 0; se <= logAlphaSize; se++ {
		for msb := 0; msb <= se; msb++ {
			for lsb := 0; lsb+msb <= se; lsb++ {
				if se == logAlphaSize && (msb != 0 || lsb != 0) {
					continue
				}

				out = append(out, newHybridUint(uint32(se), uint32(msb), uint32(lsb)))
			}
		}
	}

	return out
}

func TestHybridUintRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(41, 43))

	values := []uint32{0, 1, 2, 3, 7, 8, 255, 256, 4095, 65535, 1 << 20, 1<<31 - 1, 1<<32 - 1}

	for range 200 {
		values = append(values, rng.Uint32()>>uint(rng.IntN(32)))
	}

	cases := 0

	for _, logAlphaSize := range []int{5, 6, 7, 8, huffmanMaxBits} {
		for _, cfg := range hybridConfigs(logAlphaSize) {
			var head bitWriter

			cfg.write(&head, logAlphaSize)

			if err := head.err(); err != nil {
				t.Fatalf("%v log %d: config write gave %v", cfg, logAlphaSize, err)
			}

			var got hybridUint

			got.read(newBitReader(head.bytes()), logAlphaSize)

			if got != cfg {
				t.Fatalf("log %d: config %v read back as %v", logAlphaSize, cfg, got)
			}

			for _, v := range values {
				token, nbits, extra := cfg.encode(v)

				var body bitWriter

				body.bits(int(nbits), uint64(extra))

				if err := body.err(); err != nil {
					t.Fatalf("%v: value %d gave %v", cfg, v, err)
				}

				r := newBitReader(append(body.bytes(), 0, 0, 0, 0, 0, 0, 0, 0))

				if back := cfg.decode(token, r); back != v {
					t.Fatalf("%v: value %d encoded to token %d extra %d/%d bits, decoded to %d",
						cfg, v, token, extra, nbits, back)
				}
			}

			cases++
		}
	}

	t.Logf("%d hybrid uint configurations round-tripped", cases)
}

func TestContextMapRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(51, 53))

	for _, clusters := range []int{1, 2, 3, 5, 8} {
		for _, n := range []int{1, 2, 9, 64, 300} {
			if n < clusters {
				continue
			}

			m := make([]uint8, n)

			for i := range m {
				m[i] = uint8(rng.IntN(clusters))
			}

			for i := range clusters {
				m[i] = uint8(i)
			}

			var w bitWriter

			writeContextMap(&w, m)

			if err := w.err(); err != nil {
				t.Fatalf("clusters %d n %d: write gave %v", clusters, n, err)
			}

			r := newBitReader(w.bytes())

			got := decodeContextMap(r, n)

			if err := r.err(); err != nil {
				t.Fatalf("clusters %d n %d: read gave %v", clusters, n, err)
			}

			for i := range m {
				if got[i] != m[i] {
					t.Fatalf("clusters %d n %d: entry %d read back as %d, want %d",
						clusters, n, i, got[i], m[i])
				}
			}
		}
	}
}

func TestContextMapRejected(t *testing.T) {
	for _, m := range [][]uint8{{0, 2}, {1}, {0, 1, 3}, {0, 1, 2, 3, 4, 5, 6, 7, 8}} {
		var w bitWriter

		writeContextMap(&w, m)

		if w.err() == nil {
			t.Fatalf("%v was accepted", m)
		}
	}
}

func randomValues(rng *rand.Rand, n, kind int) []uint32 {
	out := make([]uint32, n)

	for i := range out {
		switch kind {
		case 0:
			out[i] = uint32(rng.IntN(4))
		case 1:
			out[i] = uint32(rng.IntN(256))
		case 2:
			out[i] = rng.Uint32() >> uint(2+rng.IntN(28))
		case 3:
			out[i] = uint32(rng.IntN(11)) * uint32(rng.IntN(11))
		case 4:
			out[i] = rng.Uint32()
		case 5:
			if rng.IntN(10) == 0 {
				out[i] = uint32(rng.IntN(1 << 20))
			}
		case 6:
			out[i] = uint32(rng.IntN(3)) << uint(4*rng.IntN(7))
		case 7:
			out[i] = uint32(rng.NormFloat64()*3000 + 40000)
		case 8:
			if rng.IntN(2) == 0 {
				out[i] = uint32(rng.IntN(8))
			} else {
				out[i] = uint32(rng.IntN(1 << 24))
			}
		}
	}

	return out
}

func TestHistogramRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(61, 67))

	for _, numContexts := range []int{1, 2, 7, 40} {
		for _, clusters := range []int{1, 2, 5, 8} {
			if clusters > numContexts {
				continue
			}

			for kind := range 9 {
				contextMap := make([]uint8, numContexts)

				for i := range contextMap {
					contextMap[i] = uint8(rng.IntN(clusters))
				}

				for i := range clusters {
					contextMap[i] = uint8(i)
				}

				type item struct {
					context int
					value   uint32
				}

				stream := make([]item, 3000)
				values := make([][]uint32, clusters)

				for i := range stream {
					c := rng.IntN(numContexts)
					v := randomValues(rng, 1, kind)[0]
					stream[i] = item{c, v}
					values[contextMap[c]] = append(values[contextMap[c]], v)
				}

				h, counts := buildHistograms(contextMap, values)

				var w bitWriter

				codes := h.write(&w, counts)

				if err := w.err(); err != nil {
					t.Fatalf("contexts %d clusters %d kind %d: header gave %v",
						numContexts, clusters, kind, err)
				}

				sw := symbolWriter{codes: codes}

				for _, it := range stream {
					sw.writeUnsigned(h, &w, it.context, it.value)
				}

				if err := w.err(); err != nil {
					t.Fatalf("contexts %d clusters %d kind %d: body gave %v",
						numContexts, clusters, kind, err)
				}

				r := newBitReader(append(w.bytes(), 0, 0, 0, 0, 0, 0, 0, 0))

				var back histograms

				back.read(r, numContexts, false)

				if err := r.err(); err != nil {
					t.Fatalf("contexts %d clusters %d kind %d: header read gave %v",
						numContexts, clusters, kind, err)
				}

				var sr symbolReader

				sr.init(&back, r, 0)

				for i, it := range stream {
					if got := sr.readUnsigned(&back, r, it.context); got != it.value {
						t.Fatalf("contexts %d clusters %d kind %d: item %d read back as %d, want %d",
							numContexts, clusters, kind, i, got, it.value)
					}
				}

				if err := sr.checkFinalState(&back, r); err != nil {
					t.Fatalf("contexts %d clusters %d kind %d: final state %v",
						numContexts, clusters, kind, err)
				}
			}
		}
	}
}

func TestSignedRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(81, 83))

	stream := []int32{0, 1, -1, 2, -2, 127, -128, 1 << 30, -1 << 30, -1<<31 + 1, 1<<31 - 1}

	for range 2000 {
		stream = append(stream, int32(rng.Uint32()>>uint(rng.IntN(31)))*(1-2*int32(rng.IntN(2))))
	}

	values := make([]uint32, len(stream))

	for i, v := range stream {
		values[i] = packSigned(v)
	}

	h, counts := buildHistograms([]uint8{0}, [][]uint32{values})

	var w bitWriter

	sw := symbolWriter{codes: h.write(&w, counts)}

	for _, v := range stream {
		sw.writeSigned(h, &w, 0, v)
	}

	if err := w.err(); err != nil {
		t.Fatalf("write gave %v", err)
	}

	r := newBitReader(append(w.bytes(), 0, 0, 0, 0, 0, 0, 0, 0))

	var back histograms

	back.read(r, 1, false)

	var sr symbolReader

	sr.init(&back, r, 0)

	for i, v := range stream {
		if got := sr.readSigned(&back, r, 0); got != v {
			t.Fatalf("item %d read back as %d, want %d", i, got, v)
		}
	}

	if err := sr.checkFinalState(&back, r); err != nil {
		t.Fatalf("final state %v", err)
	}
}

func encodedBits(t *testing.T, cfg hybridUint, values []uint32) (int, bool) {
	t.Helper()

	maxToken := uint32(0)

	for _, v := range values {
		token, _, _ := cfg.encode(v)

		if token >= 4096 {
			return 0, false
		}

		maxToken = max(maxToken, token)
	}

	counts := make([]uint32, maxToken+1)

	for _, v := range values {
		token, _, _ := cfg.encode(v)
		counts[token]++
	}

	var w bitWriter

	codes := writePrefixCode(&w, counts, int(maxToken)+1)

	for _, v := range values {
		token, nbits, extra := cfg.encode(v)

		w.symbol(codes[token])
		w.bits(int(nbits), uint64(extra))
	}

	if err := w.err(); err != nil {
		t.Fatalf("%v: %v", cfg, err)
	}

	return len(w.bytes()) * 8, true
}

func TestUintConfigChoice(t *testing.T) {
	rng := rand.New(rand.NewPCG(71, 73))

	worst := 0.0

	for kind := range 9 {
		for range 3 {
			values := randomValues(rng, 4000, kind)
			distinct, weights := valueHistogram(values)

			chosen := chooseUintConfig(distinct, weights, huffmanMaxBits, 1<<huffmanMaxBits-1)

			got, ok := encodedBits(t, chosen, values)
			if !ok {
				continue
			}

			best := got

			for _, cfg := range hybridConfigs(huffmanMaxBits) {
				if n, ok := encodedBits(t, cfg, values); ok {
					best = min(best, n)
				}
			}

			over := float64(got-best) / float64(best)
			worst = max(worst, over)

			if over > 0.0025 {
				t.Fatalf("kind %d: chose %v at %d bits, best is %d, %.1f%% over",
					kind, chosen, got, best, over*100)
			}
		}
	}

	t.Logf("chosen configuration is at most %.2f%% over the exhaustive best", worst*100)
}
