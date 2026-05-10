package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// MovementSystem handles agent movement with realm-based speed scaling.
type MovementSystem struct{}

func (s *MovementSystem) Name() string  { return "MovementSystem" }
func (s *MovementSystem) Priority() int { return 3 }

func (s *MovementSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	env := w.Next.Env
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	xSnapshot := append([]int(nil), agents.X...)
	ySnapshot := append([]int(nil), agents.Y...)
	aliveSnapshot := append([]bool(nil), agents.Alive...)

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		for i := start; i < end; i++ {
			if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
				continue
			}
			moveCultivator(rng, agents, env, w.Grid, i, gridW, gridH, xSnapshot, ySnapshot, aliveSnapshot)
		}
	})
}

func moveCultivator(
	rng *engine.RNG,
	agents *engine.AgentStore,
	env *engine.Grid,
	spatial *engine.Grid,
	i, gridW, gridH int,
	xSnapshot, ySnapshot []int,
	aliveSnapshot []bool,
) {
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
		if chaseX, chaseY, ok := chaseTargetPosition(rng, agents, spatial, i, x, y, gridW, gridH, rc.DetectRange, xSnapshot, ySnapshot, aliveSnapshot); ok {
			agents.X[i], agents.Y[i] = chaseX, chaseY
			continue
		}

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

func chaseTargetPosition(
	rng *engine.RNG,
	agents *engine.AgentStore,
	spatial *engine.Grid,
	i, x, y, gridW, gridH, detectRange int,
	xSnapshot, ySnapshot []int,
	aliveSnapshot []bool,
) (int, int, bool) {
	if detectRange <= 0 {
		return 0, 0, false
	}

	bestTarget := -1
	bestWeight := 0.0
	bestProb := 0.0
	for _, j := range spatial.GetNeighbors(x, y, detectRange) {
		if j == i || j >= len(agents.ID) || !aliveSnapshot[j] || agents.Kind[j] != "cultivator" {
			continue
		}
		if sameSect(agents, i, j) {
			continue
		}

		dx := toroidalDelta(x, xSnapshot[j], gridW)
		dy := toroidalDelta(y, ySnapshot[j], gridH)
		distSq := float64(dx*dx + dy*dy)
		if distSq == 0 {
			continue
		}

		desire := attackDesire(agents.Attrs[i], agents.Attrs[j])
		if desire <= 0.5 {
			continue
		}
		weight := desire / distSq
		if weight > bestWeight {
			bestWeight = weight
			bestProb = math.Min(1, weight)
			bestTarget = j
		}
	}

	if bestTarget < 0 || rng.Float64() >= bestProb {
		return 0, 0, false
	}
	targetX := (x + sign(toroidalDelta(x, xSnapshot[bestTarget], gridW)) + gridW) % gridW
	targetY := (y + sign(toroidalDelta(y, ySnapshot[bestTarget], gridH)) + gridH) % gridH
	return targetX, targetY, true
}

func sameSect(agents *engine.AgentStore, i, j int) bool {
	sectI := agents.Attrs[i].Str["sect"]
	sectJ := agents.Attrs[j].Str["sect"]
	return sectI != "" && sectI == sectJ
}

func toroidalDelta(from, to, size int) int {
	d := to - from
	if d > size/2 {
		d -= size
	}
	if d < -size/2 {
		d += size
	}
	return d
}

func sign(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
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
