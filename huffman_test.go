package jxl

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestSimplePrefixRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 32))

	cases := 0

	for _, alSize := range []int{2, 3, 4, 17, 256, 1024} {
		for num := 1; num <= min(4, alSize); num++ {
			for _, special := range []bool{false, true} {
				if special && num != 4 {
					continue
				}

				for range 40 {
					symbols := make([]uint16, 0, num)

					for len(symbols) < num {
						s := uint16(rng.IntN(alSize))

						dup := false

						for _, o := range symbols {
							if o == s {
								dup = true
							}
						}

						if !dup {
							symbols = append(symbols, s)
						}
					}

					var w bitWriter

					writeSimplePrefix(&w, alSize, symbols, special)

					if err := w.err(); err != nil {
						t.Fatalf("alSize %d num %d: write gave %v", alSize, num, err)
					}

					head := w.bytes()

					var tab huffTable

					r := newBitReader(head)
					tab.read(r, alSize)

					if err := r.err(); err != nil {
						t.Fatalf("alSize %d num %d: read gave %v", alSize, num, err)
					}

					codes, ok := prefixCodes(&tab, alSize)
					if !ok {
						t.Fatalf("alSize %d num %d: codes do not fit the root table", alSize, num)
					}

					for _, s := range symbols {
						if codes[s].bits == 0 && num > 1 {
							t.Fatalf("alSize %d num %d: symbol %d has no code", alSize, num, s)
						}
					}

					var body bitWriter

					want := make([]uint16, 200)

					for i := range want {
						want[i] = symbols[rng.IntN(num)]
						body.symbol(codes[want[i]])
					}

					if err := body.err(); err != nil {
						t.Fatalf("alSize %d num %d: encoding gave %v", alSize, num, err)
					}

					br := newBitReader(body.bytes())

					for i, s := range want {
						if got := tab.decode(br); got != uint32(s) {
							t.Fatalf("alSize %d num %d special %v: symbol %d decoded to %d, want %d",
								alSize, num, special, i, got, s)
						}
					}

					if err := br.err(); err != nil {
						t.Fatalf("alSize %d num %d: decoding gave %v", alSize, num, err)
					}

					cases++
				}
			}
		}
	}

	t.Logf("%d simple prefix tables round-tripped", cases)
}

func randomCounts(rng *rand.Rand, alSize, kind int) []uint32 {
	counts := make([]uint32, alSize)

	switch kind {
	case 0:
		for i := range counts {
			counts[i] = 1
		}
	case 1:
		for i := range counts {
			counts[i] = uint32(rng.IntN(1000)) + 1
		}
	case 2:
		w := uint64(1) << 40

		for i := range counts {
			counts[i] = uint32(max(w>>uint(i), 1))
		}
	case 3:
		for i := range counts {
			if rng.IntN(6) == 0 {
				counts[i] = uint32(rng.IntN(50)) + 1
			}
		}
	case 4:
		for i := range counts {
			if i%37 == 0 || i < 3 {
				counts[i] = uint32(rng.IntN(9)) + 1
			}
		}
	case 5:
		for i := range counts {
			counts[i] = 7
		}

		for range alSize / 4 {
			counts[rng.IntN(alSize)] = 0
		}
	}

	return counts
}

func TestPrefixCodeRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))

	var complex, simple int

	for _, alSize := range []int{2, 5, 8, 15, 16, 64, 256, 300, 1024, 4096} {
		for kind := range 6 {
			for range 8 {
				counts := randomCounts(rng, alSize, kind)

				total := uint64(0)
				nonzero := 0

				for _, c := range counts {
					total += uint64(c)

					if c > 0 {
						nonzero++
					}
				}

				if total == 0 {
					continue
				}

				var w bitWriter

				codes := writePrefixCode(&w, counts, alSize)
				if err := w.err(); err != nil {
					t.Fatalf("alSize %d kind %d: write gave %v", alSize, kind, err)
				}

				if nonzero > 4 {
					complex++
				} else {
					simple++
				}

				var tab huffTable

				r := newBitReader(w.bytes())
				tab.read(r, alSize)

				if err := r.err(); err != nil {
					t.Fatalf("alSize %d kind %d: read gave %v", alSize, kind, err)
				}

				var body bitWriter

				want := make([]uint16, 500)

				for i := range want {
					pick := uint64(rng.Int64N(int64(total)))

					for s, c := range counts {
						if pick < uint64(c) {
							want[i] = uint16(s)

							break
						}

						pick -= uint64(c)
					}

					if codes[want[i]].bits == 0 && nonzero > 1 {
						t.Fatalf("alSize %d kind %d: symbol %d has no code", alSize, kind, want[i])
					}

					body.symbol(codes[want[i]])
				}

				if err := body.err(); err != nil {
					t.Fatalf("alSize %d kind %d: encoding gave %v", alSize, kind, err)
				}

				br := newBitReader(body.bytes())

				for i, s := range want {
					if got := tab.decode(br); got != uint32(s) {
						t.Fatalf("alSize %d kind %d: symbol %d decoded to %d, want %d",
							alSize, kind, i, got, s)
					}
				}

				if err := br.err(); err != nil {
					t.Fatalf("alSize %d kind %d: decoding gave %v", alSize, kind, err)
				}
			}
		}
	}

	t.Logf("%d complex and %d simple prefix tables round-tripped", complex, simple)
}

func huffmanCost(counts []uint32) uint64 {
	var w []uint64

	for _, c := range counts {
		if c > 0 {
			w = append(w, uint64(c))
		}
	}

	if len(w) < 2 {
		return 0
	}

	slices.Sort(w)

	q := make([]uint64, 0, len(w))
	i, j := 0, 0

	take := func() uint64 {
		if i < len(w) && (j >= len(q) || w[i] <= q[j]) {
			i++

			return w[i-1]
		}

		j++

		return q[j-1]
	}

	total := uint64(0)

	for range len(w) - 1 {
		n := take() + take()
		total += n
		q = append(q, n)
	}

	return total
}

func TestPrefixCodeOptimal(t *testing.T) {
	rng := rand.New(rand.NewPCG(23, 29))

	bound := 0

	for _, alSize := range []int{5, 8, 16, 64, 256, 1024} {
		for kind := range 6 {
			for range 4 {
				counts := randomCounts(rng, alSize, kind)

				nonzero := 0

				for _, c := range counts {
					if c > 0 {
						nonzero++
					}
				}

				if nonzero < 2 {
					continue
				}

				lengths := limitedLengths(counts, huffmanMaxBits)
				if lengths == nil {
					t.Fatalf("alSize %d kind %d: refused", alSize, kind)
				}

				cost, maxLen := uint64(0), uint8(0)

				for s, l := range lengths {
					cost += uint64(counts[s]) * uint64(l)
					maxLen = max(maxLen, l)
				}

				want := huffmanCost(counts)

				if cost < want {
					t.Fatalf("alSize %d kind %d: cost %d below the huffman optimum %d",
						alSize, kind, cost, want)
				}

				if maxLen < huffmanMaxBits && cost != want {
					t.Fatalf("alSize %d kind %d: cost %d, huffman optimum %d, longest code %d",
						alSize, kind, cost, want, maxLen)
				}

				if maxLen == huffmanMaxBits {
					bound++
				}
			}
		}
	}

	t.Logf("%d distributions reached the 15 bit limit", bound)
}

func TestLimitedLengths(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 17))

	for _, limit := range []int{5, 8, huffmanMaxBits} {
		for _, n := range []int{2, 3, 17, 1 << limit} {
			for kind := range 6 {
				counts := randomCounts(rng, n, kind)

				lengths := limitedLengths(counts, limit)
				if lengths == nil {
					t.Fatalf("limit %d n %d kind %d: refused", limit, n, kind)
				}

				space, nonzero := uint64(0), 0

				for s, l := range lengths {
					if counts[s] == 0 {
						if l != 0 {
							t.Fatalf("limit %d n %d kind %d: unused symbol %d got length %d",
								limit, n, kind, s, l)
						}

						continue
					}

					if l == 0 || int(l) > limit {
						t.Fatalf("limit %d n %d kind %d: symbol %d got length %d",
							limit, n, kind, s, l)
					}

					space += 1 << (limit - int(l))
					nonzero++
				}

				if nonzero > 1 && space != 1<<limit {
					t.Fatalf("limit %d n %d kind %d: kraft sum %d, want %d",
						limit, n, kind, space, 1<<limit)
				}
			}
		}
	}
}
