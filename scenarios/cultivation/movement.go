package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// MovementSystem handles agent movement with realm-based speed scaling.
type MovementSystem struct{}

func (s *MovementSystem) Name() string  { return "MovementSystem" }
func (s *MovementSystem) Priority() int { return 2 }

func (s *MovementSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	env := w.Next.Env
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	xSnapshot := append([]int(nil), agents.X...)
	ySnapshot := append([]int(nil), agents.Y...)
	aliveSnapshot := append([]bool(nil), agents.Alive...)
	results := make([]moveResult, len(agents.ID))

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		for i := start; i < end; i++ {
			if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
				continue
			}
			results[i] = moveCultivator(rng, agents, env, w.Grid, i, gridW, gridH, xSnapshot, ySnapshot, aliveSnapshot)
		}
	})

	for i, result := range results {
		if !result.valid {
			continue
		}
		agents.X[i] = result.x
		agents.Y[i] = result.y
		if result.moved {
			agents.Attrs[i].Num["moved_this_tick"] = 1
		} else {
			agents.Attrs[i].Num["moved_this_tick"] = 0
		}
	}
}

type moveResult struct {
	valid bool
	x, y  int
	moved bool
}

func moveCultivator(
	rng *engine.RNG,
	agents *engine.AgentStore,
	env *engine.Grid,
	spatial *engine.Grid,
	i, gridW, gridH int,
	xSnapshot, ySnapshot []int,
	aliveSnapshot []bool,
) moveResult {
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

	result := moveResult{valid: true, x: xSnapshot[i], y: ySnapshot[i]}
	for range steps {
		x, y := result.x, result.y
		lowQi := qiFraction(agents.Attrs[i]) < 0.8
		poorCell := cellSpiritFraction(env, x, y) < 0.25
		if rng.Float64() >= movementProbabilityForCultivator(env, x, y, agents.Attrs[i]) {
			continue
		}

		startX, startY := x, y
		if !(lowQi && poorCell) {
			if chaseX, chaseY, ok := chaseTargetPosition(rng, agents, spatial, i, x, y, gridW, gridH, rc.DetectRange, xSnapshot, ySnapshot, aliveSnapshot); ok {
				result.x, result.y = chaseX, chaseY
				if chaseX != startX || chaseY != startY {
					result.moved = true
				}
				continue
			}
		}

		spiritX, spiritY, foundBetterSpirit := bestAdjacentSpiritPosition(env, x, y, gridW, gridH)
		if lowQi && poorCell && foundBetterSpirit {
			result.x, result.y = spiritX, spiritY
			if spiritX != startX || spiritY != startY {
				result.moved = true
			}
			continue
		}

		targetX, targetY := x, y
		roll := rng.Float64()
		spiritSeekProb := spiritSeekProbability(agents.Attrs[i])
		if roll < spiritSeekProb {
			targetX, targetY = spiritX, spiritY
		} else if roll < spiritSeekProb+0.1 {
			targetX, targetY = randomAdjacentPosition(rng, x, y, gridW, gridH)
		}
		if targetX == x && targetY == y && spiritX == x && spiritY == y {
			targetX, targetY = randomAdjacentPosition(rng, x, y, gridW, gridH)
		}

		result.x = targetX
		result.y = targetY
		if targetX != startX || targetY != startY {
			result.moved = true
		}
	}
	return result
}

func bestAdjacentSpiritPosition(env *engine.Grid, x, y, gridW, gridH int) (int, int, bool) {
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
	return bestX, bestY, bestX != x || bestY != y
}

func movementProbabilityForCultivator(env *engine.Grid, x, y int, attrs engine.AttrBag) float64 {
	base := movementProbability(env, x, y)
	if qiFraction(attrs) < 0.8 && cellSpiritFraction(env, x, y) >= 0.25 {
		base *= conservationFactor(attrs)
	}
	base *= breakthroughPressureFactor(attrs, DefaultScenarioConfig())
	if base > 1 {
		return 1
	}
	return base
}

func spiritSeekProbability(attrs engine.AttrBag) float64 {
	prob := 0.7 * breakthroughPressureFactor(attrs, DefaultScenarioConfig())
	if prob > 1 {
		return 1
	}
	return prob
}

func movementProbability(env *engine.Grid, x, y int) float64 {
	prob := 1 - cellSpiritFraction(env, x, y)
	if prob < 0 {
		return 0
	}
	if prob > 1 {
		return 1
	}
	return prob
}

func cellSpiritFraction(env *engine.Grid, x, y int) float64 {
	spirit := env.Env0(x, y)
	maxSpirit := env.Env1(x, y)
	if maxSpirit <= 0 {
		return 0
	}
	frac := spirit / maxSpirit
	if frac < 0 {
		return 0
	}
	if frac > 1 {
		return 1
	}
	return frac
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
