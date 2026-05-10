package cultivation

import (
	"sync"

	"github.com/runmin/sugarscape/engine"
)

// LifecycleSystem handles aging, natural death, and births.
type LifecycleSystem struct{}

func (s *LifecycleSystem) Name() string  { return "LifecycleSystem" }
func (s *LifecycleSystem) Priority() int { return 7 }

func (s *LifecycleSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	agents := w.Next.Agents
	env := w.Next.Env
	ticksPerYear := float64(w.Config.TicksPerYear)

	type deathReq struct {
		idx    int
		x, y   int
		qi     float64
		realm  int
		id     int
		reason string
	}

	var mu sync.Mutex
	var allDeaths []deathReq

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		var localDeaths []deathReq
		for i := start; i < end; i++ {
			if !agents.Alive[i] {
				continue
			}
			kind := agents.Kind[i]
			attrs := &agents.Attrs[i]

			attrs.Num["age"] += 1.0 / ticksPerYear

			switch kind {
			case "cultivator":
				realm := int(attrs.Num["realm"])
				if realm < 1 {
					realm = 1
				}
				rc := GetRealm(realm)
				lifespan := rc.Lifespan
				consumeCultivatorUpkeep(attrs, cfg)

				x, y := agents.X[i], agents.Y[i]
				if lowSpiritDeathEligible(env, x, y, *attrs, cfg) {
					attrs.Num["low_spirit_years"] += 1.0 / ticksPerYear
				} else {
					attrs.Num["low_spirit_years"] = 0
				}

				lowSpiritDeath := attrs.Num["low_spirit_years"] > lifespan*0.1 && rng.Float64() < 0.3
				if attrs.Num["age"] >= lifespan || lowSpiritDeath {
					reason := "寿元耗尽"
					if attrs.Num["age"] < lifespan && lowSpiritDeath {
						reason = "低灵气滞留超过寿元 1/10 后触发死亡"
					}
					localDeaths = append(localDeaths, deathReq{
						idx:    i,
						x:      x,
						y:      y,
						qi:     attrs.Num["qi"],
						realm:  realm,
						id:     agents.ID[i],
						reason: reason,
					})
					continue
				}
			}
		}
		if len(localDeaths) > 0 {
			mu.Lock()
			allDeaths = append(allDeaths, localDeaths...)
			mu.Unlock()
		}
	})

	for _, d := range allDeaths {
		if agents.Alive[d.idx] {
			addSpirit(w.Next.Env, d.x, d.y, returnedDeathQi(cfg, d.qi, 0))
			agents.Kill(d.idx)
			w.Stats.RecordDeath()
			if shouldRecordDeathEvent(d.realm, d.reason) {
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
	}
}

func consumeCultivatorUpkeep(attrs *engine.AttrBag, cfg ScenarioConfig) {
	cost := realmQiMax(*attrs, cfg) * cfg.CultivatorUpkeepQiFrac
	if cost <= 0 {
		return
	}
	attrs.Num["qi"] -= cost
	if attrs.Num["qi"] < 0 {
		attrs.Num["qi"] = 0
	}
}

func shouldRecordDeathEvent(realm int, reason string) bool {
	return realm >= 4 || (realm >= 3 && reason == "寿元耗尽")
}

func lowSpiritDeathEligible(env *engine.Grid, x, y int, attrs engine.AttrBag, cfg ScenarioConfig) bool {
	qiMax := realmQiMax(attrs, cfg)
	if qiMax <= 0 {
		return false
	}
	return env.Env0(x, y) < qiMax*0.01 && attrs.Num["qi"]/qiMax < cfg.LowSpiritDeathQiFrac
}

func realmQiMax(attrs engine.AttrBag, cfg ScenarioConfig) float64 {
	qiMax := attrs.Num["qi_max"]
	if qiMax <= 0 {
		realm := int(attrs.Num["realm"])
		if realm < 1 {
			realm = 1
		}
		qiMax = cfg.BaseQi * GetRealm(realm).QiMultiplier
	}
	return qiMax
}
