package cultivation

import (
	"math"
	"testing"

	"github.com/runmin/sugarscape/engine"
)

func TestSetupNormalizesMortalPopulation(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 40
	cfg.GridHeight = 40
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	Setup(w)

	scnCfg := DefaultScenarioConfig()
	want := float64(cfg.GridWidth*cfg.GridHeight) * scnCfg.MortalBaseDensity
	got := w.Curr.Env.TotalMortals()
	if math.Abs(got-want) > want*1e-9 {
		t.Fatalf("total mortals = %f, want %f", got, want)
	}
	if next := w.Next.Env.TotalMortals(); math.Abs(next-want) > want*1e-9 {
		t.Fatalf("next-frame total mortals = %f, want %f", next, want)
	}
}

func TestBreakthroughUsesNewRealmQiMax(t *testing.T) {
	oldBreakthrough := DefaultRealms[0].BreakthroughBase
	DefaultRealms[0].BreakthroughBase = 1.0
	defer func() { DefaultRealms[0].BreakthroughBase = oldBreakthrough }()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 200
	attrs.Num["qi_max"] = 200
	attrs.Num["lifespan"] = 120
	attrs.Num["cultivation_speed"] = 0
	attrs.Num["breakthrough_cooldown"] = 0
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&CultivationSystem{}).Tick(w)

	got := w.Next.Agents.Attrs[0].Num
	if got["realm"] != 2 {
		t.Fatalf("realm = %v, want 2", got["realm"])
	}
	if got["qi_max"] != 600 {
		t.Fatalf("qi_max = %v, want 600", got["qi_max"])
	}
	if got["qi"] != 300 {
		t.Fatalf("qi = %v, want 300", got["qi"])
	}
	if got["combat_power"] != 450 {
		t.Fatalf("combat_power = %v, want 450", got["combat_power"])
	}
}

func TestBreakthroughCooldownDoublesByRealm(t *testing.T) {
	cfg := DefaultScenarioConfig()
	cases := []struct {
		realm int
		want  int
	}{
		{realm: 1, want: 20},
		{realm: 2, want: 40},
		{realm: 3, want: 80},
		{realm: 4, want: 160},
	}

	for _, tc := range cases {
		if got := breakthroughCooldownTicks(cfg, tc.realm); got != tc.want {
			t.Fatalf("realm %d cooldown = %d, want %d", tc.realm, got, tc.want)
		}
	}
}

func TestDefaultRealmBreakthroughProbabilitiesMatchStrategy(t *testing.T) {
	want := []float64{0.10, 0.05, 0.05, 0.05, 0}
	for i, prob := range want {
		if got := DefaultRealms[i].BreakthroughBase; got != prob {
			t.Fatalf("realm %d breakthrough probability = %v, want %v", i+1, got, prob)
		}
	}
}

func TestBreakthroughToHuashenRecordsBirthReason(t *testing.T) {
	oldBreakthrough := DefaultRealms[3].BreakthroughBase
	DefaultRealms[3].BreakthroughBase = 1.0
	defer func() { DefaultRealms[3].BreakthroughBase = oldBreakthrough }()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 4
	attrs.Num["qi"] = 6000
	attrs.Num["qi_max"] = 6000
	attrs.Num["lifespan"] = 1000
	attrs.Num["cultivation_speed"] = 0
	attrs.Num["breakthrough_cooldown"] = 0
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&CultivationSystem{}).Tick(w)

	events := w.Stats.DrainNotableEvents()
	if len(events) != 1 {
		t.Fatalf("notable events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "诞生" || ev.Realm != "化神" || ev.Reason != "元婴 -> 化神" {
		t.Fatalf("event = %+v, want huashen birth breakthrough reason", ev)
	}
}

func TestBreakthroughToYuanyingRecordsBirthReason(t *testing.T) {
	oldBreakthrough := DefaultRealms[2].BreakthroughBase
	DefaultRealms[2].BreakthroughBase = 1.0
	defer func() { DefaultRealms[2].BreakthroughBase = oldBreakthrough }()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 3
	attrs.Num["qi"] = 2000
	attrs.Num["qi_max"] = 2000
	attrs.Num["lifespan"] = 500
	attrs.Num["cultivation_speed"] = 0
	attrs.Num["breakthrough_cooldown"] = 0
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&CultivationSystem{}).Tick(w)

	events := w.Stats.DrainNotableEvents()
	if len(events) != 1 {
		t.Fatalf("notable events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "诞生" || ev.Realm != "元婴" || ev.Reason != "金丹 -> 元婴" {
		t.Fatalf("event = %+v, want yuanying birth breakthrough reason", ev)
	}
}

func TestHuashenNaturalDeathRecordsReason(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 5
	attrs.Num["qi"] = 123
	attrs.Num["age"] = 3000
	attrs.Num["qi_max"] = 20000
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&LifecycleSystem{}).Tick(w)

	events := w.Stats.DrainNotableEvents()
	if len(events) != 1 {
		t.Fatalf("notable events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "死亡" || ev.Realm != "化神" || ev.Reason != "寿元耗尽" {
		t.Fatalf("event = %+v, want huashen natural death reason", ev)
	}
}

func TestYuanyingNaturalDeathRecordsReason(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 4
	attrs.Num["qi"] = 123
	attrs.Num["age"] = 1000
	attrs.Num["qi_max"] = 6000
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&LifecycleSystem{}).Tick(w)

	events := w.Stats.DrainNotableEvents()
	if len(events) != 1 {
		t.Fatalf("notable events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "死亡" || ev.Realm != "元婴" || ev.Reason != "寿元耗尽" {
		t.Fatalf("event = %+v, want yuanying natural death reason", ev)
	}
}

func TestEffectiveCombatDeathChanceScalesByAdvantageAndRealm(t *testing.T) {
	cfg := DefaultScenarioConfig()

	got := effectiveCombatDeathChance(cfg, 1000, 250, 4)
	want := cfg.CombatDeathChance * 0.75 * 0.35
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("yuanying death chance = %v, want %v", got, want)
	}

	upset := effectiveCombatDeathChance(cfg, 250, 1000, 4)
	wantUpset := cfg.CombatDeathChance * 0.05 * 0.35
	if math.Abs(upset-wantUpset) > 1e-12 {
		t.Fatalf("yuanying upset death chance = %v, want %v", upset, wantUpset)
	}

	huashen := effectiveCombatDeathChance(cfg, 1000, 250, 5)
	wantHuashen := cfg.CombatDeathChance * 0.75 * 0.15
	if math.Abs(huashen-wantHuashen) > 1e-12 {
		t.Fatalf("huashen death chance = %v, want %v", huashen, wantHuashen)
	}
}

func TestCombatCostUsesOpponentQi(t *testing.T) {
	cfg := DefaultScenarioConfig()
	got := combatCost(cfg, 1000, 100)
	want := 100*cfg.CombatCostBase + 1000*cfg.CombatSelfMinCost
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("combat cost = %v, want %v", got, want)
	}
}

func TestReturnedDeathQiLosesFixedFraction(t *testing.T) {
	cfg := DefaultScenarioConfig()
	got := returnedDeathQi(cfg, 100, 30)
	want := 50.0
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("returned death qi = %v, want %v", got, want)
	}
}

func TestStrongerSecondCultivatorAttacksOnFleeThreshold(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	weak := engine.NewAttrBag()
	weak.Num["realm"] = 1
	weak.Num["combat_power"] = 10
	weak.Num["qi"] = 100
	weak.Num["qi_max"] = 100
	strong := engine.NewAttrBag()
	strong.Num["realm"] = 1
	strong.Num["combat_power"] = 40
	strong.Num["qi"] = 100
	strong.Num["qi_max"] = 100

	w.Next.Agents.Add("cultivator", 1, 1, weak)
	w.Next.Agents.Add("cultivator", 1, 1, strong)
	pendingFights = nil

	(&InteractionSystem{}).Tick(w)

	if len(pendingFights) != 1 {
		t.Fatalf("pending fights = %d, want 1", len(pendingFights))
	}
	if pendingFights[0].Attacker != 1 || pendingFights[0].Defender != 0 {
		t.Fatalf("fight = %+v, want attacker 1 defender 0", pendingFights[0])
	}
	pendingFights = nil
}

func TestFleeThresholdAttackScalesWithQi(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	weak := engine.NewAttrBag()
	weak.Num["realm"] = 1
	weak.Num["combat_power"] = 10
	weak.Num["qi"] = 100
	weak.Num["qi_max"] = 100
	exhaustedStrong := engine.NewAttrBag()
	exhaustedStrong.Num["realm"] = 1
	exhaustedStrong.Num["combat_power"] = 40
	exhaustedStrong.Num["qi"] = 0
	exhaustedStrong.Num["qi_max"] = 100

	w.Next.Agents.Add("cultivator", 1, 1, weak)
	w.Next.Agents.Add("cultivator", 1, 1, exhaustedStrong)
	pendingFights = nil

	(&InteractionSystem{}).Tick(w)

	if len(pendingFights) != 0 {
		t.Fatalf("pending fights = %d, want 0 for exhausted stronger cultivator", len(pendingFights))
	}
}

func TestAttackDesireScalesWithQi(t *testing.T) {
	attacker := engine.NewAttrBag()
	attacker.Num["aggression"] = 1
	attacker.Num["perceived_cp_mult"] = 1
	attacker.Num["combat_power"] = 100
	attacker.Num["qi"] = 50
	attacker.Num["qi_max"] = 100
	defender := engine.NewAttrBag()
	defender.Num["combat_power"] = 25
	defender.Num["qi"] = 25

	got := attackDesire(attacker, defender)
	want := 0.5 * math.Sqrt(0.75) * 0.625 * 0.804
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("attack desire = %v, want %v", got, want)
	}
}

func TestExpectedCombatLossFactorPenalizesRiskyFights(t *testing.T) {
	cfg := DefaultScenarioConfig()

	favorable := combatLossFactorForTest(100, 100, 25, 25, cfg)
	even := combatLossFactorForTest(100, 100, 100, 100, cfg)
	unfavorable := combatLossFactorForTest(25, 25, 100, 100, cfg)

	if !(favorable > even && even > unfavorable) {
		t.Fatalf("loss factors favorable=%v even=%v unfavorable=%v, want descending by risk", favorable, even, unfavorable)
	}
}

func combatLossFactorForTest(attackerCP, attackerQi, defenderCP, defenderQi float64, cfg ScenarioConfig) float64 {
	attacker := engine.NewAttrBag()
	attacker.Num["combat_power"] = attackerCP
	attacker.Num["qi"] = attackerQi
	defender := engine.NewAttrBag()
	defender.Num["combat_power"] = defenderCP
	defender.Num["qi"] = defenderQi
	return expectedCombatLossFactor(attacker, defender, cfg)
}

func TestConversionSpiritFactors(t *testing.T) {
	cfg := DefaultScenarioConfig()
	if got := proportionalFactor(10, 20); got != 0.5 {
		t.Fatalf("global proportional factor = %v, want 0.5", got)
	}
	if got := conversionLocalSpiritFactor(5, cfg); got != 0.5 {
		t.Fatalf("local conversion factor = %v, want 0.5", got)
	}
}

func TestMovementProbabilityScalesWithCellSpirit(t *testing.T) {
	env := engine.NewGrid(1, 1)
	env.SetEnv1(0, 0, 100)

	env.SetEnv0(0, 0, 100)
	if got := movementProbability(env, 0, 0); got != 0 {
		t.Fatalf("movement probability at full spirit = %v, want 0", got)
	}

	env.SetEnv0(0, 0, 25)
	if got := movementProbability(env, 0, 0); got != 0.75 {
		t.Fatalf("movement probability at quarter spirit = %v, want 0.75", got)
	}

	env.SetEnv0(0, 0, 0)
	if got := movementProbability(env, 0, 0); got != 1 {
		t.Fatalf("movement probability at zero spirit = %v, want 1", got)
	}
}

func TestLowQiCultivatorMovesLessUnlessCellSpiritIsPoor(t *testing.T) {
	env := engine.NewGrid(1, 1)
	env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["qi"] = 40
	attrs.Num["qi_max"] = 100

	env.SetEnv0(0, 0, 50)
	got := movementProbabilityForCultivator(env, 0, 0, attrs)
	want := 0.5 * 0.5
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("movement probability for low qi in adequate cell = %v, want %v", got, want)
	}

	env.SetEnv0(0, 0, 20)
	if got := movementProbabilityForCultivator(env, 0, 0, attrs); got != 0.8 {
		t.Fatalf("movement probability for low qi in poor cell = %v, want 0.8", got)
	}
}

func TestEnvironmentKeepsReturnedQiAboveSpiritMax(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	for i := range w.Next.Env.Cells {
		w.Next.Env.Cells[i].Env0 = 0
		w.Next.Env.Cells[i].Env1 = 100
		w.Next.Env.Cells[i].Env2 = 0
	}
	w.Next.Env.SetEnv0(1, 1, 200)

	(&EnvironmentSystem{}).Tick(w)

	center := w.Next.Env.Env0(1, 1)
	if center <= 100 {
		t.Fatalf("center spirit after diffusion = %v, want above spirit_max", center)
	}
	neighbor := w.Next.Env.Env0(0, 0)
	if neighbor <= 0 {
		t.Fatalf("neighbor spirit after diffusion = %v, want returned qi to diffuse outward", neighbor)
	}
}

func TestEnvironmentRegenStillClampsBelowMax(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 99)
	w.Next.Env.SetEnv1(0, 0, 100)
	w.Next.Env.SetEnv2(0, 0, 5)

	(&EnvironmentSystem{}).Tick(w)

	if got := w.Next.Env.Env0(0, 0); math.Abs(got-100) > 1e-9 {
		t.Fatalf("spirit after regen = %v, want clamped to spirit_max", got)
	}
}

func TestMovedCultivatorDoesNotAbsorbSpirit(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 100)
	w.Next.Env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 0
	attrs.Num["qi_max"] = 200
	attrs.Num["cultivation_speed"] = 1
	attrs.Num["moved_this_tick"] = 1
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&CultivationSystem{}).Tick(w)

	if got := w.Next.Agents.Attrs[0].Num["qi"]; got != 0 {
		t.Fatalf("moved cultivator qi = %v, want 0", got)
	}
	if got := w.Next.Env.Env0(0, 0); got != 100 {
		t.Fatalf("cell spirit after moved cultivator = %v, want 100", got)
	}
}

func TestStationaryCultivatorAbsorbsSpirit(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 100)
	w.Next.Env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 0
	attrs.Num["qi_max"] = 200
	attrs.Num["cultivation_speed"] = 1
	attrs.Num["moved_this_tick"] = 0
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&CultivationSystem{}).Tick(w)

	if got := w.Next.Agents.Attrs[0].Num["qi"]; got <= 0 {
		t.Fatalf("stationary cultivator qi = %v, want > 0", got)
	}
	if got := w.Next.Env.Env0(0, 0); got >= 100 {
		t.Fatalf("cell spirit after stationary cultivator = %v, want < 100", got)
	}
}

func TestInteractionOnlyTriggersOnSameCell(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 5
	cfg.GridHeight = 5
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	a := engine.NewAttrBag()
	a.Num["realm"] = 5
	a.Num["combat_power"] = 1000
	b := engine.NewAttrBag()
	b.Num["realm"] = 1
	b.Num["combat_power"] = 10

	w.Next.Agents.Add("cultivator", 2, 2, a)
	w.Next.Agents.Add("cultivator", 3, 2, b)
	pendingFights = nil

	(&InteractionSystem{}).Tick(w)

	if len(pendingFights) != 0 {
		t.Fatalf("pending fights = %d, want 0 for adjacent cultivators", len(pendingFights))
	}
}

func TestNaturalDeathReturnsQiToWorld(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 123
	attrs.Num["age"] = 120
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	before := w.Next.Env.Env0(1, 1)
	(&LifecycleSystem{}).Tick(w)

	if w.Next.Agents.Alive[0] {
		t.Fatal("cultivator is alive, want natural death")
	}
	want := before + 123*(1-DefaultScenarioConfig().DeathQiLossFrac)
	if got := w.Next.Env.Env0(1, 1); math.Abs(got-want) > 1e-12 {
		t.Fatalf("cell spirit = %v, want %v", got, want)
	}
}

func TestLifecycleDoesNotBirthCultivators(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 100
	attrs.Num["age"] = 30
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&LifecycleSystem{}).Tick(w)

	if got := len(w.Next.Agents.ID); got != 1 {
		t.Fatalf("cultivator slots = %d, want 1", got)
	}
	if !w.Next.Agents.Alive[0] {
		t.Fatal("cultivator died unexpectedly")
	}
}

func TestLowSpiritExposureCanKillCultivator(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 0)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 123
	attrs.Num["qi_max"] = 200
	attrs.Num["age"] = 30
	attrs.Num["low_spirit_years"] = 13
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	tries := 0
	for w.Next.Agents.Alive[0] && tries < 100 {
		(&LifecycleSystem{}).Tick(w)
		tries++
	}

	if w.Next.Agents.Alive[0] {
		t.Fatal("cultivator survived repeated low-spirit death checks, want eventual death")
	}
	want := 123 * (1 - DefaultScenarioConfig().DeathQiLossFrac)
	if got := w.Next.Env.Env0(0, 0); got < want {
		t.Fatalf("cell spirit after low-spirit death = %v, want at least %v", got, want)
	}
}

func TestMortalBirthRateRange(t *testing.T) {
	cfg := DefaultScenarioConfig()
	if cfg.MortalBirthRateMin != 0.9 {
		t.Fatalf("MortalBirthRateMin = %v, want 0.9", cfg.MortalBirthRateMin)
	}
	if cfg.MortalBirthRateMax != 1.2 {
		t.Fatalf("MortalBirthRateMax = %v, want 1.2", cfg.MortalBirthRateMax)
	}
}
