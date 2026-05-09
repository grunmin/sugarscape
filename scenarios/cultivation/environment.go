package cultivation

import "github.com/runmin/sugarscape/engine"

// EnvironmentSystem handles spirit energy regeneration and diffusion.
type EnvironmentSystem struct{}

func (s *EnvironmentSystem) Name() string  { return "EnvironmentSystem" }
func (s *EnvironmentSystem) Priority() int { return 1 }

func (s *EnvironmentSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	env := w.Next.Env

	// Spirit energy regeneration.
	for i := range env.Cells {
		current := env.Cells[i].Env0
		maxVal := env.Cells[i].Env1
		if maxVal == 0 {
			maxVal = cfg.SpiritMax
		}
		regen := env.Cells[i].Env2
		if regen == 0 {
			regen = cfg.SpiritRegenRate
		}
		next := current + regen
		if next > maxVal {
			next = maxVal
		}
		env.Cells[i].Env0 = next
	}

	// Diffusion.
	diffusionRate := 0.05
	changes := make([]float64, len(env.Cells))

	for y := range env.Height {
		for x := range env.Width {
			i := y*env.Width + x
			spirit := env.Cells[i].Env0
			leak := spirit * diffusionRate / 8.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx := (x + dx + env.Width) % env.Width
					ny := (y + dy + env.Height) % env.Height
					ni := ny*env.Width + nx
					changes[ni] += leak
					changes[i] -= leak
				}
			}
		}
	}

	for i := range env.Cells {
		newVal := env.Cells[i].Env0 + changes[i]
		if newVal < 0 {
			newVal = 0
		}
		maxVal := env.Cells[i].Env1
		if maxVal == 0 {
			maxVal = cfg.SpiritMax
		}
		if newVal > maxVal {
			newVal = maxVal
		}
		env.Cells[i].Env0 = newVal
	}
}
