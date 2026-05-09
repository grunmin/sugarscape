package cultivation

import (
	"fmt"
	"sort"

	"github.com/runmin/sugarscape/engine"
)

// Setup initializes the cultivation world scenario.
func Setup(w *engine.World) {
	cfg := DefaultScenarioConfig()

	// --- Initialize environment ---
	env := w.Curr.Env
	// Create uneven spirit density using noise-like pattern.
	for y := range env.Height {
		for x := range env.Width {
			// Mix of sine waves for natural-looking distribution.
			v := 0.0
			v += sinF(x, y, 8, 0.7) * 25
			v += sinF(x, y, 15, 0.3) * 15
			v += sinF(x, y, 4, 0.5) * 10
			spirit := cfg.BaseSpiritDensity + v
			if spirit < 5 {
				spirit = 5
			}
			if spirit > cfg.SpiritMax {
				spirit = cfg.SpiritMax
			}
			env.SetEnv(x, y, "spirit_density", spirit)
			env.SetEnv(x, y, "spirit_max", spirit+15) // max slightly above initial
			env.SetEnv(x, y, "regeneration_rate", cfg.SpiritRegenRate+w.RNG.Float64()*0.3)
		}
	}

	// Create a few "spirit springs" — high density areas.
	for range 5 {
		sx := w.RNG.Intn(w.Config.GridWidth)
		sy := w.RNG.Intn(w.Config.GridHeight)
		for dy := -5; dy <= 5; dy++ {
			for dx := -5; dx <= 5; dx++ {
				nx := (sx + dx + w.Config.GridWidth) % w.Config.GridWidth
				ny := (sy + dy + w.Config.GridHeight) % w.Config.GridHeight
				dist := float64(dx*dx + dy*dy)
				boost := 30 * exp(-dist/15)
				current := env.GetEnv(nx, ny, "spirit_density")
				newVal := current + boost
				maxV := current + 40
				if newVal > maxV {
					newVal = maxV
				}
				if newVal > cfg.SpiritMax {
					newVal = cfg.SpiritMax
				}
				env.SetEnv(nx, ny, "spirit_density", newVal)
				env.SetEnv(nx, ny, "spirit_max", newVal+10)
				env.SetEnv(nx, ny, "regeneration_rate", cfg.SpiritRegenRate+0.5)
			}
		}
	}

	// Clone initial env to Next frame.
	w.Next.Env = env.CloneEnv()

	// --- Initialize cultivators ---
	strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
	// Sort realm levels for deterministic initialization.
	realms := make([]int, 0, len(cfg.InitRealmDist))
	for r := range cfg.InitRealmDist {
		realms = append(realms, r)
	}
	sort.Ints(realms)
	for _, realm := range realms {
		fraction := cfg.InitRealmDist[realm]
		count := int(float64(cfg.InitialCultivators) * fraction)
		rc := GetRealm(realm)
		for range count {
			x := w.RNG.Intn(w.Config.GridWidth)
			y := w.RNG.Intn(w.Config.GridHeight)

			attrs := engine.NewAttrBag()
			attrs.Num["realm"] = float64(realm)
			attrs.Num["qi"] = cfg.BaseQi * rc.QiMultiplier * (0.5 + w.RNG.Float64()*0.5)
			attrs.Num["qi_max"] = cfg.BaseQi * rc.QiMultiplier
			attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (0.5 + w.RNG.Float64())
			attrs.Num["age"] = w.RNG.Float64() * rc.Lifespan * 0.3 // start at 0-30% of lifespan
			attrs.Num["lifespan"] = rc.Lifespan
			attrs.Num["cultivation_speed"] = 0.3 + w.RNG.Float64()*0.7
			attrs.Num["breakthrough_chance"] = 1.0 // base multiplier
			attrs.Str["strategy"] = strategies[w.RNG.Intn(len(strategies))]
			// 80% are 散修 (rogue cultivators).
			if w.RNG.Float64() < 0.8 {
				attrs.Str["sect"] = ""
			} else {
				attrs.Str["sect"] = fmt.Sprintf("宗门%d", 1+w.RNG.Intn(4))
			}

			w.Curr.Agents.Add("cultivator", x, y, attrs)
		}
	}

	// --- Initialize spirit beasts ---
	for range cfg.InitialBeasts {
		x := w.RNG.Intn(w.Config.GridWidth)
		y := w.RNG.Intn(w.Config.GridHeight)

		attrs := engine.NewAttrBag()
		attrs.Num["age"] = w.RNG.Float64() * 100
		attrs.Num["combat_power"] = cfg.BeastCombatBase * (0.5 + w.RNG.Float64())
		attrs.Num["qi"] = 10 + w.RNG.Float64()*20
		attrs.Num["qi_max"] = 50
		attrs.Num["lifespan"] = 200 + w.RNG.Float64()*100

		w.Curr.Agents.Add("spirit_beast", x, y, attrs)
	}

	// Clone initial agents to Next frame.
	w.Next.Agents = w.Curr.Agents.Clone()

	// --- Register systems ---
	w.RegisterSystem(&EnvironmentSystem{})
	w.RegisterSystem(&CultivationSystem{})
	w.RegisterSystem(&MovementSystem{})
	w.RegisterSystem(&InteractionSystem{})
	w.RegisterSystem(&CombatSystem{})
	w.RegisterSystem(&LifecycleSystem{})
}

// Utility math helpers (avoid importing math for simple ops).
func sinF(x, y int, period float64, phase float64) float64 {
	v := float64(x)*phase + float64(y)*(1-phase)
	return sin(v/period*2*3.14159 + phase)
}

func sin(v float64) float64 {
	// Taylor series approximation, good enough for terrain generation.
	v = v - float64(int(v/(2*3.14159)))*2*3.14159
	s := v
	t := v
	for n := 1; n < 6; n++ {
		t *= -v * v / float64((2*n)*(2*n+1))
		s += t
	}
	return s
}

func exp(v float64) float64 {
	if v > 50 {
		return 1e10
	}
	if v < -50 {
		return 0
	}
	sum := 1.0
	term := 1.0
	for n := 1; n < 20; n++ {
		term *= v / float64(n)
		sum += term
	}
	return sum
}
