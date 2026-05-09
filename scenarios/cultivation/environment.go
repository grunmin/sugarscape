package cultivation

import "github.com/runmin/sugarscape/engine"

// EnvironmentSystem handles spirit energy regeneration and diffusion.
type EnvironmentSystem struct{}

func (s *EnvironmentSystem) Name() string     { return "EnvironmentSystem" }
func (s *EnvironmentSystem) Priority() int    { return 1 }

func (s *EnvironmentSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	env := w.Next.Env

	// Spirit energy regeneration (every cell regains some spirit).
	for y := range env.Height {
		for x := range env.Width {
			current := env.GetEnv(x, y, "spirit_density")
			maxVal := env.GetEnv(x, y, "spirit_max")
			if maxVal == 0 {
				maxVal = cfg.SpiritMax
			}
			regen := env.GetEnv(x, y, "regeneration_rate")
			if regen == 0 {
				regen = cfg.SpiritRegenRate
			}
			next := current + regen
			if next > maxVal {
				next = maxVal
			}
			env.SetEnv(x, y, "spirit_density", next)
		}
	}

	// Simple diffusion: each cell leaks a fraction of spirit to neighbors.
	diffusionRate := 0.05
	changes := make([][]float64, env.Height)
	for y := range env.Height {
		changes[y] = make([]float64, env.Width)
	}

	for y := range env.Height {
		for x := range env.Width {
			spirit := env.GetEnv(x, y, "spirit_density")
			leak := spirit * diffusionRate / 8.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx := (x + dx + env.Width) % env.Width
					ny := (y + dy + env.Height) % env.Height
					changes[ny][nx] += leak
					changes[y][x] -= leak
				}
			}
		}
	}

	for y := range env.Height {
		for x := range env.Width {
			newVal := env.GetEnv(x, y, "spirit_density") + changes[y][x]
			if newVal < 0 {
				newVal = 0
			}
			maxVal := env.GetEnv(x, y, "spirit_max")
			if maxVal == 0 {
				maxVal = cfg.SpiritMax
			}
			if newVal > maxVal {
				newVal = maxVal
			}
			env.SetEnv(x, y, "spirit_density", newVal)
		}
	}
}
