package jxl

import (
	"math/bits"
	"slices"
)

const (
	treeMaxLeaves     = 512
	treeMinSamples    = 64
	treeSplitCost     = 16
	treeSampleTarget  = 1 << 16
	treeBuckets       = 32
	treeFirstProperty = 4
	treeTokens        = 32
)

type treeSamples struct {
	buckets [numNonRefProperties][]uint8
	bounds  [numNonRefProperties][]int32
	values  [numNonRefProperties][]int32
	tokens  []uint8
	step    int
	seen    int
}

func newTreeSamples(pixels int) *treeSamples {
	s := &treeSamples{step: max(pixels/treeSampleTarget, 1)}

	for p := treeFirstProperty; p < numNonRefProperties; p++ {
		s.values[p] = make([]int32, 0, min(pixels, treeSampleTarget))
	}

	return s
}

func (s *treeSamples) add(props []int32, residual int32) {
	s.seen++

	if s.seen%s.step != 0 {
		return
	}

	for p := treeFirstProperty; p < numNonRefProperties; p++ {
		s.values[p] = append(s.values[p], props[p])
	}

	s.tokens = append(s.tokens, uint8(bits.Len32(packSigned(residual))))
}

// quantise replaces every property's values with a bucket index, so a split
// search walks at most treeBuckets thresholds per property.
func (s *treeSamples) quantise() {
	for p := treeFirstProperty; p < numNonRefProperties; p++ {
		vals := s.values[p]

		sorted := slices.Clone(vals)
		slices.Sort(sorted)
		sorted = slices.Compact(sorted)

		bounds := sorted

		if len(sorted) > treeBuckets {
			bounds = make([]int32, 0, treeBuckets)

			for i := range treeBuckets {
				bounds = append(bounds, sorted[(i+1)*len(sorted)/(treeBuckets+1)])
			}

			bounds = slices.Compact(bounds)
		}

		s.bounds[p] = bounds
		s.buckets[p] = make([]uint8, len(vals))

		for i, v := range vals {
			b, _ := slices.BinarySearch(bounds, v)
			s.buckets[p][i] = uint8(min(b, len(bounds)-1))
		}

		s.values[p] = nil
	}
}

func tokenCost(counts *[treeTokens]uint32) float64 {
	total := uint32(0)

	for _, n := range counts {
		total += n
	}

	if total == 0 {
		return 0
	}

	bits := 0.0

	for b, n := range counts {
		if n == 0 {
			continue
		}

		bits += float64(n) * (log2Int(uint64(total)) - log2Int(uint64(n)))

		if b > 1 {
			bits += float64(n) * float64(b-1)
		}
	}

	return bits
}

type treeSplit struct {
	property int32
	bucket   int
	gain     float64
}

func (s *treeSamples) bestSplit(idx []int32) treeSplit {
	var counts [treeTokens]uint32

	for _, i := range idx {
		counts[s.tokens[i]]++
	}

	base := tokenCost(&counts)
	best := treeSplit{property: -1}

	for p := treeFirstProperty; p < numNonRefProperties; p++ {
		nb := len(s.bounds[p])
		if nb < 2 {
			continue
		}

		per := make([][treeTokens]uint32, nb)

		for _, i := range idx {
			per[s.buckets[p][i]][s.tokens[i]]++
		}

		var (
			low  [treeTokens]uint32
			high [treeTokens]uint32
		)

		high = counts

		for b := range nb - 1 {
			for t := range treeTokens {
				low[t] += per[b][t]
				high[t] -= per[b][t]
			}

			gain := base - tokenCost(&low) - tokenCost(&high) - treeSplitCost

			if gain > best.gain {
				best = treeSplit{property: int32(p), bucket: b, gain: gain}
			}
		}
	}

	return best
}

func (s *treeSamples) grow(idx []int32, leaves *int, p uint8) *treeSpec {
	if len(idx) < treeMinSamples || *leaves >= treeMaxLeaves {
		return predictorLeaf(p)
	}

	split := s.bestSplit(idx)
	if split.property < 0 {
		return predictorLeaf(p)
	}

	var lo, hi []int32

	for _, i := range idx {
		if int(s.buckets[split.property][i]) <= split.bucket {
			lo = append(lo, i)
		} else {
			hi = append(hi, i)
		}
	}

	if len(lo) == 0 || len(hi) == 0 {
		return predictorLeaf(p)
	}

	*leaves++

	spec := &treeSpec{property: split.property, val: s.bounds[split.property][split.bucket]}
	spec.left = s.grow(hi, leaves, p)
	spec.right = s.grow(lo, leaves, p)

	return spec
}

func (s *treeSamples) learn(p uint8) *treeSpec {
	if len(s.tokens) < treeMinSamples {
		return predictorLeaf(p)
	}

	s.quantise()

	idx := make([]int32, len(s.tokens))

	for i := range idx {
		idx[i] = int32(i)
	}

	leaves := 1

	return s.grow(idx, &leaves, p)
}

// learnTree samples every channel under its own predictor and grows a context
// tree per channel, joined by the channel property.
func (m *modImage) learnTree(wph *weightedHeader, preds []uint8) *maTree {
	specs := make([]*treeSpec, len(m.channels))

	var scratch modScratch

	for i, c := range m.channels {
		if c.w == 0 || c.h == 0 {
			specs[i] = predictorLeaf(preds[i])

			continue
		}

		samples := newTreeSamples(c.w * c.h)

		tree := &maTree{nodes: serializeTree(predictorLeaf(preds[i]))}
		tree.finish()
		tree.usesWP = true

		ok := m.walkChannel(tree, wph, i, 0, &scratch,
			func(props []int32, _ uint32, residual int32) bool {
				samples.add(props, residual)

				return true
			})

		if !ok {
			return nil
		}

		specs[i] = samples.learn(preds[i])
	}

	spec := specs[len(specs)-1]

	for c := len(specs) - 2; c >= 0; c-- {
		spec = &treeSpec{property: 0, val: int32(c), left: spec, right: specs[c]}
	}

	t := &maTree{nodes: serializeTree(spec)}
	t.finish()

	return t
}
