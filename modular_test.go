package jxl

import (
	"math/rand/v2"
	"testing"
)

func randomLeaf(rng *rand.Rand, allowWP bool) *treeSpec {
	p := uint8(rng.IntN(numPredictors))

	for !allowWP && p == predWeighted {
		p = uint8(rng.IntN(numPredictors))
	}

	return &treeSpec{property: -1, predictor: p, offset: int32(rng.IntN(9)) - 4, mul: 1}
}

func randomTree(rng *rand.Rand, leaves, maxProperty int, allowWP bool) *treeSpec {
	lo := make([]int32, maxProperty+1)
	hi := make([]int32, maxProperty+1)

	for i := range lo {
		lo[i], hi[i] = -1<<31, 1<<31-1
	}

	root := randomLeaf(rng, allowWP)
	open := []**treeSpec{&root}
	los := [][]int32{lo}
	his := [][]int32{hi}

	for n := 1; n < leaves; n++ {
		k := rng.IntN(len(open))
		slot, l, h := open[k], los[k], his[k]

		open = append(open[:k], open[k+1:]...)
		los = append(los[:k], los[k+1:]...)
		his = append(his[:k], his[k+1:]...)

		p := int32(rng.IntN(maxProperty + 1))

		width := int64(h[p]) - int64(l[p])
		if width < 1 {
			open = append(open, slot)
			los = append(los, l)
			his = append(his, h)
			n--

			continue
		}

		v := int32(int64(l[p]) + rng.Int64N(width))

		if l[p] == -1<<31 && h[p] == 1<<31-1 {
			v = int32(rng.IntN(17)) - 8
		}

		split := &treeSpec{
			property: p, val: v,
			left:  randomLeaf(rng, allowWP),
			right: randomLeaf(rng, allowWP),
		}

		*slot = split

		gl, gh := append([]int32(nil), l...), append([]int32(nil), h...)
		gl[p] = v + 1

		ll, lh := append([]int32(nil), l...), append([]int32(nil), h...)
		lh[p] = v

		open = append(open, &split.left, &split.right)
		los = append(los, gl, ll)
		his = append(his, gh, lh)
	}

	return root
}

func randomModImage(rng *rand.Rand, shapes [][2]int, amp int32) *modImage {
	m := &modImage{bitDepth: 8}

	for _, s := range shapes {
		c := newModChannel(s[0], s[1], 0, 0)

		for i := range c.data {
			c.data[i] = rng.Int32N(2*amp+1) - amp
		}

		m.channels = append(m.channels, c)
	}

	return m
}

func emptyLike(m *modImage) *modImage {
	out := &modImage{bitDepth: m.bitDepth}

	for _, c := range m.channels {
		out.channels = append(out.channels, newModChannel(c.w, c.h, c.hshift, c.vshift))
	}

	return out
}

func roundTripModular(t *testing.T, src *modImage, tree *maTree, streamID int, wp weightedHeader) error {
	t.Helper()

	var w bitWriter

	src.encodeStream(&w, &groupHeader{wp: wp}, tree, streamID)

	if err := w.err(); err != nil {
		return err
	}

	got := emptyLike(src)

	r := newBitReader(append(w.bytes(), 0, 0, 0, 0, 0, 0, 0, 0))

	got.decodeStream(r, nil, nil, streamID)

	if err := r.err(); err != nil {
		return err
	}

	for i, c := range src.channels {
		for j, v := range c.data {
			if got.channels[i].data[j] != v {
				t.Fatalf("channel %d sample %d read back as %d, want %d",
					i, j, got.channels[i].data[j], v)
			}
		}
	}

	return nil
}

func TestModularTrivialTree(t *testing.T) {
	rng := rand.New(rand.NewPCG(91, 97))

	shapes := [][2]int{{16, 12}, {8, 8}, {1, 5}, {5, 1}, {33, 3}}

	for _, amp := range []int32{0, 1, 7, 300, 1 << 20} {
		src := randomModImage(rng, shapes, amp)

		tree := &maTree{nodes: serializeTree(&treeSpec{
			property: -1, predictor: predGradient, mul: 1,
		})}

		if err := roundTripModular(t, src, tree, 3, defaultWeightedHeader()); err != nil {
			t.Fatalf("amplitude %d: %v", amp, err)
		}
	}
}

func TestModularRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(101, 103))

	shapes := [][2]int{{13, 9}, {13, 9}, {7, 4}, {13, 9}}

	cases, withWP, custom := 0, 0, 0

	for _, leaves := range []int{1, 2, 3, 5, 8} {
		for _, maxProperty := range []int{0, 5, 15, 19} {
			for _, amp := range []int32{1, 20, 5000} {
				for range 3 {
					src := randomModImage(rng, shapes, amp)

					tree := &maTree{
						nodes: serializeTree(randomTree(rng, leaves, maxProperty, true)),
					}

					tree.finish()

					if tree.usesWP {
						withWP++
					}

					wp := defaultWeightedHeader()

					if rng.IntN(2) == 0 {
						wp = weightedHeader{
							p1c: uint32(rng.IntN(32)), p2c: uint32(rng.IntN(32)),
							p3ca: uint32(rng.IntN(32)), p3cb: uint32(rng.IntN(32)),
							p3cc: uint32(rng.IntN(32)), p3cd: uint32(rng.IntN(32)),
							p3ce: uint32(rng.IntN(32)), w0: uint32(rng.IntN(16)),
							w1: uint32(rng.IntN(16)), w2: uint32(rng.IntN(16)),
							w3: uint32(rng.IntN(16)),
						}
						custom++
					}

					if err := roundTripModular(t, src, tree, 1, wp); err != nil {
						t.Fatalf("leaves %d property %d amplitude %d: %v",
							leaves, maxProperty, amp, err)
					}

					cases++
				}
			}
		}
	}

	t.Logf("%d modular streams round-tripped, %d using the weighted predictor, %d with a custom weighted header",
		cases, withWP, custom)
}

func TestModularMultiplier(t *testing.T) {
	rng := rand.New(rand.NewPCG(111, 113))

	for _, mul := range []uint32{1, 2, 4, 64, 1 << 20} {
		src := randomModImage(rng, [][2]int{{9, 7}, {4, 4}}, 1000)

		for _, c := range src.channels {
			for i := range c.data {
				c.data[i] *= int32(mul)
			}
		}

		tree := &maTree{nodes: serializeTree(&treeSpec{
			property: -1, predictor: predZero, mul: mul,
		})}

		if err := roundTripModular(t, src, tree, 0, defaultWeightedHeader()); err != nil {
			t.Fatalf("multiplier %d: %v", mul, err)
		}
	}
}

func TestModularManyLeaves(t *testing.T) {
	rng := rand.New(rand.NewPCG(131, 137))

	for _, leaves := range []int{9, 17, 40} {
		src := randomModImage(rng, [][2]int{{40, 30}, {40, 30}}, 300)

		tree := &maTree{nodes: serializeTree(randomTree(rng, leaves, 15, true))}

		if err := roundTripModular(t, src, tree, 1, defaultWeightedHeader()); err != nil {
			t.Fatalf("%d leaves: %v", leaves, err)
		}
	}
}

func TestModularRejected(t *testing.T) {
	src := randomModImage(rand.New(rand.NewPCG(1, 2)), [][2]int{{8, 8}}, 100)

	var w bitWriter

	src.encodeStream(&w, &groupHeader{
		wp:         defaultWeightedHeader(),
		transforms: []modTransform{{id: xformRCT, rctType: 6}},
	}, &maTree{nodes: serializeTree(&treeSpec{property: -1, predictor: predZero, mul: 1})}, 0)

	if w.err() != ErrUnsupported {
		t.Fatalf("a transform gave %v, want %v", w.err(), ErrUnsupported)
	}

	var w2 bitWriter

	src.encodeStream(&w2, &groupHeader{wp: defaultWeightedHeader()}, &maTree{nodes: serializeTree(&treeSpec{
		property: -1, predictor: predZero, mul: 64,
	})}, 0)

	if w2.err() != ErrInvalid {
		t.Fatalf("an indivisible residual gave %v, want %v", w2.err(), ErrInvalid)
	}
}

func TestModularDebug(t *testing.T) {
	rng := rand.New(rand.NewPCG(101, 103))

	src := randomModImage(rng, [][2]int{{16, 12}, {8, 8}, {1, 5}, {5, 1}, {33, 3}}, 0)

	tree := &maTree{nodes: serializeTree(&treeSpec{
		property: -1, predictor: predGradient, mul: 1,
	})}

	var w bitWriter

	src.encodeStream(&w, &groupHeader{wp: defaultWeightedHeader()}, tree, 3)

	t.Logf("%d bytes", len(w.bytes()))

	if err := w.err(); err != nil {
		t.Fatalf("write: %v", err)
	}

	r := newBitReader(append(w.bytes(), make([]byte, 16)...))

	var hdr groupHeader

	hdr.read(r)

	var back maTree

	back.read(r, 1024)

	t.Logf("tree read err %v, nodes %d want %d", r.err(), len(back.nodes), len(tree.nodes))

	got := emptyLike(src)

	var sr symbolReader

	sr.init(&back.histograms, r, 0)

	var sc modScratch

	wph := defaultWeightedHeader()

	for i := range got.channels {
		got.decodeChannel(r, &sr, &back, &wph, i, 3, &sc)
	}

	t.Logf("data read err %v final %v", r.err(), sr.checkFinalState(&back.histograms, r))

	for i, c := range src.channels {
		for j, v := range c.data {
			if got.channels[i].data[j] != v {
				t.Fatalf("channel %d sample %d: %d want %d", i, j, got.channels[i].data[j], v)
			}
		}
	}

	for i := range back.nodes {
		if i < len(tree.nodes) && back.nodes[i] != tree.nodes[i] {
			t.Logf("node %d: %+v want %+v", i, back.nodes[i], tree.nodes[i])
		}
	}
}
