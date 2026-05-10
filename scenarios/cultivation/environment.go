package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// EnvironmentSystem handles spirit energy regeneration and diffusion.
type EnvironmentSystem struct {
	base []float64 // pre-allocated spirit snapshot, reused across ticks
}

func (s *EnvironmentSystem) Name() string  { return "EnvironmentSystem" }
func (s *EnvironmentSystem) Priority() int { return 1 }

func (s *EnvironmentSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	interval := cfg.EnvironmentTickEvery
	if interval < 1 {
		interval = 1
	}
	if w.Clock.Tick%int64(interval) != 0 {
		return
	}

	env := w.Next.Env

	// Ensure base slice is allocated once.
	if s.base == nil || len(s.base) != len(env.Cells) {
		s.base = make([]float64, len(env.Cells))
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
			regen *= float64(interval)
			next := current
			if current < maxVal {
				next = current + regen
				if next > maxVal {
					next = maxVal
				}
			}
			env.Cells[i].Env0 = next
		}
	})

	// Phase 2: Diffusion from a stable post-regeneration snapshot.
	diffusionRate := 1 - math.Pow(0.95, float64(interval))
	engine.ParaFor(len(env.Cells), func(start, end int) {
		for i := start; i < end; i++ {
			s.base[i] = env.Cells[i].Env0
		}
	})

	engine.ParaFor(env.Height, func(startY, endY int) {
		for y := startY; y < endY; y++ {
			for x := 0; x < env.Width; x++ {
				i := y*env.Width + x
				newVal := s.base[i] * (1.0 - diffusionRate)
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						nx := (x + dx + env.Width) % env.Width
						ny := (y + dy + env.Height) % env.Height
						ni := ny*env.Width + nx
						newVal += s.base[ni] * diffusionRate / 8.0
					}
				}
				env.Cells[i].Env0 = newVal
			}
		}
	})
}
