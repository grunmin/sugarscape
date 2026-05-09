package cultivation

import "github.com/runmin/sugarscape/engine"

// LifecycleSystem handles aging, natural death, and births.
type LifecycleSystem struct{}

func (s *LifecycleSystem) Name() string  { return "LifecycleSystem" }
func (s *LifecycleSystem) Priority() int { return 7 }

func (s *LifecycleSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	agents := w.Next.Agents
	env := w.Next.Env

	type birthReq struct {
		x, y  int
		kind  string
		attrs engine.AttrBag
	}
	var births []birthReq

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}
		kind := agents.Kind[i]
		attrs := &agents.Attrs[i]

		// Age all living agents.
		attrs.Num["age"] += 1.0 / float64(w.Config.TicksPerYear)

		switch kind {
		case "cultivator":
			realm := int(attrs.Num["realm"])
			if realm < 1 {
				realm = 1
			}
			rc := GetRealm(realm)
			lifespan := rc.Lifespan

			// Natural death.
			if attrs.Num["age"] >= lifespan {
				agents.Kill(i)
				w.Stats.RecordDeath()
				continue
			}

			// Birth.
			age := attrs.Num["age"]
			if age >= cfg.BirthCooldown &&
				age < lifespan*0.7 &&
				w.RNG.Float64() < cfg.BaseBirthRate {

				childRC := GetRealm(1)
				childAttrs := engine.NewAttrBag()
				childAttrs.Num["realm"] = 1
				childAttrs.Num["qi"] = cfg.BaseQi * childRC.QiMultiplier * 0.3
				childAttrs.Num["qi_max"] = cfg.BaseQi * childRC.QiMultiplier
				childAttrs.Num["combat_power"] = cfg.BaseQi * childRC.CombatMultiplier * 0.3
				childAttrs.Num["age"] = 0
				childAttrs.Num["lifespan"] = childRC.Lifespan
				childAttrs.Num["cultivation_speed"] = 0.5 + w.RNG.Float64()*0.5
				childAttrs.Num["aggression"] = w.RNG.Float64()
				childAttrs.Num["breakthrough_cooldown"] = 0
				childAttrs.Str["sect"] = attrs.Str["sect"]
				childAttrs.Str["strategy"] = attrs.Str["strategy"]
				if w.RNG.Float64() < 0.1 {
					strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
					childAttrs.Str["strategy"] = strategies[w.RNG.Intn(len(strategies))]
				}

				births = append(births, birthReq{
					x: agents.X[i], y: agents.Y[i],
					kind: "cultivator", attrs: childAttrs,
				})
				w.Stats.RecordBirth()
			}

		case "spirit_beast":
			if attrs.Num["age"] >= 200+w.RNG.Float64()*100 {
				agents.Kill(i)
				w.Stats.RecordDeath()
				continue
			}
			// Beast reproduction.
			if w.RNG.Float64() < 0.01 {
				bEA := engine.NewAttrBag()
				bEA.Num["age"] = 0
				bEA.Num["combat_power"] = cfg.BeastCombatBase * (0.5 + w.RNG.Float64())
				bEA.Num["qi"] = 10
				bEA.Num["qi_max"] = 50
				bEA.Num["lifespan"] = 200 + w.RNG.Float64()*100
				births = append(births, birthReq{
					x: (agents.X[i] + w.RNG.Intn(3) - 1 + w.Config.GridWidth) % w.Config.GridWidth,
					y: (agents.Y[i] + w.RNG.Intn(3) - 1 + w.Config.GridHeight) % w.Config.GridHeight,
					kind: "spirit_beast", attrs: bEA,
				})
			}
		}
	}

	// Apply births.
	for _, b := range births {
		agents.Add(b.kind, b.x, b.y, b.attrs)
	}

	// Beast population floor.
	beastCount := agents.CountKind("spirit_beast")
	if beastCount < cfg.BeastMinPopulation {
		for range cfg.BeastSpawnPerTick {
			for attempt := 0; attempt < 10; attempt++ {
				bx := w.RNG.Intn(w.Config.GridWidth)
				by := w.RNG.Intn(w.Config.GridHeight)
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
