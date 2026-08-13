package jxl

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestXYBForwardInvertsApply(t *testing.T) {
	rng := rand.New(rand.NewPCG(97, 101))

	o := defaultOpsinInverseMatrix()

	for _, target := range []float32{255, 100, 1000} {
		p := newXYBParams(&o, target)

		fwd, ok := newXYBForward(&p)
		if !ok {
			t.Fatalf("intensity %g: the matrix was not invertible", target)
		}

		var worst float64

		for range 20000 {
			r := float32(rng.Float64())
			g := float32(rng.Float64())
			b := float32(rng.Float64())

			x, y, bb := fwd.apply(r, g, b)

			gr, gg, gb := p.apply(x, y, bb)

			for i, d := range [3]float64{
				math.Abs(float64(gr - r)), math.Abs(float64(gg - g)), math.Abs(float64(gb - b)),
			} {
				if math.IsNaN(d) {
					t.Fatalf("intensity %g channel %d: %g %g %g gave NaN", target, i, r, g, b)
				}

				worst = math.Max(worst, d)
			}
		}

		if worst > 2e-5 {
			t.Fatalf("intensity %g: round trip is %g off", target, worst)
		}

		t.Logf("intensity %g: round trip within %g", target, worst)
	}
}
