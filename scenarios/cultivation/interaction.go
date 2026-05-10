package cultivation

import (
	"math"
	"sync"

	"github.com/runmin/sugarscape/engine"
)

// InteractionSystem decides encounters based on realm detection range and personality.
type InteractionSystem struct {
	seen map[int]bool // reused across ticks to avoid allocation
}

func (s *InteractionSystem) Name() string  { return "InteractionSystem" }
func (s *InteractionSystem) Priority() int { return 4 }

var (
	pendingFights   []PendingFight
	pendingFightsMu sync.Mutex
)

type PendingFight struct {
	Attacker int
	Defender int
}

func (s *InteractionSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	var fights []PendingFight

	// Movement runs before interactions, so refresh the spatial index here.
	w.Grid.Rebuild(agents)

	// Reuse or allocate seen map.
	if s.seen == nil {
		s.seen = make(map[int]bool, 65536)
	} else {
		clear(s.seen)
	}

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}
		if agents.Kind[i] != "cultivator" {
			continue
		}

		x, y := agents.X[i], agents.Y[i]
		neighbors := w.Grid.GetNeighbors(x, y, 0)

		for _, j := range neighbors {
			if j >= len(agents.ID) || j == i || !agents.Alive[j] {
				continue
			}
			pairKey := i*10000000 + j
			if s.seen[pairKey] || s.seen[j*10000000+i] {
				continue
			}
			s.seen[pairKey] = true

			fight := s.resolveInteraction(w, i, j)
			if fight.Attacker != 0 || fight.Defender != 0 {
				fights = append(fights, fight)
			}
		}
	}

	pendingFightsMu.Lock()
	pendingFights = append(pendingFights, fights...)
	pendingFightsMu.Unlock()
}

func (s *InteractionSystem) resolveInteraction(w *engine.World, i, j int) PendingFight {
	agents := w.Next.Agents
	kindI, kindJ := agents.Kind[i], agents.Kind[j]

	// Cultivator vs cultivator: cp-based personality-driven.
	if kindI == "cultivator" && kindJ == "cultivator" {
		if sameSect(agents, i, j) {
			return PendingFight{}
		}

		cfg := DefaultScenarioConfig()

		// Flee threshold: power ratio check.
		cpI := agents.Attrs[i].Num["combat_power"]
		cpJ := agents.Attrs[j].Num["combat_power"]
		if cpJ > 0 && cpI/cpJ > cfg.FleeThreshold {
			if w.RNG.Float64() < qiFraction(agents.Attrs[i])*conservationFactor(agents.Attrs[i])*expectedCombatLossFactor(agents.Attrs[i], agents.Attrs[j], cfg) {
				return PendingFight{Attacker: i, Defender: j}
			}
			return PendingFight{}
		}
		if cpI > 0 && cpJ/cpI > cfg.FleeThreshold {
			if w.RNG.Float64() < qiFraction(agents.Attrs[j])*conservationFactor(agents.Attrs[j])*expectedCombatLossFactor(agents.Attrs[j], agents.Attrs[i], cfg) {
				return PendingFight{Attacker: j, Defender: i}
			}
			return PendingFight{}
		}

		if attackDesire(agents.Attrs[i], agents.Attrs[j]) > 0.5 {
			return PendingFight{Attacker: i, Defender: j}
		}
	}

	return PendingFight{}
}

func attackDesire(attacker, defender engine.AttrBag) float64 {
	cfg := DefaultScenarioConfig()
	aggression := attacker.Num["aggression"]
	perceivedMult := attacker.Num["perceived_cp_mult"]
	if perceivedMult < 1.0 {
		perceivedMult = 1.15
	}

	selfCP := attacker.Num["combat_power"] * perceivedMult
	enemyCP := defender.Num["combat_power"]
	maxCP := math.Max(selfCP, enemyCP)
	if maxCP == 0 {
		maxCP = 1
	}
	cpDiffNorm := (selfCP - enemyCP) / maxCP

	sign := 1.0
	if cpDiffNorm < 0 {
		sign = -1.0
	}
	lossFactor := expectedCombatLossFactor(attacker, defender, cfg)
	return aggression * sign * math.Sqrt(math.Abs(cpDiffNorm)) * qiFraction(attacker) * conservationFactor(attacker) * lossFactor
}

func conservationFactor(attrs engine.AttrBag) float64 {
	frac := qiFraction(attrs)
	if frac >= 0.8 {
		return 1
	}
	return frac / 0.8
}

func expectedCombatLossFactor(attacker, defender engine.AttrBag, cfg ScenarioConfig) float64 {
	attackerCP := attacker.Num["combat_power"]
	defenderCP := defender.Num["combat_power"]
	total := attackerCP + defenderCP
	if total <= 0 {
		return 1
	}
	winProb := attackerCP / total
	selfQi := attacker.Num["qi"]
	expectedCost := combatCost(cfg, selfQi, defender.Num["qi"])
	expectedLossFrac := 0.0
	if selfQi > 0 {
		expectedLossFrac = winProb*expectedCost/selfQi + (1-winProb)*0.5
	} else {
		expectedLossFrac = 1
	}
	if expectedLossFrac < 0 {
		return 1
	}
	if expectedLossFrac > 1 {
		return 0
	}
	return 1 - expectedLossFrac
}

func qiFraction(attrs engine.AttrBag) float64 {
	qiMax := attrs.Num["qi_max"]
	if qiMax <= 0 {
		return 1
	}
	frac := attrs.Num["qi"] / qiMax
	if frac < 0 {
		return 0
	}
	if frac > 1 {
		return 1
	}
	return frac
}
