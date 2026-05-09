package cultivation

import "github.com/runmin/sugarscape/engine"

// MovementSystem handles agent movement with realm-based speed scaling.
type MovementSystem struct{}

func (s *MovementSystem) Name() string  { return "MovementSystem" }
func (s *MovementSystem) Priority() int { return 3 }

func (s *MovementSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	env := w.Next.Env

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}
		kind := agents.Kind[i]

		switch kind {
		case "cultivator":
			s.moveCultivator(w, agents, env, i)
		case "spirit_beast":
			s.moveBeast(w, agents, env, i)
		}
	}
}

func (s *MovementSystem) moveCultivator(w *engine.World, agents *engine.AgentStore, env *engine.Grid, i int) {
	realm := int(agents.Attrs[i].Num["realm"])
	if realm < 1 {
		realm = 1
	}
	rc := GetRealm(realm)
	moveSpeed := rc.MoveSpeed

	// Integer part: guaranteed steps.
	steps := int(moveSpeed)
	// Fractional part: probability of extra step.
	if w.RNG.Float64() < moveSpeed-float64(steps) {
		steps++
	}

	for range steps {
		x, y := agents.X[i], agents.Y[i]
		bestX, bestY := x, y
		bestSpirit := env.Env0(x, y)

		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx := (x + dx + env.Width) % env.Width
				ny := (y + dy + env.Height) % env.Height
				sp := env.Env0(nx, ny)
				if sp > bestSpirit && w.RNG.Float64() < 0.7 {
					bestX, bestY = nx, ny
					bestSpirit = sp
				}
			}
		}

		// Exploration or stuck.
		if w.RNG.Float64() < 0.1 || (bestX == x && bestY == y) {
			bestX = (x + w.RNG.Intn(3) - 1 + env.Width) % env.Width
			bestY = (y + w.RNG.Intn(3) - 1 + env.Height) % env.Height
		}

		agents.X[i] = bestX
		agents.Y[i] = bestY
	}
}

func (s *MovementSystem) moveBeast(w *engine.World, agents *engine.AgentStore, env *engine.Grid, i int) {
	x, y := agents.X[i], agents.Y[i]
	bestX, bestY := x, y
	bestSpirit := env.Env0(x, y)

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := (x + dx + env.Width) % env.Width
			ny := (y + dy + env.Height) % env.Height
			sp := env.Env0(nx, ny)
			if sp > bestSpirit {
				bestX, bestY = nx, ny
				bestSpirit = sp
			}
		}
	}

	agents.X[i] = bestX
	agents.Y[i] = bestY
}
