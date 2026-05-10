package cultivation

import "github.com/runmin/sugarscape/engine"

// MortalSystem handles mortal population dynamics and conversion to cultivators.
type MortalSystem struct {
	maxMortalPop float64
}

func (s *MortalSystem) Name() string  { return "MortalSystem" }
func (s *MortalSystem) Priority() int { return 0 }

func (s *MortalSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	env := w.Next.Env
	ratePerTick := 1.0 / (cfg.MortalLifespan * float64(w.Config.TicksPerYear))
	convPerTick := cfg.MortalConvChance / (cfg.MortalLifespan * float64(w.Config.TicksPerYear))
	rng := w.RNG

	pop := env.TotalMortals()
	if pop <= 0 {
		return
	}
	if s.maxMortalPop <= 0 {
		s.maxMortalPop = maxMortalPop(env)
	}

	expectedDeaths := pop * ratePerTick
	actualDeaths := expectedDeaths * (0.8 + rng.Float64()*0.4)
	birthMult := cfg.MortalBirthRateMin + rng.Float64()*(cfg.MortalBirthRateMax-cfg.MortalBirthRateMin)
	actualBirths := pop * ratePerTick * birthMult
	env.AddMortalTotal(actualBirths - actualDeaths)

	expectedConvs := env.TotalMortals() * convPerTick
	convs := int(expectedConvs)
	fracPart := expectedConvs - float64(convs)
	if rng.Float64() < fracPart {
		convs++
	}
	if convs > int(env.TotalMortals()) {
		convs = int(env.TotalMortals())
	}

	for range convs {
		sr := sampleMortalSpawn(rng, env, s.maxMortalPop)
		if env.Mortal(sr.x, sr.y) <= 0 {
			continue
		}
		if env.AddMortal(sr.x, sr.y, -1) <= 0 {
			env.SetMortal(sr.x, sr.y, 0)
		}
		spawnCultivator(w, sr.x, sr.y)
		w.Stats.RecordMortalConversion()
	}
}

type spawnReq struct{ x, y int }

func maxMortalPop(env *engine.Grid) float64 {
	maxPop := 0.0
	for i := range env.Cells {
		if env.Cells[i].MortalPop > maxPop {
			maxPop = env.Cells[i].MortalPop
		}
	}
	if maxPop <= 0 {
		return 1
	}
	return maxPop
}

func sampleMortalSpawn(rng *engine.RNG, env *engine.Grid, maxPop float64) spawnReq {
	if maxPop <= 0 {
		maxPop = 1
	}
	for tries := 0; tries < 10000; tries++ {
		idx := rng.Intn(len(env.Cells))
		if env.Cells[idx].MortalPop <= 0 {
			continue
		}
		if rng.Float64()*maxPop <= env.Cells[idx].MortalPop {
			return spawnReq{x: idx % env.Width, y: idx / env.Width}
		}
	}
	idx := rng.Intn(len(env.Cells))
	return spawnReq{x: idx % env.Width, y: idx / env.Width}
}

// spawnCultivator creates a new 练气 cultivator. Called serially, uses w.RNG.
func spawnCultivator(w *engine.World, x, y int) {
	cfg := DefaultScenarioConfig()
	rc := GetRealm(1)
	rng := w.RNG

	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = cfg.BaseQi * rc.QiMultiplier * (0.3 + rng.Float64()*0.5)
	attrs.Num["qi_max"] = cfg.BaseQi * rc.QiMultiplier
	updateCombatPower(&attrs, cfg)
	attrs.Num["age"] = 15 + rng.Float64()*15
	attrs.Num["lifespan"] = rc.Lifespan
	attrs.Num["cultivation_speed"] = 0.3 + rng.Float64()*0.7
	attrs.Num["aggression"] = clampNorm(rng.NormFloat64()*0.15+0.5, 0, 1)
	attrs.Num["perceived_cp_mult"] = 1.10 + rng.Float64()*0.10
	attrs.Num["breakthrough_cooldown"] = 0

	strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
	attrs.Str["strategy"] = strategies[rng.Intn(len(strategies))]
	if rng.Float64() < 0.9 {
		attrs.Str["sect"] = ""
	} else {
		attrs.Str["sect"] = sectNames[rng.Intn(len(sectNames))]
	}

	w.Next.Agents.Add("cultivator", x, y, attrs)
}

var sectNames = []string{"宗门1", "宗门2", "宗门3", "宗门4"}

func clampNorm(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
