package jxl

const (
	wpProperty            = 15
	numNonRefProperties   = 16
	propertiesPerPrevChan = 4

	ctxSplitVal       = 0
	ctxProperty       = 1
	ctxPredictor      = 2
	ctxOffset         = 3
	ctxMultiplierLog  = 4
	ctxMultiplierBits = 5
	numTreeContexts   = 6

	treeHeightLimit = 2048
)

type treeNode struct {
	property int32
	val      int32
	child    uint32
	id       uint32
}

func (n *treeNode) isLeaf() bool { return n.property < 0 }

func (n *treeNode) predictor() uint8 { return uint8(-1 - n.property) }

type maTree struct {
	nodes         []treeNode
	histograms    histograms
	numProperties int
	usesWP        bool
}

func (t *maTree) read(r *bitReader, sizeLimit int) {
	var th histograms
	th.read(r, numTreeContexts, true)

	if r.err() != nil {
		return
	}

	var sr symbolReader

	sr.init(&th, r, 0)

	var (
		toDecode = 1
		leafID   uint32
	)

	for toDecode > 0 {
		if len(t.nodes) > sizeLimit {
			r.setErr(ErrInvalid)

			return
		}
		if r.err() != nil {
			return
		}

		toDecode--

		property := sr.readUnsigned(&th, r, ctxProperty)

		if property > 0 {
			property--

			if property > 255 {
				r.setErr(ErrInvalid)

				return
			}

			splitVal := sr.readSigned(&th, r, ctxSplitVal)
			left := uint32(len(t.nodes) + toDecode + 1)

			toDecode += 2
			t.nodes = append(t.nodes, treeNode{
				property: int32(property), val: splitVal, child: left,
			})

			continue
		}

		predictor := sr.readUnsigned(&th, r, ctxPredictor)
		if predictor >= numPredictors {
			r.setErr(ErrInvalid)

			return
		}

		offset := sr.readSigned(&th, r, ctxOffset)

		mulLog := sr.readUnsigned(&th, r, ctxMultiplierLog)
		if mulLog >= 31 {
			r.setErr(ErrInvalid)

			return
		}

		mulBits := sr.readUnsigned(&th, r, ctxMultiplierBits)

		multiplier := (uint64(mulBits) + 1) << mulLog
		if multiplier > 0xffffffff {
			r.setErr(ErrInvalid)

			return
		}

		t.nodes = append(t.nodes, treeNode{
			property: -1 - int32(predictor), val: offset,
			child: uint32(multiplier), id: leafID,
		})

		leafID++
	}

	if err := sr.checkFinalState(&th, r); err != nil {
		r.setErr(err)

		return
	}

	t.finish()

	if !t.validate(r) {
		return
	}

	t.histograms.read(r, (len(t.nodes)+1)/2, true)
}

func (t *maTree) validate(r *bitReader) bool {
	if len(t.nodes) == 0 {
		return true
	}

	type frame struct {
		node    int
		depth   int
		lo, hi  int32
		restore int
		stage   int
	}

	lower := make([]int32, t.numProperties)
	upper := make([]int32, t.numProperties)

	for i := range lower {
		lower[i], upper[i] = -1<<31, 1<<31-1
	}

	stack := []frame{{node: 0, restore: -1}}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if f.depth > treeHeightLimit {
			r.setErr(ErrInvalid)

			return false
		}
		if f.node < 0 || f.node >= len(t.nodes) {
			r.setErr(ErrInvalid)

			return false
		}

		n := t.nodes[f.node]

		if n.isLeaf() {
			if f.restore >= 0 {
				lower[f.restore], upper[f.restore] = f.lo, f.hi
			}

			continue
		}

		p := int(n.property)
		if p >= t.numProperties {
			r.setErr(ErrInvalid)

			return false
		}

		if lower[p] > n.val || upper[p] <= n.val {
			r.setErr(ErrInvalid)

			return false
		}

		switch f.stage {
		case 0:
			f.stage = 1
			stack = append(stack, f)

			lo, hi := lower[p], upper[p]
			lower[p] = n.val + 1
			stack = append(stack, frame{
				node: int(n.child), depth: f.depth + 1, lo: lo, hi: hi, restore: p,
			})
		case 1:
			f.stage = 2
			stack = append(stack, f)

			lo, hi := lower[p], upper[p]
			upper[p] = n.val
			stack = append(stack, frame{
				node: int(n.child) + 1, depth: f.depth + 1, lo: lo, hi: hi, restore: p,
			})
		default:
			if f.restore >= 0 {
				lower[f.restore], upper[f.restore] = f.lo, f.hi
			}
		}
	}

	return true
}

func (t *maTree) walk(properties []int32) *treeNode {
	nodes := t.nodes
	i := uint32(0)

	for {
		n := &nodes[i]
		if n.property < 0 {
			return n
		}

		i = n.child
		if properties[n.property] <= n.val {
			i++
		}
	}
}
