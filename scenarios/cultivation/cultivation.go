package cultivation

import "github.com/runmin/sugarscape/engine"

// CultivationSystem handles qi absorption and breakthrough attempts.
type CultivationSystem struct{}

func (s *CultivationSystem) Name() string  { return "CultivationSystem" }
func (s *CultivationSystem) Priority() int { return 2 }

func (s *CultivationSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	agents := w.Next.Agents // write to next frame
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

		// Absorb qi from environment.
		x, y := agents.X[i], agents.Y[i]
		spirit := env.GetEnv(x, y, "spirit_density")
		absorb := spirit * attrs.Num["cultivation_speed"] * cfg.CultivationSpeed
		if absorb > spirit {
			absorb = spirit
		}
		env.SetEnv(x, y, "spirit_density", spirit-absorb)

		attrs.Num["qi"] += absorb
		qiMax := cfg.BaseQi * rc.QiMultiplier
		attrs.Num["qi_max"] = qiMax
		if attrs.Num["qi"] > qiMax {
			attrs.Num["qi"] = qiMax
		}

		// Breakthrough attempt.
		if rc.BreakthroughBase > 0 &&
			attrs.Num["qi"] >= qiMax*cfg.BreakthroughQiFrac &&
			w.RNG.Float64() < rc.BreakthroughBase {

			attrs.Num["realm"] = float64(realm + 1)
			attrs.Num["qi"] = qiMax * 0.5 // reset qi after breakthrough
			newRC := GetRealm(realm + 1)
			attrs.Num["lifespan"] = newRC.Lifespan
			attrs.Num["combat_power"] = cfg.BaseQi * newRC.CombatMultiplier * (1 + attrs.Num["qi"]/qiMax)
			w.Stats.RecordBreakthrough()
		}

		// Update combat power based on current qi.
		attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (1 + attrs.Num["qi"]/qiMax)
	}
}
