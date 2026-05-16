package cultivation

import (
	"math"

	"github.com/runmin/sugarscape/engine"
)

// MortalSystem handles mortal population dynamics and conversion to cultivators.
type MortalSystem struct {
	maxMortalPop              float64
	globalSpiritFactor        float64
	maxSpawnSpirit            float64
	lastGlobalSpiritCheckTick int64
	lastSpawnSpiritCheckTick  int64
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
	globalFactor := s.conversionGlobalSpiritFactor(w, cfg)

	expectedDeaths := pop * ratePerTick
	actualDeaths := expectedDeaths * (0.8 + rng.Float64()*0.4)
	birthMult := cfg.MortalBirthRateMin + rng.Float64()*(cfg.MortalBirthRateMax-cfg.MortalBirthRateMin)
	actualBirths := pop * ratePerTick * birthMult
	env.AddMortalTotal(actualBirths - actualDeaths)

	expectedConvs := env.TotalMortals() * convPerTick * globalFactor
	convs := int(expectedConvs)
	fracPart := expectedConvs - float64(convs)
	if rng.Float64() < fracPart {
		convs++
	}
	if convs > int(env.TotalMortals()) {
		convs = int(env.TotalMortals())
	}
	recruitWeights := sectRecruitWeights(w.Next.Agents)
	maxSpawnSpirit := s.conversionSpawnMaxSpirit(w, cfg)

	for range convs {
		sr := sampleMortalSpawn(rng, env, s.maxMortalPop, maxSpawnSpirit, cfg)
		if env.Mortal(sr.x, sr.y) <= 0 {
			continue
		}
		if rng.Float64() >= conversionLocalSpiritFactor(env.Env0(sr.x, sr.y), cfg) {
			continue
		}
		if env.AddMortal(sr.x, sr.y, -1) <= 0 {
			env.SetMortal(sr.x, sr.y, 0)
		}
		spawnCultivator(w, sr.x, sr.y, recruitWeights)
		w.Stats.RecordMortalConversion()
	}
}

type spawnReq struct{ x, y int }

func (s *MortalSystem) conversionGlobalSpiritFactor(w *engine.World, cfg ScenarioConfig) float64 {
	interval := cfg.ConversionSpiritCheckEvery
	if interval < 1 {
		interval = 1
	}
	if s.globalSpiritFactor == 0 || w.Clock.Tick-s.lastGlobalSpiritCheckTick >= int64(interval) {
		threshold := cfg.ConversionGlobalSpiritThresholdAvg * float64(len(w.Next.Env.Cells))
		s.globalSpiritFactor = proportionalFactor(w.Next.Env.TotalSpirit(), threshold)
		s.lastGlobalSpiritCheckTick = w.Clock.Tick
	}
	return s.globalSpiritFactor
}

func (s *MortalSystem) conversionSpawnMaxSpirit(w *engine.World, cfg ScenarioConfig) float64 {
	interval := cfg.ConversionSpiritCheckEvery
	if interval < 1 {
		interval = 1
	}
	if s.maxSpawnSpirit == 0 || w.Clock.Tick-s.lastSpawnSpiritCheckTick >= int64(interval) {
		s.maxSpawnSpirit = maxCurrentSpirit(w.Next.Env)
		s.lastSpawnSpiritCheckTick = w.Clock.Tick
	}
	return s.maxSpawnSpirit
}

func conversionLocalSpiritFactor(spirit float64, cfg ScenarioConfig) float64 {
	return proportionalFactor(spirit, cfg.ConversionLocalSpiritThreshold)
}

func proportionalFactor(value, threshold float64) float64 {
	if threshold <= 0 {
		return 1
	}
	factor := value / threshold
	if factor < 0 {
		return 0
	}
	if factor > 1 {
		return 1
	}
	return factor
}

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

func maxCurrentSpirit(env *engine.Grid) float64 {
	maxSpirit := 0.0
	for i := range env.Cells {
		if env.Cells[i].Env0 > maxSpirit {
			maxSpirit = env.Cells[i].Env0
		}
	}
	if maxSpirit <= 0 {
		return 1
	}
	return maxSpirit
}

func conversionSpawnSpiritFactor(spirit, maxSpirit float64, cfg ScenarioConfig) float64 {
	floor := cfg.ConversionSpawnSpiritFloor
	if floor < 0 {
		floor = 0
	}
	if floor > 1 {
		floor = 1
	}
	if maxSpirit <= 0 {
		return floor
	}
	relative := spirit / maxSpirit
	if relative < 0 {
		relative = 0
	}
	if relative > 1 {
		relative = 1
	}
	exponent := cfg.ConversionSpawnSpiritExponent
	if exponent <= 0 {
		exponent = 1
	}
	return floor + (1-floor)*math.Pow(relative, exponent)
}

func sampleMortalSpawn(rng *engine.RNG, env *engine.Grid, maxPop, maxSpirit float64, cfg ScenarioConfig) spawnReq {
	if maxPop <= 0 {
		maxPop = 1
	}
	if maxSpirit <= 0 {
		maxSpirit = 1
	}

	// Rejection sampling with joint weight: mortal population × spirit suitability.
	// The suitability curve is intentionally steep so new cultivators cluster
	// around high-spirit regions instead of mirroring the broad mortal map.
	for tries := 0; tries < 10000; tries++ {
		idx := rng.Intn(len(env.Cells))
		cell := &env.Cells[idx]
		if cell.MortalPop <= 0 {
			continue
		}
		spiritFactor := conversionSpawnSpiritFactor(cell.Env0, maxSpirit, cfg)
		jointWeight := (cell.MortalPop / maxPop) * spiritFactor
		if rng.Float64() <= jointWeight {
			return spawnReq{x: idx % env.Width, y: idx / env.Width}
		}
	}
	// Fallback: pure random (should rarely be reached).
	idx := rng.Intn(len(env.Cells))
	return spawnReq{x: idx % env.Width, y: idx / env.Width}
}

// spawnCultivator creates a new 练气 cultivator. Called serially, uses w.RNG.
func spawnCultivator(w *engine.World, x, y int, recruitWeights []float64) {
	cfg := DefaultScenarioConfig()
	rc := GetRealm(1)
	rng := w.RNG

	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = cfg.BaseQi * rc.QiMultiplier * (0.3 + rng.Float64()*0.5)
	attrs.Num["qi_max"] = cfg.BaseQi * rc.QiMultiplier
	updateCombatPower(&attrs, cfg)
	attrs.Num["age"] = 15 + rng.Float64()*15
	attrs.Num["lifespan"] = randomLifespan(rng, rc)
	attrs.Num["cultivation_speed"] = 0.3 + rng.Float64()*0.7
	attrs.Num["aggression"] = clampNorm(rng.NormFloat64()*0.15+0.5, 0, 1)
	attrs.Num["perceived_cp_mult"] = 1.10 + rng.Float64()*0.10
	attrs.Num["breakthrough_cooldown"] = 0

	strategies := []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}
	attrs.Str["strategy"] = strategies[rng.Intn(len(strategies))]
	if rng.Float64() < cfg.SectMembershipChance {
		sect := weightedSectName(rng, recruitWeights)
		attrs.Str["sect"] = sect
		attrs.Num["aggression"] = clampNorm(attrs.Num["aggression"]+sectTraitForName(sect).AggressionBias, 0, 1)
	}

	w.Next.Agents.Add("cultivator", x, y, attrs)
}

var (
	sectNames   = []string{"宗门1", "宗门2", "宗门3", "宗门4", "宗门5", "宗门6", "宗门7"}
	sectWeights []float64
)

// SectTrait defines a fixed sect identity that shapes long-run recruitment and growth.
type SectTrait struct {
	Style                  string
	RecruitMultiplier      float64
	PowerRecruitMultiplier float64
	BreakthroughMultiplier float64
	AggressionBias         float64
}

var sectTraits = []SectTrait{
	{Style: "开山", RecruitMultiplier: 1.35, PowerRecruitMultiplier: 0.90, BreakthroughMultiplier: 0.95, AggressionBias: 0.02},
	{Style: "战修", RecruitMultiplier: 0.85, PowerRecruitMultiplier: 1.30, BreakthroughMultiplier: 1.05, AggressionBias: 0.10},
	{Style: "丹鼎", RecruitMultiplier: 1.00, PowerRecruitMultiplier: 1.05, BreakthroughMultiplier: 1.25, AggressionBias: -0.03},
	{Style: "隐峰", RecruitMultiplier: 0.70, PowerRecruitMultiplier: 1.45, BreakthroughMultiplier: 1.15, AggressionBias: -0.08},
	{Style: "外门", RecruitMultiplier: 1.55, PowerRecruitMultiplier: 0.80, BreakthroughMultiplier: 0.90, AggressionBias: 0.04},
	{Style: "霸道", RecruitMultiplier: 0.95, PowerRecruitMultiplier: 1.20, BreakthroughMultiplier: 1.00, AggressionBias: 0.14},
	{Style: "清修", RecruitMultiplier: 1.10, PowerRecruitMultiplier: 0.95, BreakthroughMultiplier: 1.10, AggressionBias: -0.10},
}

// SectStat summarizes a sect's live cultivator population and combat standing.
type SectStat struct {
	Name           string
	Count          int
	MaxCombatPower float64
	CombatValue    float64
	RealmCounts    [6]int
}

func SectNames() []string {
	names := make([]string, len(sectNames))
	copy(names, sectNames)
	return names
}

func SectTraits() []SectTrait {
	traits := make([]SectTrait, len(sectTraits))
	copy(traits, sectTraits)
	return traits
}

func CalculateSectStats(agents *engine.AgentStore) []SectStat {
	stats := make([]SectStat, len(sectNames))
	index := make(map[string]int, len(sectNames))
	for i, name := range sectNames {
		stats[i].Name = name
		index[name] = i
	}

	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		idx, ok := index[agents.Attrs[i].Str["sect"]]
		if !ok {
			continue
		}
		cp := agents.Attrs[i].Num["combat_power"]
		stats[idx].Count++
		stats[idx].CombatValue += cp * cp
		realm := int(agents.Attrs[i].Num["realm"])
		if realm < 1 {
			realm = 1
		}
		if realm > 5 {
			realm = 5
		}
		stats[idx].RealmCounts[realm]++
		if cp > stats[idx].MaxCombatPower {
			stats[idx].MaxCombatPower = cp
		}
	}
	return stats
}

func initializeSectWeights(rng *engine.RNG) {
	sectWeights = make([]float64, len(sectNames))
	total := 0.0
	for i := range sectWeights {
		weight := 0.05 + rng.Float64()*rng.Float64()
		sectWeights[i] = weight
		total += weight
	}
	if total <= 0 {
		return
	}
	for i := range sectWeights {
		sectWeights[i] /= total
	}
}

func sectRecruitWeights(agents *engine.AgentStore) []float64 {
	cfg := DefaultScenarioConfig()
	stats := CalculateSectStats(agents)
	weights := make([]float64, len(stats))
	total := 0.0
	for i, stat := range stats {
		trait := sectTraitForIndex(i)
		base := cfg.SectRecruitBaseWeight * trait.RecruitMultiplier
		power := 0.0
		if stat.CombatValue > 0 {
			exponent := cfg.SectRecruitCombatExponent
			if exponent <= 0 {
				exponent = 0.5
			}
			power = math.Pow(stat.CombatValue, exponent) * trait.PowerRecruitMultiplier
		}
		weights[i] = base + power
		total += weights[i]
	}
	if total > 0 {
		return weights
	}
	if len(sectWeights) != len(sectNames) {
		initializeSectWeights(engine.NewRNG(1))
	}
	copy(weights, sectWeights)
	return weights
}

func sectTraitForName(name string) SectTrait {
	for i, sectName := range sectNames {
		if sectName == name {
			return sectTraitForIndex(i)
		}
	}
	return SectTrait{RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1}
}

func sectTraitForIndex(idx int) SectTrait {
	if idx >= 0 && idx < len(sectTraits) {
		return sectTraits[idx]
	}
	return SectTrait{RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1}
}

func weightedSectName(rng *engine.RNG, weights []float64) string {
	if len(sectWeights) != len(sectNames) {
		initializeSectWeights(rng)
	}
	if len(weights) != len(sectNames) {
		weights = sectWeights
	}
	roll := rng.Float64()
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 {
		weights = sectWeights
		total = 1
	}
	roll *= total
	acc := 0.0
	for i, weight := range weights {
		acc += weight
		if roll <= acc {
			return sectNames[i]
		}
	}
	return sectNames[len(sectNames)-1]
}

func clampNorm(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
