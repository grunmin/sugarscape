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
	MortalCoreRadius                   int
	MortalInnerRadius                  int
	MortalOuterRadius                  int
	MortalCoreDensityMultiplier        float64
	MortalInnerDensityMultiplier       float64
	MortalOuterDensityMultiplier       float64
	MortalWildernessDensityMultiplier  float64
	MortalLifespan                     float64 // years
	MortalConvChance                   float64 // lifetime probability of becoming cultivator
	MortalBirthRateMin                 float64 // multiplier on base mortality rate
	MortalBirthRateMax                 float64 // multiplier on base mortality rate
	ConversionGlobalSpiritThresholdAvg float64
	ConversionLocalSpiritThreshold     float64
	ConversionSpiritCheckEvery         int
	ConversionSpawnSpiritFloor         float64
	ConversionSpawnSpiritExponent      float64
	// Spirit density
	BaseSpiritDensity      float64
	SpiritRegenRate        float64
	SpiritMax              float64
	SpiritDiffusionRate    float64
	NumSpiritSprings       int
	SpiritSpringRadius     int
	SpiritSpringBoost      float64
	SpiritSpringMaxBonus   float64
	SpiritSpringRegenBonus float64
	NumSpiritVeins         int
	SpiritVeinLength       int
	SpiritVeinRadius       int
	SpiritVeinBoost        float64
	SpiritVeinMaxBonus     float64
	SpiritVeinRegenBonus   float64
	NumBlessedLands        int
	BlessedLandRadius      int
	BlessedLandBoost       float64
	BlessedLandMaxBonus    float64
	BlessedLandRegenBonus  float64
	EnvironmentTickEvery   int
	// Cultivation
	BaseQi                     float64
	CultivationSpeed           float64
	CultivatorUpkeepQiFrac     float64 // fraction of realm qi max consumed per tick while alive
	BreakthroughQiFrac         float64
	BreakthroughSustainTicks   []int
	BreakthroughPostQiFrac     float64 // fraction of the new realm qi max retained after breakthrough
	BreakthroughCD             int     // ticks of cooldown after failed breakthrough
	JindanBreakFailDeathChance float64
	// Sects
	LooseBreakthroughMultiplier        float64
	SectAllyCombatAssist               float64 // fraction of same-sect same-cell combat power counted in attack judgment
	SectBreakthroughBonus              float64 // relative breakthrough probability bonus for sect cultivators
	SectMentorBonusCap                 float64 // max extra breakthrough multiplier from one-realm-higher sect mentors
	SectMentorScale                    float64 // saturation scale for one-realm-higher mentor count
	SectFormationCheckEvery            int
	SectFormationRadius                int
	SectFormationInfluenceRadius       int
	SectFormationMinCultivators        int
	SectFormationMinSustainTicks       int
	SectFormationMinCombatDeaths       int
	SectFormationMinSpiritMaxBonus     float64
	SectFormationMinRegenBonus         float64
	SectFormationExistingSectExclusion int
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
		MortalCoreRadius:                   3,
		MortalInnerRadius:                  10,
		MortalOuterRadius:                  18,
		MortalCoreDensityMultiplier:        8.0,
		MortalInnerDensityMultiplier:       3.0,
		MortalOuterDensityMultiplier:       0.6,
		MortalWildernessDensityMultiplier:  0.0,
		MortalLifespan:                     70,
		MortalConvChance:                   0.001,
		MortalBirthRateMin:                 0.9,
		MortalBirthRateMax:                 1.2,
		ConversionGlobalSpiritThresholdAvg: 20,
		ConversionLocalSpiritThreshold:     10,
		ConversionSpiritCheckEvery:         20,
		ConversionSpawnSpiritFloor:         0.02,
		ConversionSpawnSpiritExponent:      3.0,
		BaseSpiritDensity:                  30,
		SpiritRegenRate:                    0.02,
		SpiritMax:                          100,
		SpiritDiffusionRate:                0.02,
		NumSpiritSprings:                   30,
		SpiritSpringRadius:                 8,
		SpiritSpringBoost:                  60,
		SpiritSpringMaxBonus:               80,
		SpiritSpringRegenBonus:             0.08,
		NumSpiritVeins:                     12,
		SpiritVeinLength:                   80,
		SpiritVeinRadius:                   6,
		SpiritVeinBoost:                    45,
		SpiritVeinMaxBonus:                 120,
		SpiritVeinRegenBonus:               0.05,
		NumBlessedLands:                    4,
		BlessedLandRadius:                  18,
		BlessedLandBoost:                   140,
		BlessedLandMaxBonus:                260,
		BlessedLandRegenBonus:              0.16,
		EnvironmentTickEvery:               5,
		BaseQi:                             100,
		CultivationSpeed:                   0.5,
		CultivatorUpkeepQiFrac:             0.0001,
		BreakthroughQiFrac:                 0.95,
		BreakthroughSustainTicks:           []int{50, 200, 500, 1000},
		BreakthroughPostQiFrac:             0.25,
		BreakthroughCD:                     100,
		JindanBreakFailDeathChance:         0.30,
		LooseBreakthroughMultiplier:        0.65,
		SectAllyCombatAssist:               0.25,
		SectBreakthroughBonus:              0.30,
		SectMentorBonusCap:                 0.50,
		SectMentorScale:                    10,
		SectFormationCheckEvery:            10,
		SectFormationRadius:                18,
		SectFormationInfluenceRadius:       36,
		SectFormationMinCultivators:        35,
		SectFormationMinSustainTicks:       80,
		SectFormationMinCombatDeaths:       3,
		SectFormationMinSpiritMaxBonus:     45,
		SectFormationMinRegenBonus:         0.04,
		SectFormationExistingSectExclusion: 72,
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
