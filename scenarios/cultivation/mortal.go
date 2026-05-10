package cultivation

import (
	"sync"

	"github.com/runmin/sugarscape/engine"
)

// MortalSystem handles mortal population dynamics and conversion to cultivators.
type MortalSystem struct{}

func (s *MortalSystem) Name() string  { return "MortalSystem" }
func (s *MortalSystem) Priority() int { return 0 }

func (s *MortalSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	env := w.Next.Env
	ratePerTick := 1.0 / (cfg.MortalLifespan * float64(w.Config.TicksPerYear))
	convPerTick := cfg.MortalConvChance / (cfg.MortalLifespan * float64(w.Config.TicksPerYear))

	var mu sync.Mutex
	var spawnReqs []spawnReq

	engine.ParaForRNG(len(env.Cells), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		var localSpawns []spawnReq
		for i := start; i < end; i++ {
			pop := env.Cells[i].MortalPop
			if pop <= 0 {
				continue
			}

			expectedDeaths := pop * ratePerTick
			actualDeaths := expectedDeaths * (0.8 + rng.Float64()*0.4)
			expectedBirths := pop * ratePerTick
			actualBirths := expectedBirths * (0.8 + rng.Float64()*0.4)
			pop = pop - actualDeaths + actualBirths

			expectedConvs := pop * convPerTick
			convs := 0
			for range int(expectedConvs) {
				if rng.Float64() < convPerTick*float64(w.Config.TicksPerYear) {
					convs++
				}
			}
			fracPart := expectedConvs - float64(int(expectedConvs))
			if rng.Float64() < fracPart {
				convs++
			}

			pop -= float64(convs)
			if pop < 0 {
				pop = 0
			}
			env.Cells[i].MortalPop = pop

			for range convs {
				y := i / w.Config.GridWidth
				x := i % w.Config.GridWidth
				localSpawns = append(localSpawns, spawnReq{x: x, y: y})
			}
		}
		if len(localSpawns) > 0 {
			mu.Lock()
			spawnReqs = append(spawnReqs, localSpawns...)
			mu.Unlock()
		}
	})

	for _, sr := range spawnReqs {
		spawnCultivator(w, sr.x, sr.y)
		w.Stats.RecordMortalConversion()
	}
}

type spawnReq struct{ x, y int }

// spawnCultivator creates a new 练气 cultivator. Called serially, uses w.RNG.
func spawnCultivator(w *engine.World, x, y int) {
	cfg := DefaultScenarioConfig()
	rc := GetRealm(1)
	rng := w.RNG

	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = cfg.BaseQi * rc.QiMultiplier * (0.3 + rng.Float64()*0.5)
	attrs.Num["qi_max"] = cfg.BaseQi * rc.QiMultiplier
	attrs.Num["combat_power"] = cfg.BaseQi * rc.CombatMultiplier * (0.3 + rng.Float64()*0.5)
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
