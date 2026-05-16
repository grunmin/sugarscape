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

// --- Rumor system (道听途说) ---

// createRumor records a high-spirit location the cultivator has discovered.
// Called from the cultivation system when absorbing in a high-spirit cell,
// and from the interaction system when cultivators share knowledge.
func createRumor(attrs *engine.AttrBag, x, y int, spirit, maxSpirit float64) {
	if maxSpirit <= 0 {
		return
	}
	frac := spirit / maxSpirit
	if frac < 0.6 {
		return // only noteworthy locations create rumors
	}
	existingStrength := attrs.Num["rumor_strength"]
	if frac <= existingStrength {
		return // already have a better rumor
	}
	attrs.Num["rumor_x"] = float64(x)
	attrs.Num["rumor_y"] = float64(y)
	attrs.Num["rumor_strength"] = frac
	attrs.Num["rumor_spirit"] = spirit
}

// shareRumor transfers rumor knowledge from one cultivator to another.
// Same-sect members share more effectively. Called from interaction system
// when cultivators meet peacefully in the same cell.
func shareRumor(from, to *engine.AttrBag, sameSect bool) {
	fromStrength := from.Num["rumor_strength"]
	if fromStrength < 0.3 {
		return // rumor too weak to share
	}
	toStrength := to.Num["rumor_strength"]

	// Sharing efficiency: same-sect 90%, different-sect 60%, strangers 40%.
	efficiency := 0.4
	if sameSect {
		efficiency = 0.9
	}

	sharedStrength := fromStrength * efficiency
	if sharedStrength <= toStrength {
		return // recipient already has an equal or better rumor
	}

	to.Num["rumor_x"] = from.Num["rumor_x"]
	to.Num["rumor_y"] = from.Num["rumor_y"]
	to.Num["rumor_strength"] = sharedStrength
	to.Num["rumor_spirit"] = from.Num["rumor_spirit"]
}

// getRumorTarget returns the next step toward the cultivator's rumored location.
// Returns (x, y, false) if the rumor is too weak, already at target, or proven stale.
func getRumorTarget(attrs *engine.AttrBag, curX, curY, gridW, gridH int) (int, int, bool) {
	strength := attrs.Num["rumor_strength"]
	if strength < 0.25 {
		return 0, 0, false // rumor too weak
	}

	tx := int(math.Round(attrs.Num["rumor_x"]))
	ty := int(math.Round(attrs.Num["rumor_y"]))

	// Toroidal wrap the target coordinates.
	tx = (tx + gridW) % gridW
	ty = (ty + gridH) % gridH

	if tx == curX && ty == curY {
		return 0, 0, false // already at rumor location
	}

	nx, ny := stepToward(curX, curY, tx, ty, gridW, gridH)
	return nx, ny, true
}

// decayRumor reduces rumor strength over time and clears stale rumors.
func decayRumor(attrs *engine.AttrBag) {
	strength := attrs.Num["rumor_strength"]
	if strength <= 0 {
		return
	}
	// Slow decay: ~0.2% per tick. A rumor at strength 0.9 lasts ~600 ticks.
	strength *= 0.998
	if strength < 0.15 {
		// Clear weak rumor entirely.
		attrs.Num["rumor_x"] = 0
		attrs.Num["rumor_y"] = 0
		attrs.Num["rumor_strength"] = 0
		attrs.Num["rumor_spirit"] = 0
	} else {
		attrs.Num["rumor_strength"] = strength
	}
}

// verifyRumorAtLocation checks if a rumor is still valid when the cultivator
// arrives at the rumored location. Called from cultivation system.
func verifyRumorAtLocation(attrs *engine.AttrBag, actualSpirit, maxSpirit float64) {
	rumorSpirit := attrs.Num["rumor_spirit"]
	if rumorSpirit <= 0 {
		return
	}
	if maxSpirit <= 0 {
		maxSpirit = 1
	}
	// If actual spirit is significantly lower than expected, the rumor was
	// exaggerated or the location has been depleted. Clear it.
	if actualSpirit/maxSpirit < 0.4 && rumorSpirit/maxSpirit > 0.6 {
		attrs.Num["rumor_x"] = 0
		attrs.Num["rumor_y"] = 0
		attrs.Num["rumor_strength"] = 0
		attrs.Num["rumor_spirit"] = 0
	}
}

// --- Movement helpers ---

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
