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

const resourceCompetitionWeight = 4.0

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
	env := w.Next.Env
	kindI, kindJ := agents.Kind[i], agents.Kind[j]

	// Cultivator vs cultivator: cp-based personality-driven.
	if kindI == "cultivator" && kindJ == "cultivator" {
		if sameSect(agents, i, j) {
			// Same-sect: no fighting, share rumors (道听途说).
			shareRumor(&agents.Attrs[i], &agents.Attrs[j], rumorRelationSameSect)
			shareRumor(&agents.Attrs[j], &agents.Attrs[i], rumorRelationSameSect)
			return PendingFight{}
		}
		sectI := agents.Attrs[i].Str["sect"]
		sectJ := agents.Attrs[j].Str["sect"]
		if hasDiplomacyMissionFor(agents.Attrs[i], sectJ) || hasDiplomacyMissionFor(agents.Attrs[j], sectI) {
			shareRumor(&agents.Attrs[i], &agents.Attrs[j], rumorRelationDifferentSect)
			shareRumor(&agents.Attrs[j], &agents.Attrs[i], rumorRelationDifferentSect)
			return PendingFight{}
		}

		cfg := DefaultScenarioConfig()

		x, y := agents.X[i], agents.Y[i]
		cellAgents := w.Grid.GetNeighbors(x, y, 0)
		cellSpiritFrac := cellSpiritFraction(env, x, y)
		selfI := effectiveSelfCombatPower(agents, cellAgents, i, cfg)
		enemyForI := effectiveObservedCombatPower(agents, cellAgents, j, cfg)
		selfJ := effectiveSelfCombatPower(agents, cellAgents, j, cfg)
		enemyForJ := effectiveObservedCombatPower(agents, cellAgents, i, cfg)
		desireI := attackDesireWithEffectiveCP(agents.Attrs[i], agents.Attrs[j], cellSpiritFrac, selfI, enemyForI)
		desireJ := attackDesireWithEffectiveCP(agents.Attrs[j], agents.Attrs[i], cellSpiritFrac, selfJ, enemyForJ)
		if enemyForJ > 0 && enemyForJ/selfJ > cfg.FleeThreshold {
			desireJ = 0
		}
		if enemyForI > 0 && enemyForI/selfI > cfg.FleeThreshold {
			desireI = 0
		}

		threshold := attackThreshold(agents.Attrs[i], agents.Attrs[j])
		if desireI > threshold && desireI >= desireJ {
			return PendingFight{Attacker: i, Defender: j}
		}
		if desireJ > threshold {
			return PendingFight{Attacker: j, Defender: i}
		}

		// No fight: peaceful meeting, share rumors between different sects.
		shareRumor(&agents.Attrs[i], &agents.Attrs[j], rumorRelationDifferentSect)
		shareRumor(&agents.Attrs[j], &agents.Attrs[i], rumorRelationDifferentSect)
	}

	return PendingFight{}
}

func hasDiplomacyMissionFor(attrs engine.AttrBag, otherSect string) bool {
	if attrs.Str[sectMissionKey] != sectMissionDiplomacy {
		return false
	}
	targetSect := attrs.Str[sectMissionTargetSect]
	return targetSect == "" || targetSect == otherSect
}

func attackDesire(attacker, defender engine.AttrBag) float64 {
	return attackDesireWithResource(attacker, defender, 1)
}

func attackDesireWithResource(attacker, defender engine.AttrBag, cellSpiritFrac float64) float64 {
	selfCP := perceivedCombatPower(attacker)
	enemyCP := defender.Num["combat_power"]
	return attackDesireWithEffectiveCP(attacker, defender, cellSpiritFrac, selfCP, enemyCP)
}

func attackDesireWithEffectiveCP(attacker, defender engine.AttrBag, cellSpiritFrac, selfCP, enemyCP float64) float64 {
	cfg := DefaultScenarioConfig()
	aggression := attacker.Num["aggression"]
	maxCP := math.Max(selfCP, enemyCP)
	if maxCP == 0 {
		maxCP = 1
	}
	cpDiffNorm := (selfCP - enemyCP) / maxCP

	sign := 1.0
	if cpDiffNorm < 0 {
		sign = -1.0
	}
	lossFactor := expectedCombatLossFactorWithCP(attacker, defender, cfg, selfCP, enemyCP)
	base := aggression * sign * math.Sqrt(math.Abs(cpDiffNorm)) * qiFraction(attacker) * conservationFactor(attacker) * lossFactor
	resource := resourceCompetitionDesire(attacker, defender, cellSpiritFrac, selfCP, enemyCP, cfg)
	desire := base + resource*resourceCompetitionWeight*math.Sqrt(qiFraction(attacker))*lossFactor
	if desire > 0 {
		desire *= breakthroughPressureFactor(attacker, cfg)
	}
	return desire
}

func resourceCompetitionDesire(attacker, defender engine.AttrBag, cellSpiritFrac, selfCP, enemyCP float64, cfg ScenarioConfig) float64 {
	if defender.Num["qi"] <= attacker.Num["qi"] {
		return 0
	}
	if enemyCP <= 0 {
		enemyCP = 1
	}
	relativePower := selfCP / enemyCP
	if relativePower < 0.75 {
		return 0
	}
	localScarcity := 0.0
	if cellSpiritFrac < 0.5 {
		localScarcity = (0.5 - cellSpiritFrac) / 0.5
	}
	breakthroughPressure := breakthroughResourcePressure(attacker, cfg)
	competitionPressure := localScarcity + breakthroughPressure
	if competitionPressure <= 0 {
		return 0
	}
	if competitionPressure > 1.5 {
		competitionPressure = 1.5
	}
	qiMax := math.Max(attacker.Num["qi_max"], defender.Num["qi_max"])
	if qiMax <= 0 {
		qiMax = math.Max(attacker.Num["qi"], defender.Num["qi"])
	}
	if qiMax <= 0 {
		return 0
	}
	lootGap := (defender.Num["qi"] - attacker.Num["qi"]) / qiMax
	if lootGap <= 0 {
		return 0
	}
	if lootGap > 1 {
		lootGap = 1
	}
	powerFactor := relativePower
	if powerFactor > 1 {
		powerFactor = 1
	}
	return attacker.Num["aggression"] * competitionPressure * lootGap * powerFactor
}

func attackThreshold(a, b engine.AttrBag) float64 {
	if int(a.Num["realm"]) == int(b.Num["realm"]) {
		return 0.35
	}
	return 0.5
}

func conservationFactor(attrs engine.AttrBag) float64 {
	frac := qiFraction(attrs)
	if frac >= 0.8 {
		return 1
	}
	return frac / 0.8
}

func expectedCombatLossFactor(attacker, defender engine.AttrBag, cfg ScenarioConfig) float64 {
	return expectedCombatLossFactorWithCP(attacker, defender, cfg, attacker.Num["combat_power"], defender.Num["combat_power"])
}

func expectedCombatLossFactorWithCP(attacker, defender engine.AttrBag, cfg ScenarioConfig, attackerCP, defenderCP float64) float64 {
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

func breakthroughResourcePressure(attrs engine.AttrBag, cfg ScenarioConfig) float64 {
	threshold := cfg.BreakthroughQiFrac
	if threshold <= 0 {
		return 0
	}
	frac := qiFraction(attrs)
	if frac >= threshold {
		return 0
	}
	pressure := frac / threshold
	return pressure * pressure
}

func perceivedCombatPower(attrs engine.AttrBag) float64 {
	perceivedMult := attrs.Num["perceived_cp_mult"]
	if perceivedMult < 1.0 {
		perceivedMult = 1.15
	}
	return attrs.Num["combat_power"] * perceivedMult
}

func effectiveSelfCombatPower(agents *engine.AgentStore, cellAgents []int, idx int, cfg ScenarioConfig) float64 {
	return perceivedCombatPower(agents.Attrs[idx]) + sameSectCellCombatPower(agents, cellAgents, idx)*cfg.SectAllyCombatAssist
}

func effectiveObservedCombatPower(agents *engine.AgentStore, cellAgents []int, idx int, cfg ScenarioConfig) float64 {
	return agents.Attrs[idx].Num["combat_power"] + sameSectCellCombatPower(agents, cellAgents, idx)*cfg.SectAllyCombatAssist
}

func sameSectCellCombatPower(agents *engine.AgentStore, cellAgents []int, idx int) float64 {
	sect := agents.Attrs[idx].Str["sect"]
	if sect == "" {
		return 0
	}
	total := 0.0
	for _, j := range cellAgents {
		if j == idx || !agents.Alive[j] || agents.Kind[j] != "cultivator" {
			continue
		}
		if agents.Attrs[j].Str["sect"] == sect {
			total += agents.Attrs[j].Num["combat_power"]
		}
	}
	return total
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

func breakthroughPressureFactor(attrs engine.AttrBag, cfg ScenarioConfig) float64 {
	lifespan := attrs.Num["lifespan"]
	if lifespan <= 0 {
		rc := GetRealm(int(attrs.Num["realm"]))
		lifespan = individualLifespan(attrs, rc)
	}
	if lifespan <= 0 || attrs.Num["age"] < lifespan*0.8 {
		return 1
	}

	qiMax := attrs.Num["qi_max"]
	if qiMax <= 0 {
		rc := GetRealm(int(attrs.Num["realm"]))
		qiMax = cfg.BaseQi * rc.QiMultiplier
	}
	if qiMax <= 0 || attrs.Num["qi"] >= qiMax*cfg.BreakthroughQiFrac {
		return 1
	}
	return 2
}
