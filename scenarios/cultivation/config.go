package cultivation

// RealmConfig defines a cultivation realm's parameters.
type RealmConfig struct {
	Name             string
	Level            int
	QiMultiplier     float64 // qi_max = 100 * this
	CombatMultiplier float64
	Lifespan         float64
	BreakthroughBase float64 // base probability to break through to next realm
}

var DefaultRealms = []RealmConfig{
	{Name: "练气", Level: 1, QiMultiplier: 1, CombatMultiplier: 1, Lifespan: 120, BreakthroughBase: 0.3},
	{Name: "筑基", Level: 2, QiMultiplier: 3, CombatMultiplier: 3, Lifespan: 250, BreakthroughBase: 0.2},
	{Name: "金丹", Level: 3, QiMultiplier: 10, CombatMultiplier: 10, Lifespan: 500, BreakthroughBase: 0.1},
	{Name: "元婴", Level: 4, QiMultiplier: 30, CombatMultiplier: 30, Lifespan: 1000, BreakthroughBase: 0.05},
	{Name: "化神", Level: 5, QiMultiplier: 100, CombatMultiplier: 100, Lifespan: 3000, BreakthroughBase: 0.0},
}

// ScenarioConfig holds all configurable parameters for the cultivation world.
type ScenarioConfig struct {
	// Initial population
	InitialCultivators int
	InitialBeasts      int
	// Realm distribution of initial cultivators (should sum to 1.0)
	InitRealmDist map[int]float64 // realm level → fraction
	// Spirit density
	BaseSpiritDensity float64
	SpiritRegenRate   float64
	SpiritMax         float64
	// Cultivation
	BaseQi             float64
	CultivationSpeed   float64
	BreakthroughQiFrac float64 // fraction of qi_max needed for a breakthrough attempt
	// Combat
	CombatDeathChance float64
	FleeThreshold     float64 // power ratio above which weak side flees
	// Lifecycle
	BaseBirthRate float64 // births per cultivator per tick
	BirthCooldown float64 // minimum age difference between parent and first child
	// Beast
	BeastCombatBase float64
	BeastQiReward   float64
}

func DefaultScenarioConfig() ScenarioConfig {
	return ScenarioConfig{
		InitialCultivators: 500,
		InitialBeasts:      100,
		InitRealmDist: map[int]float64{
			1: 0.80,
			2: 0.15,
			3: 0.05,
		},
		BaseSpiritDensity:  30,
		SpiritRegenRate:    0.5,
		SpiritMax:          100,
		BaseQi:             100,
		CultivationSpeed:   0.5,
		BreakthroughQiFrac: 0.9,
		CombatDeathChance:  0.3,
		FleeThreshold:      3.0,
		BaseBirthRate:      0.005,
		BirthCooldown:      20,
		BeastCombatBase:    5,
		BeastQiReward:      20,
	}
}

// GetRealm returns the realm config for a given level, or the last realm if level exceeds.
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
