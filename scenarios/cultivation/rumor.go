package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

const (
	rumorKeyX        = "rumor_x"
	rumorKeyY        = "rumor_y"
	rumorKeyStrength = "rumor_strength"
	rumorKeySpirit   = "rumor_spirit"
)

type rumorRelation int

const (
	rumorRelationStranger rumorRelation = iota
	rumorRelationDifferentSect
	rumorRelationSameSect
)

// createRumor records a high-spirit location the cultivator has discovered.
func createRumor(attrs *engine.AttrBag, x, y int, spirit, maxSpirit float64) {
	cfg := DefaultScenarioConfig()
	createRumorWithStrength(attrs, x, y, spirit, cellResourceQuality(spirit, maxSpirit, cfg.SpiritRegenRate, cfg))
}

func createRumorFromCell(attrs *engine.AttrBag, x, y int, cell engine.Cell, cfg ScenarioConfig) {
	createRumorWithStrength(attrs, x, y, cell.Env0, cellResourceQuality(cell.Env0, cell.Env1, cell.Env2, cfg))
}

func createRumorWithStrength(attrs *engine.AttrBag, x, y int, spirit, strength float64) {
	if strength < 0.6 {
		return
	}
	if strength <= attrs.Num[rumorKeyStrength] {
		return
	}

	attrs.Num[rumorKeyX] = float64(x)
	attrs.Num[rumorKeyY] = float64(y)
	attrs.Num[rumorKeyStrength] = strength
	attrs.Num[rumorKeySpirit] = spirit
}

// shareRumor transfers rumor knowledge from one cultivator to another.
func shareRumor(from, to *engine.AttrBag, relation rumorRelation) {
	fromStrength := from.Num[rumorKeyStrength]
	if fromStrength < 0.3 {
		return
	}

	sharedStrength := fromStrength * rumorShareEfficiency(relation)
	if sharedStrength <= to.Num[rumorKeyStrength] {
		return
	}

	to.Num[rumorKeyX] = from.Num[rumorKeyX]
	to.Num[rumorKeyY] = from.Num[rumorKeyY]
	to.Num[rumorKeyStrength] = sharedStrength
	to.Num[rumorKeySpirit] = from.Num[rumorKeySpirit]
}

func rumorShareEfficiency(relation rumorRelation) float64 {
	switch relation {
	case rumorRelationSameSect:
		return 0.9
	case rumorRelationDifferentSect:
		return 0.6
	default:
		return 0.4
	}
}

// getRumorTarget returns the next step toward the cultivator's rumored location.
func getRumorTarget(attrs *engine.AttrBag, curX, curY, gridW, gridH int) (int, int, bool) {
	if attrs.Num[rumorKeyStrength] < 0.25 {
		return 0, 0, false
	}

	tx, ty := rumorLocation(attrs)
	tx = (tx + gridW) % gridW
	ty = (ty + gridH) % gridH
	if tx == curX && ty == curY {
		return 0, 0, false
	}

	nx, ny := stepToward(curX, curY, tx, ty, gridW, gridH)
	return nx, ny, true
}

// decayRumor reduces rumor strength over time and clears stale rumors.
func decayRumor(attrs *engine.AttrBag) {
	strength := attrs.Num[rumorKeyStrength]
	if strength <= 0 {
		return
	}

	strength *= 0.998
	if strength < 0.15 {
		clearRumor(attrs)
		return
	}
	attrs.Num[rumorKeyStrength] = strength
}

// verifyRumorAtLocation clears a stale rumor only when the cultivator is
// actually standing at the rumored location.
func verifyRumorAtLocation(attrs *engine.AttrBag, x, y int, actualSpirit, maxSpirit float64) {
	cfg := DefaultScenarioConfig()
	verifyRumorAtLocationWithQuality(attrs, x, y, cellResourceQuality(actualSpirit, maxSpirit, cfg.SpiritRegenRate, cfg))
}

func verifyRumorAtCell(attrs *engine.AttrBag, x, y int, cell engine.Cell, cfg ScenarioConfig) {
	verifyRumorAtLocationWithQuality(attrs, x, y, cellResourceQuality(cell.Env0, cell.Env1, cell.Env2, cfg))
}

func verifyRumorAtLocationWithQuality(attrs *engine.AttrBag, x, y int, actualQuality float64) {
	rumorSpirit := attrs.Num[rumorKeySpirit]
	if rumorSpirit <= 0 {
		return
	}

	tx, ty := rumorLocation(attrs)
	if tx != x || ty != y {
		return
	}

	if actualQuality < 0.4 && attrs.Num[rumorKeyStrength] > 0.6 {
		clearRumor(attrs)
	}
}

func cellResourceQuality(spirit, maxSpirit, regen float64, cfg ScenarioConfig) float64 {
	score := resourceValue(spirit, maxSpirit, regen, cfg)
	if score > 1 {
		return 1
	}
	if score < 0 {
		return 0
	}
	return score
}

func rumorLocation(attrs *engine.AttrBag) (int, int) {
	return int(math.Round(attrs.Num[rumorKeyX])), int(math.Round(attrs.Num[rumorKeyY]))
}

func clearRumor(attrs *engine.AttrBag) {
	attrs.Num[rumorKeyX] = 0
	attrs.Num[rumorKeyY] = 0
	attrs.Num[rumorKeyStrength] = 0
	attrs.Num[rumorKeySpirit] = 0
}
