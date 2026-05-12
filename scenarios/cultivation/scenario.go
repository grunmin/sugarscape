package cultivation

import (
	"github.com/runmin/sugarscape/engine"
)

// Setup initializes the cultivation world scenario.
func Setup(w *engine.World) {
	cfg := DefaultScenarioConfig()
	initializeSectWeights(w.RNG)

	// --- Initialize environment: spirit density ---
	env := w.Curr.Env
	for y := range env.Height {
		for x := range env.Width {
			v := 0.0
			v += sinF(x, y, 80, 0.7) * 25
			v += sinF(x, y, 150, 0.3) * 15
			v += sinF(x, y, 40, 0.5) * 10
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

func sinF(x, y int, period float64, phase float64) float64 {
	v := float64(x)*phase + float64(y)*(1-phase)
	return sin(v/period*2*3.14159 + phase)
}

func applySpiritVein(env *engine.Grid, rng *engine.RNG, cfg ScenarioConfig, gridW, gridH int) {
	if cfg.SpiritVeinLength <= 0 || cfg.SpiritVeinRadius <= 0 {
		return
	}
	directions := [][2]int{
		{1, 0}, {0, 1}, {1, 1}, {1, -1},
		{2, 1}, {1, 2}, {2, -1}, {1, -2},
	}
	dir := directions[rng.Intn(len(directions))]
	cx := rng.Intn(gridW)
	cy := rng.Intn(gridH)
	half := cfg.SpiritVeinLength / 2
	for step := -half; step <= half; step++ {
		x := (cx + dir[0]*step + gridW*cfg.SpiritVeinLength) % gridW
		y := (cy + dir[1]*step + gridH*cfg.SpiritVeinLength) % gridH
		applySpiritNode(env, x, y, cfg.SpiritVeinRadius, cfg.SpiritVeinBoost, cfg.SpiritVeinMaxBonus, cfg.SpiritVeinRegenBonus, cfg)
	}
}

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
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			nx := (cx + dx + env.Width) % env.Width
			ny := (cy + dy + env.Height) % env.Height
			distSq := float64(dx*dx + dy*dy)
			boostAtCell := boost * exp(-distSq/(2*sigmaSq))
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

func sin(v float64) float64 {
	v = v - float64(int(v/(2*3.14159)))*2*3.14159
	s := v
	t := v
	for n := 1; n < 6; n++ {
		t *= -v * v / float64((2*n)*(2*n+1))
		s += t
	}
	return s
}

func exp(v float64) float64 {
	if v > 50 {
		return 1e10
	}
	if v < -50 {
		return 0
	}
	sum := 1.0
	term := 1.0
	for n := 1; n < 20; n++ {
		term *= v / float64(n)
		sum += term
	}
	return sum
}
