package cultivation

import "github.com/runmin/sugarscape/engine"

// MovementSystem handles agent movement with realm-based speed scaling.
type MovementSystem struct{}

func (s *MovementSystem) Name() string  { return "MovementSystem" }
func (s *MovementSystem) Priority() int { return 3 }

func (s *MovementSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	env := w.Next.Env
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		for i := start; i < end; i++ {
			if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
				continue
			}
			moveCultivator(rng, agents, env, i, gridW, gridH)
		}
	})
}

func moveCultivator(rng *engine.RNG, agents *engine.AgentStore, env *engine.Grid, i, gridW, gridH int) {
	realm := int(agents.Attrs[i].Num["realm"])
	if realm < 1 {
		realm = 1
	}
	rc := GetRealm(realm)
	moveSpeed := rc.MoveSpeed

	steps := int(moveSpeed)
	if rng.Float64() < moveSpeed-float64(steps) {
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
				nx := (x + dx + gridW) % gridW
				ny := (y + dy + gridH) % gridH
				sp := env.Env0(nx, ny)
				if sp > bestSpirit {
					bestX, bestY = nx, ny
					bestSpirit = sp
				}
			}
		}

		targetX, targetY := x, y
		roll := rng.Float64()
		if roll < 0.7 {
			targetX, targetY = bestX, bestY
		} else if roll < 0.8 {
			targetX, targetY = randomAdjacentPosition(rng, x, y, gridW, gridH)
		}
		if targetX == x && targetY == y && bestX == x && bestY == y {
			targetX, targetY = randomAdjacentPosition(rng, x, y, gridW, gridH)
		}

		agents.X[i] = targetX
		agents.Y[i] = targetY
	}
}

func randomAdjacentPosition(rng *engine.RNG, x, y, gridW, gridH int) (int, int) {
	for {
		dx := rng.Intn(3) - 1
		dy := rng.Intn(3) - 1
		if dx != 0 || dy != 0 {
			return (x + dx + gridW) % gridW, (y + dy + gridH) % gridH
		}
	}
}
