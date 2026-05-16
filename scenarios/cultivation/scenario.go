package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// Setup initializes the cultivation world scenario.
func Setup(w *engine.World) {
	cfg := DefaultScenarioConfig()
	initializeSectWeights(w.RNG)

	// --- Initialize environment: spirit density ---
	// Use multi-octave FBM noise instead of regular sine waves to produce
	// organic, non-repeating landscape with features at multiple scales.
	terrainSeed := w.RNG.Fork().Uint64() // derive a stable seed from world RNG
	env := w.Curr.Env

	// Pre-compute noise scale: the grid is 1000x1000, so frequency 0.005
	// gives features at ~200-cell wavelength (largest octave).
	noiseFreq := 0.005

	engine.ParaFor(env.Height, func(startY, endY int) {
		for y := startY; y < endY; y++ {
			for x := 0; x < env.Width; x++ {
				nx := float64(x) * noiseFreq
				ny := float64(y) * noiseFreq

				// FBM noise with 6 octaves: large continents down to small hills.
				// lacunarity=2.2 (non-power-of-2 avoids grid alignment artifacts),
				// gain=0.55 (moderate persistence for visible detail at all scales).
				fbm := engine.FBM2D(nx, ny, terrainSeed, 6, 2.2, 0.55)

				// Map from [0,1] to [-1,1] centered at 0, then scale.
				// Amplitude 40 gives range [30-40, 30+40] = [-10, 70] clipped to [5, SpiritMax].
				v := (fbm*2 - 1) * 40

				// Add a second domain-warped noise layer at lower frequency for
				// large-scale structure (mountain ranges / spirit deserts).
				warp := engine.DomainWarp2D(nx*0.4, ny*0.4, terrainSeed^0xDEAD, 1.5)
				v += (warp*2 - 1) * 15

				spirit := cfg.BaseSpiritDensity + v
				if spirit < 5 {
					spirit = 5
				}
				if spirit > cfg.SpiritMax {
					spirit = cfg.SpiritMax
				}

				idx := y*env.Width + x
				env.Cells[idx].Env0 = spirit
				env.Cells[idx].Env1 = spirit + 15
				env.Cells[idx].Env2 = cfg.SpiritRegenRate
			}
		}
	})

	// Layered high-spirit resources: springs, long veins, and rare blessed lands.
	for range cfg.NumSpiritSprings {
		applySpiritNode(env, w.RNG.Intn(w.Config.GridWidth), w.RNG.Intn(w.Config.GridHeight),
			cfg.SpiritSpringRadius, cfg.SpiritSpringBoost, cfg.SpiritSpringMaxBonus, cfg.SpiritSpringRegenBonus, cfg)
	}
	for range cfg.NumSpiritVeins {
		applySpiritVein(env, w.RNG, cfg, w.Config.GridWidth, w.Config.GridHeight)
	}
	for range cfg.NumBlessedLands {
		applySpiritNode(env, w.RNG.Intn(w.Config.GridWidth), w.RNG.Intn(w.Config.GridHeight),
			cfg.BlessedLandRadius, cfg.BlessedLandBoost, cfg.BlessedLandMaxBonus, cfg.BlessedLandRegenBonus, cfg)
	}

	// --- Initialize mortal population (tribal distribution) ---
	tribeCenters := make([][2]int, cfg.NumTribes)
	for i := range cfg.NumTribes {
		tribeCenters[i] = [2]int{
			w.RNG.Intn(w.Config.GridWidth),
			w.RNG.Intn(w.Config.GridHeight),
		}
	}

	for y := range env.Height {
		for x := range env.Width {
			// Find distance to nearest tribe center.
			minDist := 1e9
			for _, tc := range tribeCenters {
				dx := float64(x - tc[0])
				dy := float64(y - tc[1])
				// Toroidal distance.
				if dx < 0 {
					dx = -dx
				}
				if dy < 0 {
					dy = -dy
				}
				if dx > float64(w.Config.GridWidth)/2 {
					dx = float64(w.Config.GridWidth) - dx
				}
				if dy > float64(w.Config.GridHeight)/2 {
					dy = float64(w.Config.GridHeight) - dy
				}
				dist := dx*dx + dy*dy
				if dist < minDist {
					minDist = dist
				}
			}

			// Population density based on distance to tribe center.
			var densityMult float64
			r := minDist
			if r < 9 {
				densityMult = 5.0 // core
			} else if r < 100 {
				densityMult = 2.0 // inner
			} else {
				densityMult = 0.5 // periphery
			}

			// Add noise.
			densityMult *= 0.7 + w.RNG.Float64()*0.6

			mortalPop := cfg.MortalBaseDensity * densityMult
			env.SetMortal(x, y, mortalPop)
		}
	}

	// Normalize to the configured average while preserving tribal shape.
	targetMortals := float64(env.Width*env.Height) * cfg.MortalBaseDensity
	currentMortals := env.TotalMortals()
	if currentMortals > 0 {
		scale := targetMortals / currentMortals
		for i := range env.Cells {
			env.Cells[i].MortalPop *= scale
		}
		env.RecomputeMortalTotal()
	}

	// Environment is updated in place; both frames share the same environment grid.
	w.Next.Env = env

	// Clone initial agents to Next frame (no cultivators initially, converted from mortals).
	w.Next.Agents = w.Curr.Agents.Clone()

	// --- Register systems ---
	w.RegisterSystem(&MortalSystem{})
	w.RegisterSystem(&EnvironmentSystem{})
	w.RegisterSystem(&CultivationSystem{})
	w.RegisterSystem(&MovementSystem{})
	w.RegisterSystem(&InteractionSystem{})
	w.RegisterSystem(&CombatSystem{})
	w.RegisterSystem(&LifecycleSystem{})
}

// --- Math helpers ---

// applySpiritVein places an organic, winding spirit vein across the grid.
// Uses continuous random angles with gentle curvature (wobble) instead of
// fixed discrete directions to create natural-looking vein formations.
func applySpiritVein(env *engine.Grid, rng *engine.RNG, cfg ScenarioConfig, gridW, gridH int) {
	if cfg.SpiritVeinLength <= 0 || cfg.SpiritVeinRadius <= 0 {
		return
	}

	cx := float64(rng.Intn(gridW))
	cy := float64(rng.Intn(gridH))

	// Continuous random starting angle.
	angle := rng.Float64() * 2 * math.Pi

	// Wobble seed for organic curvature.
	wobbleSeed := rng.Fork().Uint64()
	wobblePhase := rng.Float64() * 2 * math.Pi

	half := cfg.SpiritVeinLength / 2
	for step := -half; step <= half; step++ {
		// Gentle curvature: the angle drifts smoothly along the vein.
		// Use noise to create organic meanders rather than straight lines.
		noiseVal := engine.Noise2D(float64(step)*0.04, 0, wobbleSeed)
		curvature := (noiseVal*2 - 1) * 0.6 // radians of angular drift

		// Angular momentum: the vein curves gracefully.
		wobblePhase += curvature * 0.15
		curAngle := angle + wobblePhase + curvature

		sx := cx + math.Cos(curAngle)*float64(step)
		sy := cy + math.Sin(curAngle)*float64(step)

		// Toroidal wrap with smooth behavior.
		sx = math.Mod(sx+float64(gridW), float64(gridW))
		sy = math.Mod(sy+float64(gridH), float64(gridH))

		ix := int(math.Floor(sx + 0.5))
		iy := int(math.Floor(sy + 0.5))

		// Vary the radius slightly along the vein for organic width variation.
		radiusVar := 1.0 + (engine.Noise2D(float64(step)*0.08, 1.0, wobbleSeed^0x5555)-0.5)*0.4
		effectiveRadius := int(math.Round(float64(cfg.SpiritVeinRadius) * radiusVar))
		if effectiveRadius < 2 {
			effectiveRadius = 2
		}

		applySpiritNode(env, ix, iy, effectiveRadius,
			cfg.SpiritVeinBoost, cfg.SpiritVeinMaxBonus, cfg.SpiritVeinRegenBonus, cfg)
	}
}

// applySpiritNode applies a radial Gaussian boost around a center point,
// with organic noise-based irregularity to avoid perfectly circular shapes.
func applySpiritNode(env *engine.Grid, cx, cy, radius int, boost, maxBonus, regenBonus float64, cfg ScenarioConfig) {
	if radius <= 0 || boost <= 0 {
		return
	}
	sigmaSq := float64(radius*radius) / 3
	if sigmaSq <= 0 {
		sigmaSq = 1
	}
	localMax := cfg.SpiritMax + maxBonus
	if localMax < cfg.SpiritMax {
		localMax = cfg.SpiritMax
	}

	// Noise seed for shape irregularity. Different per node instance based on
	// center coordinates to ensure variety across different features.
	shapeSeed := uint64(uint32(cx))<<32 | uint64(uint32(cy))

	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			nx := (cx + dx + env.Width) % env.Width
			ny := (cy + dy + env.Height) % env.Height
			distSq := float64(dx*dx + dy*dy)

			// Add organic irregularity: noise perturbs the effective distance,
			// breaking the perfect circular symmetry.
			noiseVal := engine.Noise2D(float64(dx)*0.3, float64(dy)*0.3, shapeSeed)
			// Perturb distance by up to ±20% to create irregular edges.
			distSq *= 1.0 + (noiseVal-0.5)*0.4

			boostAtCell := boost * math.Exp(-distSq/(2*sigmaSq))
			if boostAtCell <= 0 {
				continue
			}

			current := env.Env0(nx, ny)
			newVal := current + boostAtCell
			if newVal > localMax {
				newVal = localMax
			}
			env.SetEnv0(nx, ny, newVal)
			if env.Env1(nx, ny) < localMax {
				env.SetEnv1(nx, ny, localMax)
			}
			regen := cfg.SpiritRegenRate + regenBonus
			if env.Env2(nx, ny) < regen {
				env.SetEnv2(nx, ny, regen)
			}
		}
	}
}
