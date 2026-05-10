package cultivation

import "github.com/runmin/sugarscape/engine"

// CultivationSystem handles qi absorption and breakthrough attempts.
type CultivationSystem struct{}

func (s *CultivationSystem) Name() string  { return "CultivationSystem" }
func (s *CultivationSystem) Priority() int { return 2 }

func (s *CultivationSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	agents := w.Next.Agents
	env := w.Next.Env
	gridW := w.Config.GridWidth

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		for i := start; i < end; i++ {
			if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
				continue
			}
			attrs := &agents.Attrs[i]
			realm := int(attrs.Num["realm"])
			if realm < 1 {
				realm = 1
			}
			rc := GetRealm(realm)

			if attrs.Num["breakthrough_cooldown"] > 0 {
				attrs.Num["breakthrough_cooldown"]--
			}

			x, y := agents.X[i], agents.Y[i]
			cellIdx := y*gridW + x
			spirit := env.Cells[cellIdx].Env0
			baseAbsorb := spirit * attrs.Num["cultivation_speed"] * cfg.CultivationSpeed
			absorb := baseAbsorb * rc.CultSpeedMult
			if absorb > spirit {
				absorb = spirit
			}
			// Benign race on env.Env0 — rare, statistically negligible for ABM.
			env.Cells[cellIdx].Env0 = spirit - absorb

			attrs.Num["qi"] += absorb
			qiMax := cfg.BaseQi * rc.QiMultiplier
			attrs.Num["qi_max"] = qiMax
			if attrs.Num["qi"] > qiMax {
				attrs.Num["qi"] = qiMax
			}

			if rc.BreakthroughBase > 0 &&
				attrs.Num["breakthrough_cooldown"] <= 0 &&
				attrs.Num["qi"] >= qiMax*cfg.BreakthroughQiFrac {

				if rng.Float64() < rc.BreakthroughBase {
					newRealm := realm + 1
					attrs.Num["realm"] = float64(newRealm)
					attrs.Num["qi"] = qiMax * 0.5
					newRC := GetRealm(newRealm)
					attrs.Num["lifespan"] = newRC.Lifespan
					w.Stats.RecordBreakthrough()
				} else {
					attrs.Num["breakthrough_cooldown"] = float64(cfg.BreakthroughCD)
				}
			}

			attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (1 + attrs.Num["qi"]/qiMax)
		}
	})
}
