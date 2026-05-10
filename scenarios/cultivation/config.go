package cultivation

// RealmConfig defines a cultivation realm's parameters.
type RealmConfig struct {
	Name             string
	Level            int
	QiMultiplier     float64 // qi_max = BaseQi * this
	CombatMultiplier float64
	Lifespan         float64
	BreakthroughBase float64 // base probability to break through to next realm
	CultSpeedMult    float64 // cultivation speed multiplier
	MoveSpeed        float64 // cells per tick (fractional part = extra step probability)
	DetectRange      int     // detection range in cells (= realm level)
}

var DefaultRealms = []RealmConfig{
	{Name: "练气", Level: 1, QiMultiplier: 2, CombatMultiplier: 1, Lifespan: 120, BreakthroughBase: 0.10, CultSpeedMult: 1.0, MoveSpeed: 1.0, DetectRange: 1},
	{Name: "筑基", Level: 2, QiMultiplier: 6, CombatMultiplier: 3, Lifespan: 250, BreakthroughBase: 0.05, CultSpeedMult: 1.5, MoveSpeed: 1.3, DetectRange: 2},
	{Name: "金丹", Level: 3, QiMultiplier: 20, CombatMultiplier: 10, Lifespan: 500, BreakthroughBase: 0.03, CultSpeedMult: 2.0, MoveSpeed: 1.6, DetectRange: 3},
	{Name: "元婴", Level: 4, QiMultiplier: 60, CombatMultiplier: 30, Lifespan: 1000, BreakthroughBase: 0.02, CultSpeedMult: 2.5, MoveSpeed: 1.9, DetectRange: 4},
	{Name: "化神", Level: 5, QiMultiplier: 200, CombatMultiplier: 100, Lifespan: 3000, BreakthroughBase: 0.00, CultSpeedMult: 3.0, MoveSpeed: 2.2, DetectRange: 5},
}

// ScenarioConfig holds all configurable parameters for the cultivation world.
type ScenarioConfig struct {
	// Mortal world
	MortalBaseDensity                  float64 // average mortals per cell
	NumTribes                          int     // number of tribal centers
	MortalLifespan                     float64 // years
	MortalConvChance                   float64 // lifetime probability of becoming cultivator
	MortalBirthRateMin                 float64 // multiplier on base mortality rate
	MortalBirthRateMax                 float64 // multiplier on base mortality rate
	ConversionGlobalSpiritThresholdAvg float64
	ConversionLocalSpiritThreshold     float64
	ConversionSpiritCheckEvery         int
	// Spirit density
	BaseSpiritDensity    float64
	SpiritRegenRate      float64
	SpiritMax            float64
	NumSpiritSprings     int
	EnvironmentTickEvery int
	// Cultivation
	BaseQi                     float64
	CultivationSpeed           float64
	CultivatorUpkeepQiFrac     float64 // fraction of realm qi max consumed per tick while alive
	BreakthroughQiFrac         float64
	BreakthroughSustainTicks   []int
	BreakthroughPostQiFrac     float64 // fraction of the new realm qi max retained after breakthrough
	BreakthroughCD             int     // ticks of cooldown after failed breakthrough
	JindanBreakFailDeathChance float64
	// Combat
	CombatDeathChance    float64
	CombatCostBase       float64 // fraction of opponent qi paid by winner in combat
	CombatSelfMinCost    float64 // minimum fraction of winner qi paid in combat
	DeathQiLossFrac      float64 // fraction of dead cultivator qi permanently lost
	LowSpiritDeathQiFrac float64 // qi fraction required before low-spirit exposure can kill
	FleeThreshold        float64
}

func DefaultScenarioConfig() ScenarioConfig {
	return ScenarioConfig{
		MortalBaseDensity:                  100,
		NumTribes:                          200,
		MortalLifespan:                     70,
		MortalConvChance:                   0.001,
		MortalBirthRateMin:                 0.9,
		MortalBirthRateMax:                 1.2,
		ConversionGlobalSpiritThresholdAvg: 20,
		ConversionLocalSpiritThreshold:     10,
		ConversionSpiritCheckEvery:         20,
		BaseSpiritDensity:                  30,
		SpiritRegenRate:                    0.05,
		SpiritMax:                          100,
		NumSpiritSprings:                   20,
		EnvironmentTickEvery:               5,
		BaseQi:                             100,
		CultivationSpeed:                   0.5,
		CultivatorUpkeepQiFrac:             0.0001,
		BreakthroughQiFrac:                 0.95,
		BreakthroughSustainTicks:           []int{50, 200, 500, 1000},
		BreakthroughPostQiFrac:             0.25,
		BreakthroughCD:                     100,
		JindanBreakFailDeathChance:         0.30,
		CombatDeathChance:                  0.3,
		CombatCostBase:                     0.20,
		CombatSelfMinCost:                  0.02,
		DeathQiLossFrac:                    0.20,
		LowSpiritDeathQiFrac:               0.20,
		FleeThreshold:                      3.0,
	}
}

// GetRealm returns the realm config for a given level.
func GetRealm(level int) RealmConfig {
	idx := level - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(DefaultRealms) {
		idx = len(DefaultRealms) - 1
	}
	return DefaultRealms[idx]
}
