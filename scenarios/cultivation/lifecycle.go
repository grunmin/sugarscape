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
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight

	type birthReq struct {
		x, y  int
		kind  string
		attrs engine.AttrBag
	}

	var mu sync.Mutex
	var allBirths []birthReq

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		var localBirths []birthReq
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

				if attrs.Num["age"] >= lifespan {
					agents.Kill(i)
					w.Stats.RecordDeath()
					continue
				}

				age := attrs.Num["age"]
				if age >= cfg.BirthCooldown &&
					age < lifespan*0.7 &&
					rng.Float64() < cfg.BaseBirthRate {

					childRC := GetRealm(1)
					childAttrs := engine.NewAttrBag()
					childAttrs.Num["realm"] = 1
					childAttrs.Num["qi"] = cfg.BaseQi * childRC.QiMultiplier * 0.3
					childAttrs.Num["qi_max"] = cfg.BaseQi * childRC.QiMultiplier
					childAttrs.Num["combat_power"] = cfg.BaseQi * childRC.CombatMultiplier * 0.3
					childAttrs.Num["age"] = 0
					childAttrs.Num["lifespan"] = childRC.Lifespan
					childAttrs.Num["cultivation_speed"] = 0.5 + rng.Float64()*0.5
					childAttrs.Num["aggression"] = clampNorm(rng.NormFloat64()*0.15+0.5, 0, 1)
					childAttrs.Num["perceived_cp_mult"] = 1.10 + rng.Float64()*0.10
					childAttrs.Num["breakthrough_cooldown"] = 0
					childAttrs.Str["sect"] = attrs.Str["sect"]
					childAttrs.Str["strategy"] = attrs.Str["strategy"]
					if rng.Float64() < 0.1 {
						strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
						childAttrs.Str["strategy"] = strategies[rng.Intn(len(strategies))]
					}

					localBirths = append(localBirths, birthReq{
						x: agents.X[i], y: agents.Y[i],
						kind: "cultivator", attrs: childAttrs,
					})
					w.Stats.RecordBirth()
				}

			case "spirit_beast":
				if attrs.Num["age"] >= 200+rng.Float64()*100 {
					agents.Kill(i)
					w.Stats.RecordDeath()
					continue
				}
				if rng.Float64() < 0.01 {
					bEA := engine.NewAttrBag()
					bEA.Num["age"] = 0
					bEA.Num["combat_power"] = cfg.BeastCombatBase * (0.5 + rng.Float64())
					bEA.Num["qi"] = 10
					bEA.Num["qi_max"] = 50
					bEA.Num["lifespan"] = 200 + rng.Float64()*100
					localBirths = append(localBirths, birthReq{
						x: (agents.X[i] + rng.Intn(3) - 1 + gridW) % gridW,
						y: (agents.Y[i] + rng.Intn(3) - 1 + gridH) % gridH,
						kind: "spirit_beast", attrs: bEA,
					})
				}
			}
		}
		if len(localBirths) > 0 {
			mu.Lock()
			allBirths = append(allBirths, localBirths...)
			mu.Unlock()
		}
	})

	for _, b := range allBirths {
		agents.Add(b.kind, b.x, b.y, b.attrs)
	}

	beastCount := agents.CountKind("spirit_beast")
	if beastCount < cfg.BeastMinPopulation {
		for range cfg.BeastSpawnPerTick {
			for attempt := 0; attempt < 10; attempt++ {
				bx := w.RNG.Intn(gridW)
				by := w.RNG.Intn(gridH)
				if env.Env0(bx, by) > 40 {
					bEA := engine.NewAttrBag()
					bEA.Num["age"] = w.RNG.Float64() * 50
					bEA.Num["combat_power"] = cfg.BeastCombatBase * (0.5 + w.RNG.Float64())
					bEA.Num["qi"] = 10
					bEA.Num["qi_max"] = 50
					bEA.Num["lifespan"] = 200 + w.RNG.Float64()*100
					agents.Add("spirit_beast", bx, by, bEA)
					break
				}
			}
		}
	}
}
