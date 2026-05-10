package cultivation

import "github.com/runmin/sugarscape/engine"

// EnvironmentSystem handles spirit energy regeneration and diffusion.
type EnvironmentSystem struct {
	changes []float64 // pre-allocated, reused across ticks
}

func (s *EnvironmentSystem) Name() string  { return "EnvironmentSystem" }
func (s *EnvironmentSystem) Priority() int { return 1 }

func (s *EnvironmentSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	env := w.Next.Env

	// Ensure changes slice is allocated once.
	if s.changes == nil || len(s.changes) != len(env.Cells) {
		s.changes = make([]float64, len(env.Cells))
	}

	// Phase 1: Regeneration (no RNG needed).
	engine.ParaFor(len(env.Cells), func(start, end int) {
		for i := start; i < end; i++ {
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
	})

	// Phase 2: Diffusion — compute changes (reuse pre-allocated slice).
	diffusionRate := 0.05
	// Zero out changes.
	for i := range s.changes {
		s.changes[i] = 0
	}

	engine.ParaFor(env.Height, func(startY, endY int) {
		for y := startY; y < endY; y++ {
			for x := 0; x < env.Width; x++ {
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
						s.changes[ni] += leak
						s.changes[i] -= leak
					}
				}
			}
		}
	})

	// Phase 3: Apply changes (no RNG).
	engine.ParaFor(len(env.Cells), func(start, end int) {
		for i := start; i < end; i++ {
			newVal := env.Cells[i].Env0 + s.changes[i]
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
	})
}
