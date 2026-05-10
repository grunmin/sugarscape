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
		kindI := agents.Kind[i]

		detectRange := 0
		if kindI == "cultivator" {
			realm := int(agents.Attrs[i].Num["realm"])
			if realm < 1 {
				realm = 1
			}
			detectRange = GetRealm(realm).DetectRange - 1
		}

		x, y := agents.X[i], agents.Y[i]
		neighbors := w.Grid.GetNeighbors(x, y, detectRange)

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

	// Beast vs cultivator: always fight.
	if (kindI == "spirit_beast" && kindJ == "cultivator") ||
		(kindI == "cultivator" && kindJ == "spirit_beast") {
		return PendingFight{Attacker: i, Defender: j}
	}

	// Beast vs beast: no fight.
	if kindI == "spirit_beast" && kindJ == "spirit_beast" {
		return PendingFight{}
	}

	// Cultivator vs cultivator: cp-based personality-driven.
	if kindI == "cultivator" && kindJ == "cultivator" {
		// Same sect: no fight.
		sectI := agents.Attrs[i].Str["sect"]
		sectJ := agents.Attrs[j].Str["sect"]
		if sectI != "" && sectI == sectJ {
			return PendingFight{}
		}

		cfg := DefaultScenarioConfig()

		// Flee threshold: power ratio check.
		cpI := agents.Attrs[i].Num["combat_power"]
		cpJ := agents.Attrs[j].Num["combat_power"]
		if cpJ > 0 && cpI/cpJ > cfg.FleeThreshold {
			return PendingFight{Attacker: i, Defender: j}
		}
		if cpI > 0 && cpJ/cpI > cfg.FleeThreshold {
			return PendingFight{}
		}

		// Compute attack desire using cp_diff_norm with perceived_cp_mult and sqrt.
		aggression := agents.Attrs[i].Num["aggression"]
		perceivedMult := agents.Attrs[i].Num["perceived_cp_mult"]
		if perceivedMult < 1.0 {
			perceivedMult = 1.15 // default if not set
		}

		selfCP := cpI * perceivedMult
		enemyCP := cpJ
		maxCP := math.Max(selfCP, enemyCP)
		if maxCP == 0 {
			maxCP = 1
		}
		cpDiffNorm := (selfCP - enemyCP) / maxCP // range [-1, 1]

		sign := 1.0
		if cpDiffNorm < 0 {
			sign = -1.0
		}
		attackDesire := aggression * sign * math.Sqrt(math.Abs(cpDiffNorm))

		if attackDesire > 0.5 {
			return PendingFight{Attacker: i, Defender: j}
		}
	}

	return PendingFight{}
}
