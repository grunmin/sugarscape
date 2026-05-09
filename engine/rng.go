package engine

import (
	"math/rand/v2"
)

type RNG struct {
	src  *rand.PCG
	rand *rand.Rand
	seed [2]uint64
}

func NewRNG(seed uint64) *RNG {
	s1 := seed ^ 0x5a5a5a5a5a5a5a5a
	s2 := seed ^ 0x3c3c3c3c3c3c3c3c
	src := rand.NewPCG(s1, s2)
	return &RNG{
		src:  src,
		rand: rand.New(src),
		seed: [2]uint64{s1, s2},
	}
}

func (r *RNG) Reset() {
	r.src = rand.NewPCG(r.seed[0], r.seed[1])
	r.rand = rand.New(r.src)
}

func (r *RNG) Float64() float64      { return r.rand.Float64() }
func (r *RNG) Intn(n int) int         { return r.rand.IntN(n) }
func (r *RNG) IntRange(min, max int) int {
	return min + r.rand.IntN(max-min+1)
}
func (r *RNG) NormFloat64() float64   { return r.rand.NormFloat64() }
