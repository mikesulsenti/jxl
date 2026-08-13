package jxl

import (
	"math/rand/v2"
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
