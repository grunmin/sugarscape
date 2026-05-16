package cultivation

import (
	"math"
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
	sectRealmCounts := countSectRealms(agents)

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
			qiMax := cfg.BaseQi * rc.QiMultiplier
			attrs.Num["qi_max"] = qiMax

			if attrs.Num["breakthrough_cooldown"] > 0 {
				attrs.Num["breakthrough_cooldown"]--
			}

			x, y := agents.X[i], agents.Y[i]
			cellIdx := y*gridW + x
			breakthroughSpiritMult := 1.0
			if attrs.Num["moved_this_tick"] != 1 {
				capacity := qiMax - attrs.Num["qi"]
				if capacity < 0 {
					capacity = 0
				}
				cellLock := &s.cellLocks[cellIdx%len(s.cellLocks)]
				cellLock.Lock()
				cellBefore := env.Cells[cellIdx]
				spirit := cellBefore.Env0
				gradeCultMult := spiritGradeCultivationMultiplier(spirit, cfg)
				breakthroughSpiritMult = spiritGradeBreakthroughMultiplier(spirit, cfg)
				baseAbsorb := spirit * attrs.Num["cultivation_speed"] * cfg.CultivationSpeed * gradeCultMult
				absorb := baseAbsorb * rc.CultSpeedMult
				if absorb > spirit {
					absorb = spirit
				}
				if absorb > capacity {
					absorb = capacity
				}
				env.Cells[cellIdx].Env0 = spirit - absorb
				cellLock.Unlock()

				attrs.Num["qi"] += absorb

				// Rumor verification: if at a rumored location, check the observed
				// spirit before this cultivator consumes from the cell.
				verifyRumorAtCell(attrs, x, y, cellBefore, cfg)
				// Rumor creation: if this is a notably high-spirit cell, remember it.
				createRumorFromCell(attrs, x, y, cellBefore, cfg)
			}

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

				if attrs.Num["moved_this_tick"] == 1 {
					breakthroughSpiritMult = s.breakthroughSpiritMultiplierAt(env, cellIdx, cfg)
				}
				mentors := oneRealmHigherMentors(*attrs, realm, sectRealmCounts)
				if rng.Float64() < breakthroughProbabilityWithSpiritGrade(rc, *attrs, cfg, mentors, breakthroughSpiritMult) {
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

func (s *CultivationSystem) breakthroughSpiritMultiplierAt(env *engine.Grid, cellIdx int, cfg ScenarioConfig) float64 {
	cellLock := &s.cellLocks[cellIdx%len(s.cellLocks)]
	cellLock.Lock()
	spirit := env.Cells[cellIdx].Env0
	cellLock.Unlock()
	return spiritGradeBreakthroughMultiplier(spirit, cfg)
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

func spiritGradeForSpirit(spirit float64, cfg ScenarioConfig) SpiritGradeConfig {
	if len(DefaultSpiritGrades) == 0 {
		return SpiritGradeConfig{
			Name:                   "默认",
			Level:                  1,
			CultSpeedMultiplier:    1,
			BreakthroughMultiplier: 1,
		}
	}

	unit := cfg.SpiritMax
	if unit <= 0 {
		unit = 1
	}
	ratio := spirit / unit
	grade := DefaultSpiritGrades[0]
	for _, candidate := range DefaultSpiritGrades {
		if ratio >= candidate.MinSpiritRatio {
			grade = candidate
		}
	}
	return grade
}

func spiritGradeCultivationMultiplier(spirit float64, cfg ScenarioConfig) float64 {
	mult := spiritGradeForSpirit(spirit, cfg).CultSpeedMultiplier
	if mult <= 0 {
		return 1
	}
	return mult
}

func spiritGradeBreakthroughMultiplier(spirit float64, cfg ScenarioConfig) float64 {
	mult := spiritGradeForSpirit(spirit, cfg).BreakthroughMultiplier
	if mult <= 0 {
		return 1
	}
	return mult
}

func breakthroughProbability(rc RealmConfig, attrs engine.AttrBag, cfg ScenarioConfig, oneRealmHigherMentors int) float64 {
	return breakthroughProbabilityWithSpiritGrade(rc, attrs, cfg, oneRealmHigherMentors, 1)
}

func breakthroughProbabilityWithSpiritGrade(rc RealmConfig, attrs engine.AttrBag, cfg ScenarioConfig, oneRealmHigherMentors int, spiritGradeMultiplier float64) float64 {
	prob := rc.BreakthroughBase
	sect := attrs.Str["sect"]
	if sect == "" {
		prob *= cfg.LooseBreakthroughMultiplier
	} else {
		prob *= 1 + cfg.SectBreakthroughBonus
		prob *= sectTraitForName(sect).BreakthroughMultiplier
		prob *= mentorBreakthroughMultiplier(oneRealmHigherMentors, cfg)
	}
	prob *= spiritGradeMultiplier
	if prob > 1 {
		return 1
	}
	if prob < 0 {
		return 0
	}
	return prob
}

func mentorBreakthroughMultiplier(mentors int, cfg ScenarioConfig) float64 {
	if mentors <= 0 || cfg.SectMentorBonusCap <= 0 {
		return 1
	}
	scale := cfg.SectMentorScale
	if scale <= 0 {
		scale = 1
	}
	root := math.Sqrt(float64(mentors))
	return 1 + cfg.SectMentorBonusCap*root/(root+scale)
}

func countSectRealms(agents *engine.AgentStore) map[string][6]int {
	counts := make(map[string][6]int)
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		sect := agents.Attrs[i].Str["sect"]
		if sect == "" {
			continue
		}
		realm := int(agents.Attrs[i].Num["realm"])
		if realm < 1 {
			realm = 1
		}
		if realm > 5 {
			realm = 5
		}
		realmCounts := counts[sect]
		realmCounts[realm]++
		counts[sect] = realmCounts
	}
	return counts
}

func oneRealmHigherMentors(attrs engine.AttrBag, realm int, sectRealmCounts map[string][6]int) int {
	sect := attrs.Str["sect"]
	if sect == "" || realm < 1 || realm >= 5 {
		return 0
	}
	return sectRealmCounts[sect][realm+1]
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
