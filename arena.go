package jxl

import "sync"

type arena struct {
	mu   sync.Mutex
	free [][]float32
}

func (a *arena) take(n int) []float32 {
	if a == nil {
		return nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	best := -1

	for i, b := range a.free {
		if cap(b) < n {
			continue
		}

		if best < 0 || cap(b) < cap(a.free[best]) {
			best = i
		}
	}

	if best < 0 {
		return nil
	}

	b := a.free[best]
	a.free[best] = a.free[len(a.free)-1]
	a.free = a.free[:len(a.free)-1]

	return b[:n]
}

func (a *arena) get(n int) []float32 {
	b := a.take(n)
	if b == nil {
		return make([]float32, n)
	}

	clear(b)

	return b
}

func (a *arena) getDirty(n int) []float32 {
	if b := a.take(n); b != nil {
		return b
	}

	return make([]float32, n)
}

func (a *arena) put(b []float32) {
	if a == nil || cap(b) == 0 {
		return
	}

	b = b[:cap(b)]

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, f := range a.free {
		if &f[0] == &b[0] {
			return
		}
	}

	a.free = append(a.free, b)
}

func (a *arena) putAll(bs [][]float32) {
	for _, b := range bs {
		a.put(b)
	}
}

func (d *decoder) release(c, keep *canvas) {
	if c == nil || c == keep {
		return
	}

	for _, r := range d.refs {
		if r == c {
			return
		}
	}

	d.arena.putAll(c.planes)
}
