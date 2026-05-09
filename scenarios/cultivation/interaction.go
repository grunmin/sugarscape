package cultivation

import (
	"sync"

	"github.com/runmin/sugarscape/engine"
)

// InteractionSystem decides encounters based on realm detection range and personality.
type InteractionSystem struct{}

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

	// Track which pairs have already been processed.
	seen := make(map[int]bool)

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}
		kindI := agents.Kind[i]

		// Determine detection range.
		detectRange := 0
		if kindI == "cultivator" {
			realm := int(agents.Attrs[i].Num["realm"])
			if realm < 1 {
				realm = 1
			}
			detectRange = GetRealm(realm).DetectRange - 1 // range in cells around
		}

		x, y := agents.X[i], agents.Y[i]
		neighbors := w.Grid.GetNeighbors(x, y, detectRange)

		for _, j := range neighbors {
			if j >= len(agents.ID) || j == i || !agents.Alive[j] {
				continue
			}
			pairKey := i*10000000 + j
			if seen[pairKey] || seen[j*10000000+i] {
				continue
			}
			seen[pairKey] = true

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

	// Beast vs cultivator: always fight.
	if (kindI == "spirit_beast" && kindJ == "cultivator") ||
		(kindI == "cultivator" && kindJ == "spirit_beast") {
		return PendingFight{Attacker: i, Defender: j}
	}

	// Beast vs beast: no fight.
	if kindI == "spirit_beast" && kindJ == "spirit_beast" {
		return PendingFight{}
	}

	// Cultivator vs cultivator: personality-driven.
	if kindI == "cultivator" && kindJ == "cultivator" {
		realmI := int(agents.Attrs[i].Num["realm"])
		realmJ := int(agents.Attrs[j].Num["realm"])
		if realmI < 1 {
			realmI = 1
		}
		if realmJ < 1 {
			realmJ = 1
		}

		// Same sect: no fight.
		sectI := agents.Attrs[i].Str["sect"]
		sectJ := agents.Attrs[j].Str["sect"]
		if sectI != "" && sectI == sectJ {
			return PendingFight{}
		}

		// Compute attack desire for i → j.
		realmDiff := float64(realmI - realmJ)
		aggression := agents.Attrs[i].Num["aggression"]
		attackDesire := aggression * realmDiff * realmDiff
		if realmDiff < 0 {
			attackDesire = -aggression * realmDiff * realmDiff
		}

		// Also check power ratio for flee threshold.
		cpI := agents.Attrs[i].Num["combat_power"]
		cpJ := agents.Attrs[j].Num["combat_power"]
		if cpJ > 0 && cpI/cpJ > DefaultScenarioConfig().FleeThreshold {
			// i is much stronger — always attacks.
			return PendingFight{Attacker: i, Defender: j}
		}
		if cpI > 0 && cpJ/cpI > DefaultScenarioConfig().FleeThreshold {
			// j is much stronger — i flees.
			return PendingFight{}
		}

		if attackDesire > 0.5 {
			return PendingFight{Attacker: i, Defender: j}
		}
		// attackDesire between -0.5 and 0.5: ignore.
	}

	return PendingFight{}
}
