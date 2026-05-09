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

	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		attrs := &agents.Attrs[i]
		realm := int(attrs.Num["realm"])
		if realm < 1 {
			realm = 1
		}
		rc := GetRealm(realm)

		// --- Decrement breakthrough cooldown ---
		if attrs.Num["breakthrough_cooldown"] > 0 {
			attrs.Num["breakthrough_cooldown"]--
		}

		// --- Absorb qi from environment ---
		x, y := agents.X[i], agents.Y[i]
		cellIdx := y*env.Width + x
		spirit := env.Cells[cellIdx].Env0
		baseAbsorb := spirit * attrs.Num["cultivation_speed"] * cfg.CultivationSpeed
		absorb := baseAbsorb * rc.CultSpeedMult // realm speed scaling
		if absorb > spirit {
			absorb = spirit
		}
		env.Cells[cellIdx].Env0 = spirit - absorb

		attrs.Num["qi"] += absorb
		qiMax := cfg.BaseQi * rc.QiMultiplier
		attrs.Num["qi_max"] = qiMax
		if attrs.Num["qi"] > qiMax {
			attrs.Num["qi"] = qiMax
		}

		// --- Breakthrough attempt ---
		if rc.BreakthroughBase > 0 &&
			attrs.Num["breakthrough_cooldown"] <= 0 &&
			attrs.Num["qi"] >= qiMax*cfg.BreakthroughQiFrac {

			if w.RNG.Float64() < rc.BreakthroughBase {
				// Success.
				newRealm := realm + 1
				attrs.Num["realm"] = float64(newRealm)
				attrs.Num["qi"] = qiMax * 0.5
				newRC := GetRealm(newRealm)
				attrs.Num["lifespan"] = newRC.Lifespan
				attrs.Num["combat_power"] = cfg.BaseQi * newRC.CombatMultiplier * (1 + attrs.Num["qi"]/(cfg.BaseQi*newRC.QiMultiplier))
				w.Stats.RecordBreakthrough()
			} else {
				// Failure: enter cooldown.
				attrs.Num["breakthrough_cooldown"] = float64(cfg.BreakthroughCD)
			}
		}

		// --- Update combat power ---
		attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (1 + attrs.Num["qi"]/qiMax)
	}
}
