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

	type breakthroughDeathReq struct {
		idx    int
		x, y   int
		qi     float64
		realm  int
		id     int
		reason string
	}

	var deathMu sync.Mutex
	var breakthroughDeaths []breakthroughDeathReq

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		var localDeaths []breakthroughDeathReq
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

			if attrs.Num["qi"] >= qiMax*cfg.BreakthroughQiFrac {
				attrs.Num["breakthrough_sustain_ticks"]++
			} else {
				attrs.Num["breakthrough_sustain_ticks"] = 0
			}

			if rc.BreakthroughBase > 0 &&
				attrs.Num["breakthrough_cooldown"] <= 0 &&
				attrs.Num["breakthrough_sustain_ticks"] >= float64(breakthroughSustainTicks(cfg, realm)) {

				if rng.Float64() < rc.BreakthroughBase {
					newRealm := realm + 1
					newRC := GetRealm(newRealm)
					newQiMax := cfg.BaseQi * newRC.QiMultiplier
					attrs.Num["realm"] = float64(newRealm)
					attrs.Num["qi_max"] = newQiMax
					attrs.Num["qi"] = newQiMax * cfg.BreakthroughPostQiFrac
					attrs.Num["lifespan"] = randomLifespan(rng, newRC)
					attrs.Num["breakthrough_cooldown"] = 0
					attrs.Num["breakthrough_sustain_ticks"] = 0
					w.Stats.RecordBreakthrough()
					if newRealm >= 4 {
						eventTick := w.Clock.Tick + 1
						w.Stats.RecordNotableEvent(engine.NotableEvent{
							Tick:    eventTick,
							Year:    float64(eventTick) / float64(w.Config.TicksPerYear),
							Kind:    "诞生",
							Realm:   newRC.Name,
							AgentID: agents.ID[i],
							X:       agents.X[i],
							Y:       agents.Y[i],
							Reason:  rc.Name + " -> " + newRC.Name,
						})
					}
				} else {
					if realm == 3 && rng.Float64() < cfg.JindanBreakFailDeathChance {
						localDeaths = append(localDeaths, breakthroughDeathReq{
							idx:    i,
							x:      agents.X[i],
							y:      agents.Y[i],
							qi:     attrs.Num["qi"],
							realm:  realm,
							id:     agents.ID[i],
							reason: "冲击元婴失败死亡",
						})
						continue
					}
					attrs.Num["breakthrough_cooldown"] = float64(breakthroughCooldownTicks(cfg, realm))
				}
			}

			updateCombatPower(attrs, cfg)
		}
		if len(localDeaths) > 0 {
			deathMu.Lock()
			breakthroughDeaths = append(breakthroughDeaths, localDeaths...)
			deathMu.Unlock()
		}
	})

	for _, d := range breakthroughDeaths {
		if !agents.Alive[d.idx] {
			continue
		}
		addSpirit(env, d.x, d.y, returnedDeathQi(cfg, d.qi, 0))
		agents.Kill(d.idx)
		w.Stats.RecordDeath()
		eventTick := w.Clock.Tick + 1
		w.Stats.RecordNotableEvent(engine.NotableEvent{
			Tick:    eventTick,
			Year:    float64(eventTick) / float64(w.Config.TicksPerYear),
			Kind:    "死亡",
			Realm:   GetRealm(d.realm).Name,
			AgentID: d.id,
			X:       d.x,
			Y:       d.y,
			Reason:  d.reason,
		})
	}
}

func randomLifespan(rng *engine.RNG, rc RealmConfig) float64 {
	return rc.Lifespan * (0.6 + rng.Float64()*0.4)
}

func breakthroughCooldownTicks(cfg ScenarioConfig, realm int) int {
	if realm < 1 {
		realm = 1
	}
	return cfg.BreakthroughCD << (realm - 1)
}

func breakthroughSustainTicks(cfg ScenarioConfig, realm int) int {
	if realm < 1 {
		realm = 1
	}
	idx := realm - 1
	if idx < len(cfg.BreakthroughSustainTicks) {
		return cfg.BreakthroughSustainTicks[idx]
	}
	if len(cfg.BreakthroughSustainTicks) == 0 {
		return 1
	}
	return cfg.BreakthroughSustainTicks[len(cfg.BreakthroughSustainTicks)-1]
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
