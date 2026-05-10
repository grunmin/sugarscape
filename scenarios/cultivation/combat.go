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

		// Winner also pays a combat cost.
		maxCP := math.Max(cpWin, cpLose)
		cpRatio := 0.0
		if maxCP > 0 {
			cpRatio = math.Min(cpWin, cpLose) / maxCP
		}
		cost := agents.Attrs[winner].Num["qi"] * cfg.CombatCostBase * cpRatio
		agents.Attrs[winner].Num["qi"] -= cost
		if agents.Attrs[winner].Num["qi"] < 0 {
			agents.Attrs[winner].Num["qi"] = 0
		}
		updateCombatPower(&agents.Attrs[winner], cfg)

		if w.RNG.Float64() < cfg.CombatDeathChance {
			agents.Kill(loser)
			w.Stats.RecordDeath()

			qiGain := agents.Attrs[loser].Num["qi"] * 0.3
			agents.Attrs[winner].Num["qi"] += qiGain
			qiMax := agents.Attrs[winner].Num["qi_max"]
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
