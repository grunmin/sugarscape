package cultivation

import "github.com/runmin/sugarscape/engine"

// MovementSystem handles agent movement on the grid.
// Agents move toward cells with higher spirit density (with randomness).
type MovementSystem struct{}

func (s *MovementSystem) Name() string  { return "MovementSystem" }
func (s *MovementSystem) Priority() int { return 3 }

func (s *MovementSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	env := w.Next.Env
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}
		x, y := agents.X[i], agents.Y[i]

		// Scan 3x3 neighborhood and find the cell with highest spirit density.
		bestX, bestY := x, y
		bestSpirit := env.GetEnv(x, y, "spirit_density")

		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				nx := (x + dx + gridW) % gridW
				ny := (y + dy + gridH) % gridH
				sp := env.GetEnv(nx, ny, "spirit_density")
				if sp > bestSpirit && w.RNG.Float64() < 0.7 {
					bestX, bestY = nx, ny
					bestSpirit = sp
				}
			}
		}

		// Occasionally move randomly (exploration).
		if w.RNG.Float64() < 0.1 || (bestX == x && bestY == y) {
			bestX = (x + w.RNG.Intn(3) - 1 + gridW) % gridW
			bestY = (y + w.RNG.Intn(3) - 1 + gridH) % gridH
		}

		agents.X[i] = bestX
		agents.Y[i] = bestY
	}
}
