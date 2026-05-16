package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// MovementSystem handles agent movement with realm-based speed scaling.
// Uses a "rumor" (道听途说) system for long-range spirit attraction instead of
// per-cultivator range scanning. Cultivators share knowledge of high-spirit
// locations through social interactions, then follow their known rumors.
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
		// Decay rumors over time.
		decayRumor(&agents.Attrs[i])
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
	detectRange := rc.DetectRange
	if detectRange < 2 {
		detectRange = 2
	}

	steps := int(moveSpeed)
	if rng.Float64() < moveSpeed-float64(steps) {
		steps++
	}

	result := moveResult{valid: true, x: xSnapshot[i], y: ySnapshot[i]}
	for range steps {
		x, y := result.x, result.y

		// Decide whether to move at all.
		moveProb := movementProbabilityForCultivator(env, x, y, agents.Attrs[i])
		if rng.Float64() >= moveProb {
			continue
		}

		startX, startY := x, y

		// Priority 1: Combat — chase nearby enemies.
		lowQi := qiFraction(agents.Attrs[i]) < 0.8
		poorCell := cellSpiritFraction(env, x, y) < 0.25
		if !(lowQi && poorCell) {
			if chaseX, chaseY, ok := chaseTargetPosition(rng, agents, env, spatial, i, x, y, gridW, gridH, detectRange, xSnapshot, ySnapshot, aliveSnapshot); ok {
				result.x, result.y = chaseX, chaseY
				if chaseX != startX || chaseY != startY {
					result.moved = true
				}
				continue
			}
		}

		// Priority 2: Follow rumor (道听途说) — move toward a known high-spirit
		// location shared through social interactions. O(1) lookup, no scanning.
		if rumorX, rumorY, hasRumor := getRumorTarget(&agents.Attrs[i], x, y, gridW, gridH); hasRumor {
			result.x, result.y = rumorX, rumorY
			if result.x != startX || result.y != startY {
				result.moved = true
			}
			continue
		}

		// Priority 3: Local spirit gradient — only check 8 adjacent cells (O(8)).
		// Follow the steepest local gradient. No range scanning needed because
		// long-range information comes from rumors.
		if adjX, adjY, adjBetter := bestAdjacentSpiritPosition(env, x, y, gridW, gridH); adjBetter {
			result.x, result.y = adjX, adjY
			if result.x != startX || result.y != startY {
				result.moved = true
			}
			continue
		}

		// Priority 4: Exploration — random walk when no better local option exists.
		result.x, result.y = randomAdjacentPosition(rng, x, y, gridW, gridH)
		if result.x != startX || result.y != startY {
			result.moved = true
		}
	}
	return result
}

// --- Movement helpers ---

func bestAdjacentSpiritPosition(env *engine.Grid, x, y, gridW, gridH int) (int, int, bool) {
	bestX, bestY := x, y
	bestScore := cellResourceValue(env, x, y)

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			nx := (x + dx + gridW) % gridW
			ny := (y + dy + gridH) % gridH
			score := cellResourceValue(env, nx, ny)
			if score > bestScore {
				bestX, bestY = nx, ny
				bestScore = score
			}
		}
	}
	return bestX, bestY, bestX != x || bestY != y
}

// stepToward returns the coordinates one cell closer to (tx, ty) from (x, y),
// respecting toroidal topology for shortest-path direction.
func stepToward(x, y, tx, ty, gridW, gridH int) (int, int) {
	dx := toroidalDelta(x, tx, gridW)
	dy := toroidalDelta(y, ty, gridH)
	nx := (x + sign(dx) + gridW) % gridW
	ny := (y + sign(dy) + gridH) % gridH
	return nx, ny
}

// movementProbabilityForCultivator computes the probability that a cultivator
// will attempt to move this step.
//
//   - High spirit → low probability (stay and cultivate).
//   - Low spirit → high probability (seek better ground).
//   - Low qi → HIGHER probability to move (need to find spirit!).
//   - Breakthrough pressure increases movement urgency.
func movementProbabilityForCultivator(env *engine.Grid, x, y int, attrs engine.AttrBag) float64 {
	base := movementProbability(env, x, y) // = 1 - cellSpiritFraction

	// Qi-starved cultivators become MORE restless, not less.
	qiFrac := qiFraction(attrs)
	cellFrac := cellSpiritFraction(env, x, y)
	if qiFrac < 0.8 && cellFrac >= 0.25 {
		restlessMultiplier := 1.0 + (0.8-qiFrac)/0.8
		base *= restlessMultiplier
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
	prob := 1 - cellSettlementQuality(env, x, y)
	if prob < 0 {
		return 0
	}
	if prob < 0.05 && !isHighPotentialCell(env, x, y) {
		prob = 0.05
	}
	if prob > 1 {
		return 1
	}
	return prob
}

func cellSettlementQuality(env *engine.Grid, x, y int) float64 {
	score := cellResourceValue(env, x, y)
	if score > 1 {
		return 1
	}
	if score < 0 {
		return 0
	}
	return score
}

func cellResourceValue(env *engine.Grid, x, y int) float64 {
	cell := env.Cells[y*env.Width+x]
	return resourceValue(cell.Env0, cell.Env1, cell.Env2, DefaultScenarioConfig())
}

func resourceValue(spirit, maxSpirit, regen float64, cfg ScenarioConfig) float64 {
	current := spirit / cfg.SpiritMax
	if current < 0 {
		current = 0
	}
	if current > 1 {
		current = 1
	}

	capacity := 0.0
	if cfg.BlessedLandMaxBonus > 0 {
		capacity = (maxSpirit - cfg.SpiritMax) / cfg.BlessedLandMaxBonus
	}
	if capacity < 0 {
		capacity = 0
	}
	if capacity > 1 {
		capacity = 1
	}

	regenScore := 0.0
	if cfg.BlessedLandRegenBonus > 0 {
		regenScore = (regen - cfg.SpiritRegenRate) / cfg.BlessedLandRegenBonus
	}
	if regenScore < 0 {
		regenScore = 0
	}
	if regenScore > 1 {
		regenScore = 1
	}

	return current + 0.55*capacity + 0.35*regenScore
}

func isHighPotentialCell(env *engine.Grid, x, y int) bool {
	cfg := DefaultScenarioConfig()
	cell := env.Cells[y*env.Width+x]
	return cell.Env1 >= cfg.SpiritMax+cfg.SpiritSpringMaxBonus ||
		cell.Env2 >= cfg.SpiritRegenRate+cfg.SpiritSpringRegenBonus
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
	env *engine.Grid,
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

		desire := attackDesireWithResource(agents.Attrs[i], agents.Attrs[j], cellSpiritFraction(env, x, y))
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
