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
	attrs.Num["breakthrough_sustain_ticks"] = 49
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&CultivationSystem{}).Tick(w)

	got := w.Next.Agents.Attrs[0].Num
	if got["realm"] != 2 {
		t.Fatalf("realm = %v, want 2", got["realm"])
	}
	if got["qi_max"] != 600 {
		t.Fatalf("qi_max = %v, want 600", got["qi_max"])
	}
	if got["qi"] != 150 {
		t.Fatalf("qi = %v, want 150", got["qi"])
	}
	if got["combat_power"] != 375 {
		t.Fatalf("combat_power = %v, want 375", got["combat_power"])
	}
	if got["lifespan"] < 150 || got["lifespan"] > 250 {
		t.Fatalf("lifespan = %v, want in [150, 250]", got["lifespan"])
	}
}

func TestRandomLifespanCanDropByFortyPercent(t *testing.T) {
	rng := engine.NewRNG(42)
	for _, rc := range DefaultRealms {
		for range 100 {
			got := randomLifespan(rng, rc)
			if got < rc.Lifespan*0.6 || got > rc.Lifespan {
				t.Fatalf("%s random lifespan = %v, want in [%v, %v]", rc.Name, got, rc.Lifespan*0.6, rc.Lifespan)
			}
		}
	}
}

func TestBreakthroughCooldownDoublesByRealm(t *testing.T) {
	cfg := DefaultScenarioConfig()
	cases := []struct {
		realm int
		want  int
	}{
		{realm: 1, want: 100},
		{realm: 2, want: 200},
		{realm: 3, want: 400},
		{realm: 4, want: 800},
	}

	for _, tc := range cases {
		if got := breakthroughCooldownTicks(cfg, tc.realm); got != tc.want {
			t.Fatalf("realm %d cooldown = %d, want %d", tc.realm, got, tc.want)
		}
	}
}

func TestBreakthroughRequiresSustainedHighQi(t *testing.T) {
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
	if got["realm"] != 1 {
		t.Fatalf("realm = %v, want 1 before sustain requirement is met", got["realm"])
	}
	if got["breakthrough_sustain_ticks"] != 1 {
		t.Fatalf("breakthrough_sustain_ticks = %v, want 1", got["breakthrough_sustain_ticks"])
	}

	got["qi"] = 189
	(&CultivationSystem{}).Tick(w)
	if got["breakthrough_sustain_ticks"] != 0 {
		t.Fatalf("breakthrough_sustain_ticks = %v, want reset to 0 below threshold", got["breakthrough_sustain_ticks"])
	}
}

func TestDefaultRealmBreakthroughProbabilitiesMatchStrategy(t *testing.T) {
	want := []float64{0.10, 0.05, 0.03, 0.02, 0}
	for i, prob := range want {
		if got := DefaultRealms[i].BreakthroughBase; got != prob {
			t.Fatalf("realm %d breakthrough probability = %v, want %v", i+1, got, prob)
		}
	}
}

func TestDefaultBreakthroughThresholdsMatchStrategy(t *testing.T) {
	cfg := DefaultScenarioConfig()
	if cfg.BreakthroughQiFrac != 0.95 {
		t.Fatalf("breakthrough qi fraction = %v, want 0.95", cfg.BreakthroughQiFrac)
	}
	if cfg.BreakthroughPostQiFrac != 0.25 {
		t.Fatalf("breakthrough post qi fraction = %v, want 0.25", cfg.BreakthroughPostQiFrac)
	}
	want := []int{50, 200, 500, 1000}
	if len(cfg.BreakthroughSustainTicks) != len(want) {
		t.Fatalf("breakthrough sustain ticks = %v, want %v", cfg.BreakthroughSustainTicks, want)
	}
	for i, tick := range want {
		if got := cfg.BreakthroughSustainTicks[i]; got != tick {
			t.Fatalf("realm %d breakthrough sustain ticks = %d, want %d", i+1, got, tick)
		}
	}
}

func TestSectBreakthroughProbabilityBonus(t *testing.T) {
	cfg := DefaultScenarioConfig()
	rc := DefaultRealms[0]
	loose := engine.NewAttrBag()
	sect := engine.NewAttrBag()
	sect.Str["sect"] = "宗门1"

	if got := breakthroughProbability(rc, loose, cfg, 10); got != rc.BreakthroughBase {
		t.Fatalf("loose breakthrough probability = %v, want %v", got, rc.BreakthroughBase)
	}
	want := rc.BreakthroughBase * 1.3
	if got := breakthroughProbability(rc, sect, cfg, 0); got != want {
		t.Fatalf("sect breakthrough probability = %v, want %v", got, want)
	}
}

func TestSectMentorsIncreaseBreakthroughProbability(t *testing.T) {
	cfg := DefaultScenarioConfig()
	rc := DefaultRealms[0]
	sect := engine.NewAttrBag()
	sect.Str["sect"] = "宗门1"

	withoutMentor := breakthroughProbability(rc, sect, cfg, 0)
	withMentor := breakthroughProbability(rc, sect, cfg, 25)
	wantMultiplier := 1 + cfg.SectMentorBonusCap*5/(5+cfg.SectMentorScale)
	want := withoutMentor * wantMultiplier
	if math.Abs(withMentor-want) > 1e-12 {
		t.Fatalf("mentor breakthrough probability = %v, want %v", withMentor, want)
	}
	if withMentor <= withoutMentor {
		t.Fatalf("mentor probability = %v, want above no-mentor probability %v", withMentor, withoutMentor)
	}
}

func TestOneRealmHigherMentorsOnlyCountSameSectNextRealm(t *testing.T) {
	w := engine.NewWorld(engine.DefaultEngineConfig())
	lianqi := engine.NewAttrBag()
	lianqi.Num["realm"] = 1
	lianqi.Str["sect"] = "宗门1"
	zhuji := engine.NewAttrBag()
	zhuji.Num["realm"] = 2
	zhuji.Str["sect"] = "宗门1"
	jindan := engine.NewAttrBag()
	jindan.Num["realm"] = 3
	jindan.Str["sect"] = "宗门1"
	otherZhuji := engine.NewAttrBag()
	otherZhuji.Num["realm"] = 2
	otherZhuji.Str["sect"] = "宗门2"

	w.Next.Agents.Add("cultivator", 1, 1, lianqi)
	w.Next.Agents.Add("cultivator", 1, 1, zhuji)
	w.Next.Agents.Add("cultivator", 1, 1, jindan)
	w.Next.Agents.Add("cultivator", 1, 1, otherZhuji)

	counts := countSectRealms(w.Next.Agents)
	if got := oneRealmHigherMentors(lianqi, 1, counts); got != 1 {
		t.Fatalf("lianqi one-realm-higher mentors = %d, want 1", got)
	}
	if got := oneRealmHigherMentors(zhuji, 2, counts); got != 1 {
		t.Fatalf("zhuji one-realm-higher mentors = %d, want 1", got)
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
	attrs.Num["breakthrough_sustain_ticks"] = 999
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
	attrs.Num["breakthrough_sustain_ticks"] = 499
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

func TestJindanBreakthroughFailureCanKill(t *testing.T) {
	oldBreakthrough := DefaultRealms[2].BreakthroughBase
	DefaultRealms[2].BreakthroughBase = 0.000001
	defer func() { DefaultRealms[2].BreakthroughBase = oldBreakthrough }()

	for seed := uint64(1); seed <= 200; seed++ {
		cfg := engine.DefaultEngineConfig()
		cfg.GridWidth = 3
		cfg.GridHeight = 3
		cfg.NumWorkers = 1
		cfg.Seed = seed

		w := engine.NewWorld(cfg)
		attrs := engine.NewAttrBag()
		attrs.Num["realm"] = 3
		attrs.Num["qi"] = 2000
		attrs.Num["qi_max"] = 2000
		attrs.Num["lifespan"] = 500
		attrs.Num["cultivation_speed"] = 0
		attrs.Num["breakthrough_cooldown"] = 0
		attrs.Num["breakthrough_sustain_ticks"] = 499
		w.Next.Agents.Add("cultivator", 1, 1, attrs)

		(&CultivationSystem{}).Tick(w)
		if w.Next.Agents.Alive[0] {
			continue
		}

		events := w.Stats.DrainNotableEvents()
		if len(events) != 1 {
			t.Fatalf("notable events = %d, want 1", len(events))
		}
		ev := events[0]
		if ev.Kind != "死亡" || ev.Realm != "金丹" || ev.Reason != "冲击元婴失败死亡" {
			t.Fatalf("event = %+v, want jindan failed breakthrough death", ev)
		}
		return
	}

	t.Fatal("no jindan failed breakthrough death observed in deterministic seed range")
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

func TestJindanNaturalDeathRecordsReason(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 3
	attrs.Num["qi"] = 123
	attrs.Num["age"] = 500
	attrs.Num["qi_max"] = 2000
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&LifecycleSystem{}).Tick(w)

	events := w.Stats.DrainNotableEvents()
	if len(events) != 1 {
		t.Fatalf("notable events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "死亡" || ev.Realm != "金丹" || ev.Reason != "寿元耗尽" {
		t.Fatalf("event = %+v, want jindan natural death reason", ev)
	}
}

func TestLianqiNaturalDeathRecordsReason(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 123
	attrs.Num["age"] = 120
	attrs.Num["qi_max"] = 200
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	(&LifecycleSystem{}).Tick(w)

	events := w.Stats.DrainNotableEvents()
	if len(events) != 1 {
		t.Fatalf("notable events = %d, want 1", len(events))
	}
	ev := events[0]
	if ev.Kind != "死亡" || ev.Realm != "练气" || ev.Reason != "寿元耗尽" {
		t.Fatalf("event = %+v, want lianqi natural death reason", ev)
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

func TestStrongerSecondCultivatorAttacksWhenDesireIsHigh(t *testing.T) {
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
	strong.Num["aggression"] = 1
	strong.Num["perceived_cp_mult"] = 1

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

func TestAttackDesirePreventsExhaustedStrongCultivatorAttack(t *testing.T) {
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
	exhaustedStrong.Num["aggression"] = 1
	exhaustedStrong.Num["perceived_cp_mult"] = 1

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

func TestBreakthroughPressureDoublesPositiveAttackDesire(t *testing.T) {
	attacker := engine.NewAttrBag()
	attacker.Num["realm"] = 1
	attacker.Num["aggression"] = 1
	attacker.Num["perceived_cp_mult"] = 1
	attacker.Num["combat_power"] = 100
	attacker.Num["qi"] = 90
	attacker.Num["qi_max"] = 100
	attacker.Num["age"] = 79
	attacker.Num["lifespan"] = 100
	defender := engine.NewAttrBag()
	defender.Num["combat_power"] = 25
	defender.Num["qi"] = 25

	normal := attackDesire(attacker, defender)
	attacker.Num["age"] = 80
	pressured := attackDesire(attacker, defender)

	if math.Abs(pressured-normal*2) > 1e-12 {
		t.Fatalf("pressured attack desire = %v, want %v", pressured, normal*2)
	}

	attacker.Num["qi"] = 95
	atThresholdOld := attackDesire(attacker, defender)
	attacker.Num["age"] = 79
	atThresholdYoung := attackDesire(attacker, defender)
	if math.Abs(atThresholdOld-atThresholdYoung) > 1e-12 {
		t.Fatalf("attack desire at breakthrough threshold = %v, want %v", atThresholdOld, atThresholdYoung)
	}
}

func TestResourceCompetitionRaisesAttackDesire(t *testing.T) {
	attacker := engine.NewAttrBag()
	attacker.Num["realm"] = 3
	attacker.Num["aggression"] = 1
	attacker.Num["perceived_cp_mult"] = 1
	attacker.Num["combat_power"] = 100
	attacker.Num["qi"] = 500
	attacker.Num["qi_max"] = 2000
	defender := engine.NewAttrBag()
	defender.Num["realm"] = 3
	defender.Num["combat_power"] = 90
	defender.Num["qi"] = 1800
	defender.Num["qi_max"] = 2000

	richCell := attackDesireWithResource(attacker, defender, 0.8)
	poorCell := attackDesireWithResource(attacker, defender, 0.1)
	if poorCell <= richCell {
		t.Fatalf("poor-cell desire = %v, rich-cell desire = %v, want resource competition to raise desire", poorCell, richCell)
	}
	if poorCell <= 0.35 {
		t.Fatalf("poor-cell desire = %v, want above same-realm attack threshold", poorCell)
	}
}

func TestBreakthroughPressureRaisesResourceCompetition(t *testing.T) {
	cfg := DefaultScenarioConfig()
	low := engine.NewAttrBag()
	low.Num["qi"] = 50
	low.Num["qi_max"] = 100
	near := engine.NewAttrBag()
	near.Num["qi"] = 90
	near.Num["qi_max"] = 100

	lowPressure := breakthroughResourcePressure(low, cfg)
	nearPressure := breakthroughResourcePressure(near, cfg)
	if nearPressure <= lowPressure {
		t.Fatalf("near-threshold pressure = %v, low pressure = %v, want higher pressure near breakthrough", nearPressure, lowPressure)
	}

	near.Num["qi"] = 95
	if got := breakthroughResourcePressure(near, cfg); got != 0 {
		t.Fatalf("breakthrough pressure at threshold = %v, want 0", got)
	}
}

func TestSectDefaultsMatchStrategy(t *testing.T) {
	cfg := DefaultScenarioConfig()
	if cfg.SectMembershipChance != 0.20 {
		t.Fatalf("sect membership chance = %v, want 0.20", cfg.SectMembershipChance)
	}
	if cfg.SectAllyCombatAssist != 0.25 {
		t.Fatalf("sect ally combat assist = %v, want 0.25", cfg.SectAllyCombatAssist)
	}
	if cfg.SectBreakthroughBonus != 0.30 {
		t.Fatalf("sect breakthrough bonus = %v, want 0.30", cfg.SectBreakthroughBonus)
	}
	if cfg.SectMentorBonusCap != 0.50 {
		t.Fatalf("sect mentor bonus cap = %v, want 0.50", cfg.SectMentorBonusCap)
	}
	if cfg.SectMentorScale != 10 {
		t.Fatalf("sect mentor scale = %v, want 10", cfg.SectMentorScale)
	}
	if cfg.SectRecruitBaseWeight != 100 {
		t.Fatalf("sect recruit base weight = %v, want 100", cfg.SectRecruitBaseWeight)
	}
	if len(sectNames) != 7 {
		t.Fatalf("sect count = %d, want 7", len(sectNames))
	}
}

func TestSameSectCellCombatPowerSupportsAttackJudgment(t *testing.T) {
	cfg := DefaultScenarioConfig()
	w := engine.NewWorld(engine.DefaultEngineConfig())

	attacker := engine.NewAttrBag()
	attacker.Num["combat_power"] = 100
	attacker.Num["perceived_cp_mult"] = 1.2
	attacker.Str["sect"] = "宗门1"
	ally := engine.NewAttrBag()
	ally.Num["combat_power"] = 80
	ally.Str["sect"] = "宗门1"
	otherSect := engine.NewAttrBag()
	otherSect.Num["combat_power"] = 1000
	otherSect.Str["sect"] = "宗门2"
	distantAlly := engine.NewAttrBag()
	distantAlly.Num["combat_power"] = 1000
	distantAlly.Str["sect"] = "宗门1"

	w.Next.Agents.Add("cultivator", 1, 1, attacker)
	w.Next.Agents.Add("cultivator", 1, 1, ally)
	w.Next.Agents.Add("cultivator", 1, 1, otherSect)
	w.Next.Agents.Add("cultivator", 2, 1, distantAlly)

	got := effectiveSelfCombatPower(w.Next.Agents, []int{0, 1, 2}, 0, cfg)
	want := 100*1.2 + 80*cfg.SectAllyCombatAssist
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("effective self combat power = %v, want %v", got, want)
	}
}

func TestSectCombatStatsUseSquaredCombatPower(t *testing.T) {
	w := engine.NewWorld(engine.DefaultEngineConfig())
	a := engine.NewAttrBag()
	a.Num["combat_power"] = 3
	a.Num["realm"] = 1
	a.Str["sect"] = "宗门1"
	b := engine.NewAttrBag()
	b.Num["combat_power"] = 4
	b.Num["realm"] = 3
	b.Str["sect"] = "宗门1"
	c := engine.NewAttrBag()
	c.Num["combat_power"] = 10
	c.Num["realm"] = 4
	c.Str["sect"] = "宗门2"

	w.Next.Agents.Add("cultivator", 1, 1, a)
	w.Next.Agents.Add("cultivator", 1, 1, b)
	w.Next.Agents.Add("cultivator", 1, 1, c)

	stats := CalculateSectStats(w.Next.Agents)
	if stats[0].Count != 2 || stats[0].MaxCombatPower != 4 || stats[0].CombatValue != 25 {
		t.Fatalf("sect1 stats = %+v, want count=2 max=4 value=25", stats[0])
	}
	if stats[0].RealmCounts[1] != 1 || stats[0].RealmCounts[3] != 1 {
		t.Fatalf("sect1 realm counts = %+v, want one lianqi and one jindan", stats[0].RealmCounts)
	}
	if stats[1].Count != 1 || stats[1].MaxCombatPower != 10 || stats[1].CombatValue != 100 {
		t.Fatalf("sect2 stats = %+v, want count=1 max=10 value=100", stats[1])
	}
	if stats[1].RealmCounts[4] != 1 {
		t.Fatalf("sect2 realm counts = %+v, want one yuanying", stats[1].RealmCounts)
	}
}

func TestSectRecruitWeightsUseSqrtCombatValue(t *testing.T) {
	w := engine.NewWorld(engine.DefaultEngineConfig())
	a := engine.NewAttrBag()
	a.Num["combat_power"] = 3
	a.Str["sect"] = "宗门1"
	b := engine.NewAttrBag()
	b.Num["combat_power"] = 6
	b.Str["sect"] = "宗门2"

	w.Next.Agents.Add("cultivator", 1, 1, a)
	w.Next.Agents.Add("cultivator", 1, 1, b)

	weights := sectRecruitWeights(w.Next.Agents)
	if weights[0] != 103 || weights[1] != 106 {
		t.Fatalf("sect recruit weights = %v, want first two weights 103 and 106", weights[:2])
	}
	if weights[2] != 100 {
		t.Fatalf("empty sect recruit weight = %v, want 100", weights[2])
	}
}

func TestSameRealmUsesLowerAttackThreshold(t *testing.T) {
	a := engine.NewAttrBag()
	a.Num["realm"] = 3
	b := engine.NewAttrBag()
	b.Num["realm"] = 3
	if got := attackThreshold(a, b); got != 0.35 {
		t.Fatalf("same realm threshold = %v, want 0.35", got)
	}
	b.Num["realm"] = 2
	if got := attackThreshold(a, b); got != 0.5 {
		t.Fatalf("different realm threshold = %v, want 0.5", got)
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

func TestBreakthroughPressureDoublesSpiritSeeking(t *testing.T) {
	env := engine.NewGrid(1, 1)
	env.SetEnv1(0, 0, 100)
	env.SetEnv0(0, 0, 50)

	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 90
	attrs.Num["qi_max"] = 100
	attrs.Num["age"] = 80
	attrs.Num["lifespan"] = 100

	if got := movementProbabilityForCultivator(env, 0, 0, attrs); got != 1 {
		t.Fatalf("pressured movement probability = %v, want capped at 1", got)
	}
	if got := spiritSeekProbability(attrs); got != 1 {
		t.Fatalf("pressured spirit seek probability = %v, want capped at 1", got)
	}

	attrs.Num["age"] = 79
	if got := movementProbabilityForCultivator(env, 0, 0, attrs); got != 0.5 {
		t.Fatalf("normal movement probability = %v, want 0.5", got)
	}
	if got := spiritSeekProbability(attrs); got != 0.7 {
		t.Fatalf("normal spirit seek probability = %v, want 0.7", got)
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

func TestCultivatorAbsorbIsCappedByRemainingCapacity(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 100)
	w.Next.Env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 199
	attrs.Num["qi_max"] = 200
	attrs.Num["cultivation_speed"] = 1
	attrs.Num["moved_this_tick"] = 0
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&CultivationSystem{}).Tick(w)

	if got := w.Next.Agents.Attrs[0].Num["qi"]; got != 200 {
		t.Fatalf("cultivator qi = %v, want capped at 200", got)
	}
	if got := w.Next.Env.Env0(0, 0); got != 99 {
		t.Fatalf("cell spirit = %v, want only remaining capacity consumed", got)
	}
}

func TestFullQiCultivatorDoesNotDrainCellSpirit(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 100)
	w.Next.Env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 200
	attrs.Num["qi_max"] = 200
	attrs.Num["cultivation_speed"] = 1
	attrs.Num["moved_this_tick"] = 0
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&CultivationSystem{}).Tick(w)

	if got := w.Next.Agents.Attrs[0].Num["qi"]; got != 200 {
		t.Fatalf("cultivator qi = %v, want 200", got)
	}
	if got := w.Next.Env.Env0(0, 0); got != 100 {
		t.Fatalf("cell spirit = %v, want unchanged for full cultivator", got)
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
	scnCfg := DefaultScenarioConfig()
	want := before + (123-200*scnCfg.CultivatorUpkeepQiFrac)*(1-scnCfg.DeathQiLossFrac)
	if got := w.Next.Env.Env0(1, 1); math.Abs(got-want) > 1e-12 {
		t.Fatalf("cell spirit = %v, want %v", got, want)
	}
}

func TestLivingCultivatorConsumesUpkeepQi(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(1, 1, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 2
	attrs.Num["qi"] = 100
	attrs.Num["qi_max"] = 600
	attrs.Num["age"] = 30
	w.Next.Agents.Add("cultivator", 1, 1, attrs)

	beforeSpirit := w.Next.Env.Env0(1, 1)
	(&LifecycleSystem{}).Tick(w)

	if !w.Next.Agents.Alive[0] {
		t.Fatal("cultivator died unexpectedly")
	}
	wantQi := 100 - 600*DefaultScenarioConfig().CultivatorUpkeepQiFrac
	if got := w.Next.Agents.Attrs[0].Num["qi"]; math.Abs(got-wantQi) > 1e-12 {
		t.Fatalf("qi = %v, want %v", got, wantQi)
	}
	if got := w.Next.Env.Env0(1, 1); got != beforeSpirit {
		t.Fatalf("cell spirit = %v, want unchanged %v", got, beforeSpirit)
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
	attrs.Num["qi"] = 20
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
	want := 0.0
	if got := w.Next.Env.Env0(0, 0); got < want {
		t.Fatalf("cell spirit after low-spirit death = %v, want at least %v", got, want)
	}
}

func TestLowSpiritExposureRequiresLowQi(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 0)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 80
	attrs.Num["qi_max"] = 200
	attrs.Num["age"] = 30
	attrs.Num["low_spirit_years"] = 13
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&LifecycleSystem{}).Tick(w)

	if !w.Next.Agents.Alive[0] {
		t.Fatal("cultivator died with qi above low-spirit death threshold")
	}
	if got := w.Next.Agents.Attrs[0].Num["low_spirit_years"]; got != 0 {
		t.Fatalf("low_spirit_years = %v, want reset to 0 when qi is above threshold", got)
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
