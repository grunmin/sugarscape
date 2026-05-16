package engine

import "math/rand/v2"

// RNG is a deterministic random number generator.
// NOT goroutine-safe — use one per goroutine.
type RNG struct {
	rand *rand.Rand
}

func NewRNG(seed uint64) *RNG {
	s1 := seed ^ 0x5a5a5a5a5a5a5a5a
	s2 := seed ^ 0x3c3c3c3c3c3c3c3c
	src := rand.NewPCG(s1, s2)
	return &RNG{rand: rand.New(src)}
}

// Fork creates an independent RNG for use in another goroutine.
func (r *RNG) Fork() *RNG {
	return NewRNG(r.rand.Uint64())
}

func (r *RNG) Float64() float64 { return r.rand.Float64() }
func (r *RNG) Uint64() uint64   { return r.rand.Uint64() }
func (r *RNG) Intn(n int) int   { return r.rand.IntN(n) }
func (r *RNG) IntRange(min, max int) int {
	return min + r.rand.IntN(max-min+1)
}
func (r *RNG) NormFloat64() float64 { return r.rand.NormFloat64() }

// --- Per-worker RNG pool ---

// RNGPool provides a dedicated RNG per worker goroutine (no locks).
type RNGPool struct {
	RNGs []*RNG
}

// NewRNGPool creates n RNGs forked from a base seed.
func NewRNGPool(base *RNG, n int) *RNGPool {
	pool := &RNGPool{RNGs: make([]*RNG, n)}
	for i := range n {
		pool.RNGs[i] = base.Fork()
	}
	return pool
}
