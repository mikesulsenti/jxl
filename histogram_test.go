package jxl

import (
	"math/rand/v2"
	"slices"
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

				if got := cfg.extraBits(token); got != int(nbits) {
					t.Fatalf("%v: value %d has %d extra bits, token %d says %d",
						cfg, v, nbits, token, got)
				}

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

	for _, clusters := range []int{1, 2, 3, 5, 8, 9, 40, 255} {
		for _, n := range []int{1, 2, 9, 64, 300, 1000} {
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
	for _, m := range [][]uint8{{0, 2}, {1}, {0, 1, 3}, {0, 1, 2, 4}} {
		var w bitWriter

		writeContextMap(&w, m)

		if w.err() == nil {
			t.Fatalf("%v was accepted", m)
		}
	}
}

func TestContextMapForms(t *testing.T) {
	rng := rand.New(rand.NewPCG(121, 127))

	forms := map[string]int{}

	for _, clusters := range []int{2, 8, 9, 200} {
		for _, n := range []int{4, 50, 2000} {
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
				t.Fatalf("clusters %d n %d: %v", clusters, n, err)
			}

			r := newBitReader(w.bytes())

			if r.bit() {
				forms["simple"]++
			} else if r.bit() {
				forms["coded with move to front"]++
			} else {
				forms["coded"]++
			}
		}
	}

	if len(forms) < 3 {
		t.Fatalf("only these forms were ever chosen: %v", forms)
	}

	t.Logf("context map forms chosen: %v", forms)
}

func countValues(values ...[]uint32) []valueCounts {
	out := make([]valueCounts, len(values))

	for i, vs := range values {
		for _, v := range vs {
			out[i].add(v)
		}
	}

	return out
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
				values := make([]valueCounts, clusters)

				for i := range stream {
					c := rng.IntN(numContexts)
					v := randomValues(rng, 1, kind)[0]
					stream[i] = item{c, v}
					values[contextMap[c]].add(v)
				}

				h, counts := buildHistograms(contextMap, values)

				var w bitWriter

				codes := h.write(&w, counts)

				if err := w.err(); err != nil {
					t.Fatalf("contexts %d clusters %d kind %d: header gave %v",
						numContexts, clusters, kind, err)
				}

				sw := codes

				for _, it := range stream {
					sw.writeUnsigned(h, &w, it.context, it.value)
				}

				sw.flush(&w)

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

	h, counts := buildHistograms([]uint8{0}, countValues(values))

	var w bitWriter

	sw := h.write(&w, counts)

	for _, v := range stream {
		sw.writeSigned(h, &w, 0, v)
	}

	sw.flush(&w)

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

	codes := writePrefixCode(&w, counts, int(maxToken)+1, &prefixScratch{})

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
			hist := countValues(values)[0].entries(nil)

			chosen := chooseUintConfig(hist, huffmanMaxBits, 1<<huffmanMaxBits-1, &uintCostScratch{})

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

func clusterCost(m []uint8, merged []valueCounts) float64 {
	total := 0.0

	var hist []histEntry

	for i := range merged {
		if merged[i].total == 0 {
			continue
		}

		hist = merged[i].entries(hist[:0])

		cost := xLogX(uint64(merged[i].total)) + tableCost(len(hist))

		for _, e := range hist {
			cost -= xLogX(uint64(e.count))
		}

		total += cost
	}

	return total
}

func rankedClusters(values []valueCounts, limit int) ([]uint8, []valueCounts) {
	type ranked struct {
		index, count int
	}

	var used []ranked

	for i := range values {
		if values[i].total > 0 {
			used = append(used, ranked{i, int(values[i].total)})
		}
	}

	slices.SortFunc(used, func(a, b ranked) int {
		if a.count != b.count {
			return b.count - a.count
		}

		return a.index - b.index
	})

	m := make([]uint8, len(values))
	clusters := min(len(used), limit)
	merged := make([]valueCounts, clusters)

	for rank, c := range used {
		k := min(rank, clusters-1)
		m[c.index] = uint8(k)
		merged[k].merge(&values[c.index])
	}

	return m, merged
}

func TestClusterContextsBeatsRanking(t *testing.T) {
	rng := rand.New(rand.NewPCG(151, 157))

	shapes := [][]uint32{
		{0, 0, 0, 0, 1, 1, 2},
		{5, 5, 6, 6, 7, 7, 8},
		{40, 41, 42, 43, 44, 45, 46},
		{100, 100, 100, 101, 101, 102, 103},
		{200, 201, 200, 201, 202, 203, 204},
	}

	values := make([]valueCounts, 600)

	for i := range values {
		s := shapes[i%len(shapes)]
		n := 4 + rng.IntN(60)

		for range n {
			values[i].add(s[rng.IntN(len(s))])
		}
	}

	m, merged := clusterContexts(values, 8)

	if len(merged) > 8 {
		t.Fatalf("got %d clusters, want at most 8", len(merged))
	}

	for i := range values {
		if int(m[i]) >= len(merged) {
			t.Fatalf("context %d maps to cluster %d of %d", i, m[i], len(merged))
		}
	}

	rm, rmerged := rankedClusters(values, 8)

	got, want := clusterCost(m, merged), clusterCost(rm, rmerged)

	t.Logf("similarity clustering costs %.0f bits, population ranking %.0f", got, want)

	if got >= want*0.75 {
		t.Fatalf("similarity clustering costs %.0f bits, ranking %.0f, expected a clear win",
			got, want)
	}
}

func TestANSDistributionRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(211, 223))

	for kind := range 9 {
		for _, n := range []int{1, 2, 3, 9, 60, 256} {
			for range 20 {
				counts := make([]uint32, n)

				for i := range counts {
					switch kind {
					case 0:
						counts[i] = uint32(rng.IntN(2))
					case 1:
						counts[i] = uint32(rng.IntN(1000))
					case 2:
						counts[i] = uint32(rng.IntN(1 << 20))
					default:
						if rng.IntN(kind) == 0 {
							counts[i] = uint32(rng.IntN(1 << uint(kind)))
						}
					}
				}

				table, ok := buildANSTable(counts, ansMaxLogAlpha)
				if !ok {
					t.Fatalf("kind %d n %d: no table", kind, n)
				}

				var w bitWriter

				writeANSHistogram(&w, table.dist, table.omit, table.shift)

				if err := w.err(); err != nil {
					t.Fatalf("kind %d n %d: write gave %v", kind, n, err)
				}

				r := newBitReader(append(w.bytes(), make([]byte, 16)...))

				var got ansHistogram

				var s ansScratch

				s.init(1, 1<<ansMaxLogAlpha)

				got.read(r, ansMaxLogAlpha, &s)

				if err := r.err(); err != nil {
					t.Fatalf("kind %d n %d shift %d: read gave %v, counts %v dist %v omit %d",
						kind, n, table.shift, err, counts, table.dist, table.omit)
				}

				freq := got.frequencies(1 << ansMaxLogAlpha)

				for i, d := range freq {
					want := uint16(0)
					if i < len(table.dist) {
						want = table.dist[i]
					}

					if d != want {
						t.Fatalf("kind %d n %d shift %d: symbol %d has frequency %d, want %d",
							kind, n, table.shift, i, d, want)
					}
				}
			}
		}
	}
}

func TestANSTiny(t *testing.T) {
	for _, vals := range [][]uint32{
		{0}, {0, 0, 1}, {5}, {0, 1, 2}, {3, 3, 3, 4}, {0, 0, 0, 0, 1, 7, 7},
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, {0, 1}, {2, 9},
	} {
		counted := countValues(vals)

		h, counts := codeHistograms(singleContext[:], counted, ansMaxLogAlpha, false)
		if h == nil {
			t.Fatalf("%v: no ans histogram", vals)
		}

		var w bitWriter

		sw := h.write(&w, counts)

		if err := w.err(); err != nil {
			t.Fatalf("%v: header gave %v", vals, err)
		}

		for _, v := range vals {
			sw.writeUnsigned(h, &w, 0, v)
		}

		sw.flush(&w)

		if err := w.err(); err != nil {
			t.Fatalf("%v: body gave %v", vals, err)
		}

		r := newBitReader(append(w.bytes(), make([]byte, 16)...))

		var back histograms

		back.read(r, 1, false)

		if err := r.err(); err != nil {
			t.Fatalf("%v: header read gave %v", vals, err)
		}

		var sr symbolReader

		sr.init(&back, r, 0)

		for i, v := range vals {
			if got := sr.readUnsigned(&back, r, 0); got != v {
				t.Fatalf("%v: item %d read back as %d", vals, i, got)
			}
		}

		if err := sr.checkFinalState(&back, r); err != nil {
			t.Fatalf("%v: final state %v", vals, err)
		}
	}
}
