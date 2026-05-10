package cultivation

import (
	"github.com/runmin/sugarscape/engine"
)

// Setup initializes the cultivation world scenario.
func Setup(w *engine.World) {
	cfg := DefaultScenarioConfig()

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

	// Spirit springs.
	for range cfg.NumSpiritSprings {
		sx := w.RNG.Intn(w.Config.GridWidth)
		sy := w.RNG.Intn(w.Config.GridHeight)
		for dy := -5; dy <= 5; dy++ {
			for dx := -5; dx <= 5; dx++ {
				nx := (sx + dx + w.Config.GridWidth) % w.Config.GridWidth
				ny := (sy + dy + w.Config.GridHeight) % w.Config.GridHeight
				dist := float64(dx*dx + dy*dy)
				boost := 30 * exp(-dist/30)
				current := env.Env0(nx, ny)
				newVal := current + boost
				if newVal > current+40 {
					newVal = current + 40
				}
				if newVal > cfg.SpiritMax {
					newVal = cfg.SpiritMax
				}
				env.SetEnv0(nx, ny, newVal)
				env.SetEnv1(nx, ny, newVal+25)
				env.SetEnv2(nx, ny, cfg.SpiritRegenRate+0.05)
			}
		}
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
