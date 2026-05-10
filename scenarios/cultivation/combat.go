package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// CombatSystem resolves pending fights.
type CombatSystem struct{}

func (s *CombatSystem) Name() string  { return "CombatSystem" }
func (s *CombatSystem) Priority() int { return 5 }

func (s *CombatSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	agents := w.Next.Agents
	env := w.Next.Env

	pendingFightsMu.Lock()
	fights := pendingFights
	pendingFights = pendingFights[:0]
	pendingFightsMu.Unlock()

	for _, f := range fights {
		if f.Attacker == 0 && f.Defender == 0 {
			continue
		}
		if f.Attacker >= len(agents.ID) || f.Defender >= len(agents.ID) {
			continue
		}
		if !agents.Alive[f.Attacker] || !agents.Alive[f.Defender] {
			continue
		}

		cpA := agents.Attrs[f.Attacker].Num["combat_power"]
		cpD := agents.Attrs[f.Defender].Num["combat_power"]
		total := cpA + cpD
		if total == 0 {
			total = 1
		}

		roll := w.RNG.Float64()
		var winner, loser int
		if roll < cpA/total {
			winner, loser = f.Attacker, f.Defender
		} else {
			winner, loser = f.Defender, f.Attacker
		}

		cpWin := agents.Attrs[winner].Num["combat_power"]
		cpLose := agents.Attrs[loser].Num["combat_power"]

		cost := combatCost(cfg, agents.Attrs[loser].Num["qi"])
		agents.Attrs[winner].Num["qi"] -= cost
		if agents.Attrs[winner].Num["qi"] < 0 {
			agents.Attrs[winner].Num["qi"] = 0
		}
		updateCombatPower(&agents.Attrs[winner], cfg)

		deathChance := effectiveCombatDeathChance(cfg, cpWin, cpLose, int(agents.Attrs[loser].Num["realm"]))
		if w.RNG.Float64() < deathChance {
			loserX, loserY := agents.X[loser], agents.Y[loser]
			loserQi := agents.Attrs[loser].Num["qi"]
			loserRealm := int(agents.Attrs[loser].Num["realm"])
			loserID := agents.ID[loser]
			agents.Kill(loser)
			w.Stats.RecordDeath()
			if loserRealm >= 4 {
				eventTick := w.Clock.Tick + 1
				w.Stats.RecordNotableEvent(engine.NotableEvent{
					Tick:    eventTick,
					Year:    float64(eventTick) / float64(w.Config.TicksPerYear),
					Kind:    "死亡",
					Realm:   GetRealm(loserRealm).Name,
					AgentID: loserID,
					X:       loserX,
					Y:       loserY,
					Reason:  "战斗死亡",
				})
			}

			qiGain := loserQi * 0.3
			qiMax := agents.Attrs[winner].Num["qi_max"]
			capacity := qiMax - agents.Attrs[winner].Num["qi"]
			if capacity < 0 {
				capacity = 0
			}
			absorbed := math.Min(qiGain, capacity)
			agents.Attrs[winner].Num["qi"] += absorbed
			addSpirit(env, loserX, loserY, loserQi-absorbed)
			if agents.Attrs[winner].Num["qi"] > qiMax {
				agents.Attrs[winner].Num["qi"] = qiMax
			}
			updateCombatPower(&agents.Attrs[winner], cfg)
		} else {
			agents.Attrs[loser].Num["qi"] *= 0.5
			agents.X[loser], agents.Y[loser] = randomAdjacentPosition(
				w.RNG, agents.X[loser], agents.Y[loser], w.Config.GridWidth, w.Config.GridHeight,
			)
			updateCombatPower(&agents.Attrs[loser], cfg)
		}
	}
}

func addSpirit(env *engine.Grid, x, y int, amount float64) {
	if amount <= 0 {
		return
	}
	idx := y*env.Width + x
	env.Cells[idx].Env0 += amount
}

func combatCost(cfg ScenarioConfig, opponentQi float64) float64 {
	if opponentQi <= 0 {
		return 0
	}
	return opponentQi * cfg.CombatCostBase
}

func effectiveCombatDeathChance(cfg ScenarioConfig, winnerCP, loserCP float64, loserRealm int) float64 {
	maxCP := math.Max(winnerCP, loserCP)
	if maxCP <= 0 {
		return 0
	}

	advantage := (winnerCP - loserCP) / maxCP
	if advantage < 0.05 {
		advantage = 0.05
	}

	chance := cfg.CombatDeathChance * advantage * realmDeathFactor(loserRealm)
	if chance < 0 {
		return 0
	}
	if chance > cfg.CombatDeathChance {
		return cfg.CombatDeathChance
	}
	return chance
}

func realmDeathFactor(realm int) float64 {
	switch {
	case realm >= 5:
		return 0.15
	case realm == 4:
		return 0.35
	case realm == 3:
		return 0.65
	case realm == 2:
		return 0.85
	default:
		return 1
	}
}
