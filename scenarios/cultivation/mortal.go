package cultivation

import (
	"github.com/runmin/sugarscape/engine"
)

// MortalSystem handles mortal population dynamics and conversion to cultivators.
type MortalSystem struct{}

func (s *MortalSystem) Name() string  { return "MortalSystem" }
func (s *MortalSystem) Priority() int { return 0 } // before everything

func (s *MortalSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	env := w.Next.Env

	// Birth rate and death rate per tick (balanced for stable population).
	ratePerTick := 1.0 / (cfg.MortalLifespan * float64(w.Config.TicksPerYear))

	// Conversion rate per tick per mortal.
	convPerTick := cfg.MortalConvChance / (cfg.MortalLifespan * float64(w.Config.TicksPerYear))

	for i := range env.Cells {
		pop := env.Cells[i].MortalPop
		if pop <= 0 {
			continue
		}

		// Natural fluctuation: births and deaths.
		expectedDeaths := pop * ratePerTick
		actualDeaths := expectedDeaths * (0.8 + w.RNG.Float64()*0.4) // +/-20% randomness
		expectedBirths := pop * ratePerTick
		actualBirths := expectedBirths * (0.8 + w.RNG.Float64()*0.4)

		pop = pop - actualDeaths + actualBirths

		// Conversion to cultivator.
		expectedConvs := pop * convPerTick
		convs := 0
		// Use Poisson-like sampling for rare events.
		for range int(expectedConvs) {
			if w.RNG.Float64() < convPerTick*float64(w.Config.TicksPerYear) {
				convs++
			}
		}
		// Also check fractional part probabilistically.
		fracPart := expectedConvs - float64(int(expectedConvs))
		if w.RNG.Float64() < fracPart {
			convs++
		}

		pop -= float64(convs)
		if pop < 0 {
			pop = 0
		}
		env.Cells[i].MortalPop = pop

		// Spawn new cultivators from conversions.
		for range convs {
			// Determine cell position from flat index.
			y := i / w.Config.GridWidth
			x := i % w.Config.GridWidth
			spawnCultivator(w, x, y)
			w.Stats.RecordMortalConversion()
		}
	}
}

// spawnCultivator creates a new 练气 cultivator at (x,y) from mortal conversion.
func spawnCultivator(w *engine.World, x, y int) {
	cfg := DefaultScenarioConfig()
	rc := GetRealm(1) // 练气

	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = cfg.BaseQi * rc.QiMultiplier * (0.3 + w.RNG.Float64()*0.5)
	attrs.Num["qi_max"] = cfg.BaseQi * rc.QiMultiplier
	attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (0.3 + w.RNG.Float64()*0.5)
	attrs.Num["age"] = 15 + w.RNG.Float64()*15 // start at 15-30 years old
	attrs.Num["lifespan"] = rc.Lifespan
	attrs.Num["cultivation_speed"] = 0.3 + w.RNG.Float64()*0.7
	attrs.Num["aggression"] = w.RNG.Float64()          // 0~1
	attrs.Num["breakthrough_cooldown"] = 0

	strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
	attrs.Str["strategy"] = strategies[w.RNG.Intn(len(strategies))]
	if w.RNG.Float64() < 0.9 {
		attrs.Str["sect"] = ""
	} else {
		attrs.Str["sect"] = sectNames[w.RNG.Intn(len(sectNames))]
	}

	w.Next.Agents.Add("cultivator", x, y, attrs)
}

var sectNames = []string{"宗门1", "宗门2", "宗门3", "宗门4"}
