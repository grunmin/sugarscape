package cultivation

import "github.com/runmin/sugarscape/engine"

// InteractionSystem decides what happens when two agents occupy the same cell.
// For v1: 70% chance of combat between beasts and cultivators / different sects.
type InteractionSystem struct{}

func (s *InteractionSystem) Name() string  { return "InteractionSystem" }
func (s *InteractionSystem) Priority() int { return 4 }

// PendingFight records a fight that will be resolved by CombatSystem.
type PendingFight struct {
	Attacker int
	Defender int
}

// PendingFights is stored on the world for the combat system to consume.
// We use a simple approach: store the list as a global for the current tick.
var pendingFightsKey = "pending_fights"

func (s *InteractionSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	var fights []PendingFight

	// Track which agent pairs have already interacted this tick.
	interacted := make(map[int]bool)

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}

		x, y := agents.X[i], agents.Y[i]
		neighbors := w.Grid.GetNeighbors(x, y)
		for _, j := range neighbors {
			if j >= len(agents.ID) || j == i || !agents.Alive[j] {
				continue
			}
			// Skip if same cell & same kind with same sect
			if agents.X[j] == x && agents.Y[j] == y {
				pairKey := i*1000000 + j
				if interacted[pairKey] {
					continue
				}
				if interacted[j*1000000+i] {
					continue
				}
				interacted[pairKey] = true

				fights = append(fights, s.resolveInteraction(w, i, j))
			}
		}
	}

	// Store pending fights for CombatSystem.
	if len(fights) > 0 {
		w.Next.Agents.Attrs = append(w.Next.Agents.Attrs, engine.NewAttrBag()) // dummy, will be cleaned
	}
	// HACK: store fights in a global for now. In v2, use the event bus.
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

	// Cultivator vs cultivator.
	if kindI == "cultivator" && kindJ == "cultivator" {
		cpI := agents.Attrs[i].Num["combat_power"]
		cpJ := agents.Attrs[j].Num["combat_power"]
		ratio := cpI / cpJ
		if cpJ == 0 {
			ratio = 10
		}

		// Large power gap → weak side flees (no fight).
		if ratio > DefaultScenarioConfig().FleeThreshold ||
			(1.0/ratio) > DefaultScenarioConfig().FleeThreshold {
			return PendingFight{} // empty fight = no fight
		}

		// Same sect → cooperate (no fight).
		sectI := agents.Attrs[i].Str["sect"]
		sectJ := agents.Attrs[j].Str["sect"]
		if sectI != "" && sectI == sectJ {
			return PendingFight{}
		}

		// Otherwise, some probability of fighting.
		if w.RNG.Float64() < 0.3 {
			return PendingFight{Attacker: i, Defender: j}
		}
	}

	return PendingFight{}
}
