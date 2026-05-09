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

	// Track births this tick (to avoid spawning during iteration).
	type birthReq struct {
		x, y  int
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

			// Natural death (age > lifespan).
			if attrs.Num["age"] >= lifespan {
				agents.Kill(i)
				w.Stats.RecordDeath()
				continue
			}

			// Birth: probability based on age.
			age := attrs.Num["age"]
			if age >= cfg.BirthCooldown &&
				age < lifespan*0.7 &&
				w.RNG.Float64() < cfg.BaseBirthRate {

				childAttrs := engine.NewAttrBag()
				childAttrs.Num["realm"] = 1
				childAttrs.Num["qi"] = cfg.BaseQi * 0.3
				childAttrs.Num["qi_max"] = cfg.BaseQi
				childAttrs.Num["combat_power"] = cfg.BaseQi * 0.3
				childAttrs.Num["age"] = 0
				childAttrs.Num["lifespan"] = DefaultRealms[0].Lifespan
				childAttrs.Num["cultivation_speed"] = 0.5 + w.RNG.Float64()*0.5
				childAttrs.Num["breakthrough_chance"] = 1.0
				childAttrs.Str["sect"] = attrs.Str["sect"]
				childAttrs.Str["strategy"] = attrs.Str["strategy"]
				if w.RNG.Float64() < 0.1 {
					// 10% chance child has different strategy.
					strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
					childAttrs.Str["strategy"] = strategies[w.RNG.Intn(len(strategies))]
				}

				births = append(births, birthReq{
					x:     agents.X[i],
					y:     agents.Y[i],
					attrs: childAttrs,
				})
				w.Stats.RecordBirth()
			}

		case "spirit_beast":
			// Beasts have fixed lifespan.
			if attrs.Num["age"] >= 200 {
				agents.Kill(i)
				w.Stats.RecordDeath()
				continue
			}
			// Beast reproduction.
			if w.RNG.Float64() < 0.002 {
				bEA := engine.NewAttrBag()
				bEA.Num["age"] = 0
				bEA.Num["combat_power"] = cfg.BeastCombatBase * (0.5 + w.RNG.Float64())
				bEA.Num["qi"] = 10
				bEA.Num["qi_max"] = 50
				bEA.Num["lifespan"] = 200 + w.RNG.Float64()*100
				births = append(births, birthReq{
					x:     (agents.X[i] + w.RNG.Intn(3) - 1 + w.Config.GridWidth) % w.Config.GridWidth,
					y:     (agents.Y[i] + w.RNG.Intn(3) - 1 + w.Config.GridHeight) % w.Config.GridHeight,
					attrs: bEA,
				})
			}
		}
	}

	// Apply births.
	for _, b := range births {
		agents.Add("cultivator", b.x, b.y, b.attrs)
	}

	// Cap spirit beasts population loosely.
	beastCount := agents.CountKind("spirit_beast")
	if beastCount < 50 {
		// Spawn a few beasts in high-spirit areas.
		for range 2 {
			bx := w.RNG.Intn(w.Config.GridWidth)
			by := w.RNG.Intn(w.Config.GridHeight)
			if env.GetEnv(bx, by, "spirit_density") > 40 {
				bEA := engine.NewAttrBag()
				bEA.Num["age"] = w.RNG.Float64() * 50
				bEA.Num["combat_power"] = cfg.BeastCombatBase * (0.5 + w.RNG.Float64())
				bEA.Num["qi"] = 10
				bEA.Num["qi_max"] = 50
				bEA.Num["lifespan"] = 200 + w.RNG.Float64()*100
				agents.Add("spirit_beast", bx, by, bEA)
			}
		}
	}
}
