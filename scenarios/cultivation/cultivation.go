package cultivation

import (
	"sync"

	"github.com/runmin/sugarscape/engine"
)

// CultivationSystem handles qi absorption and breakthrough attempts.
type CultivationSystem struct {
	cellLocks []sync.Mutex
}

func (s *CultivationSystem) Name() string  { return "CultivationSystem" }
func (s *CultivationSystem) Priority() int { return 3 }

func (s *CultivationSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	agents := w.Next.Agents
	env := w.Next.Env
	gridW := w.Config.GridWidth
	if len(s.cellLocks) == 0 {
		s.cellLocks = make([]sync.Mutex, 4096)
	}

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

			if attrs.Num["moved_this_tick"] != 1 {
				x, y := agents.X[i], agents.Y[i]
				cellIdx := y*gridW + x
				cellLock := &s.cellLocks[cellIdx%len(s.cellLocks)]
				cellLock.Lock()
				spirit := env.Cells[cellIdx].Env0
				baseAbsorb := spirit * attrs.Num["cultivation_speed"] * cfg.CultivationSpeed
				absorb := baseAbsorb * rc.CultSpeedMult
				if absorb > spirit {
					absorb = spirit
				}
				env.Cells[cellIdx].Env0 = spirit - absorb
				cellLock.Unlock()

				attrs.Num["qi"] += absorb
			}

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
					newRC := GetRealm(newRealm)
					newQiMax := cfg.BaseQi * newRC.QiMultiplier
					attrs.Num["realm"] = float64(newRealm)
					attrs.Num["qi_max"] = newQiMax
					attrs.Num["qi"] = newQiMax * 0.5
					attrs.Num["lifespan"] = newRC.Lifespan
					attrs.Num["breakthrough_cooldown"] = 0
					w.Stats.RecordBreakthrough()
				} else {
					attrs.Num["breakthrough_cooldown"] = float64(breakthroughCooldownTicks(cfg, realm))
				}
			}

			updateCombatPower(attrs, cfg)
		}
	})
}

func breakthroughCooldownTicks(cfg ScenarioConfig, realm int) int {
	if realm < 1 {
		realm = 1
	}
	return cfg.BreakthroughCD << (realm - 1)
}

func updateCombatPower(attrs *engine.AttrBag, cfg ScenarioConfig) {
	realm := int(attrs.Num["realm"])
	if realm < 1 {
		realm = 1
	}
	rc := GetRealm(realm)
	qiMax := cfg.BaseQi * rc.QiMultiplier
	attrs.Num["qi_max"] = qiMax
	if attrs.Num["qi"] < 0 {
		attrs.Num["qi"] = 0
	}
	if attrs.Num["qi"] > qiMax {
		attrs.Num["qi"] = qiMax
	}
	attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (1 + attrs.Num["qi"]/qiMax)
}
