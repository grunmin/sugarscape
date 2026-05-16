package engine

import "math"

// hash2D returns a deterministic pseudo-random float in [0, 1) for integer lattice points.
// Uses a simple multiplication-shift hash with a seed.
func hash2D(x, y int, seed uint64) float64 {
	// Combine coordinates with seed using a high-quality hash construction.
	h := uint64(x) * 374761393
	h += uint64(y) * 668265263
	h ^= seed
	h = (h ^ (h >> 13)) * 1274126177
	h ^= h >> 16
	// Map to [0, 1). math.MaxUint64 as float64 is ~1.84e19, safe for division.
	const maxUint64AsFloat = float64(^uint64(0))
	return float64(h) / maxUint64AsFloat
}

// Noise2D returns smooth value noise in [0, 1) for continuous (x, y) coordinates.
// Uses bicubic-like smoothstep interpolation between lattice points.
func Noise2D(x, y float64, seed uint64) float64 {
	ix := int(math.Floor(x))
	iy := int(math.Floor(y))
	fx := x - math.Floor(x)
	fy := y - math.Floor(y)

	// Quintic smoothstep for C2 continuity (reduces grid artifacts).
	fx = fx * fx * fx * (fx*(fx*6-15) + 10)
	fy = fy * fy * fy * (fy*(fy*6-15) + 10)

	// Bilinear interpolation of 4 corner hash values.
	v00 := hash2D(ix, iy, seed)
	v10 := hash2D(ix+1, iy, seed)
	v01 := hash2D(ix, iy+1, seed)
	v11 := hash2D(ix+1, iy+1, seed)

	vx0 := v00 + (v10-v00)*fx
	vx1 := v01 + (v11-v01)*fx
	return vx0 + (vx1-vx0)*fy
}

// FBM2D returns Fractal Brownian Motion noise in approximately [0, 1].
// Combines multiple octaves of Noise2D at increasing frequencies and decreasing amplitudes.
//
// Parameters:
//   - octaves: number of noise layers (typically 4-8)
//   - lacunarity: frequency multiplier per octave (typically ~2.0)
//   - gain: amplitude multiplier per octave (typically ~0.5)
func FBM2D(x, y float64, seed uint64, octaves int, lacunarity, gain float64) float64 {
	value := 0.0
	amplitude := 1.0
	frequency := 1.0
	maxValue := 0.0

	// Use a different seed per octave to decorrelate layers.
	octaveSeed := seed
	for i := 0; i < octaves; i++ {
		value += amplitude * Noise2D(x*frequency, y*frequency, octaveSeed)
		maxValue += amplitude
		amplitude *= gain
		frequency *= lacunarity
		// Advance seed so each octave samples a different hash partition.
		octaveSeed ^= 0x9E3779B97F4A7C15 // golden ratio fraction
	}

	return value / maxValue
}

// DomainWarp2D applies a gentle domain warp to break up grid artifacts.
// Returns the warped composite value in [0, 1).
func DomainWarp2D(x, y float64, seed uint64, warpStrength float64) float64 {
	// Use two independent noise fields to displace the sampling coordinates.
	wx := Noise2D(x, y, seed)*2 - 1
	wy := Noise2D(x, y, seed^0xABCD)*2 - 1
	return Noise2D(x+wx*warpStrength, y+wy*warpStrength, seed^0x1234)
}
