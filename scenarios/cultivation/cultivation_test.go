package cultivation

import (
	"math"
	"testing"

	"github.com/runmin/sugarscape/engine"
)

func registerTestSect(name string, trait SectTrait, site SectSite) {
	resetSectState()
	sectMu.Lock()
	defer sectMu.Unlock()
	sectNames = append(sectNames, name)
	sectTraits = append(sectTraits, trait)
	if site.Name == "" {
		site.Name = name
	}
	if site.Style == "" {
		site.Style = trait.Style
	}
	sectSites = append(sectSites, site)
}

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

func TestMortalDensityLeavesWildernessEmpty(t *testing.T) {
	cfg := DefaultScenarioConfig()

	if got := mortalDensityMultiplier(0, cfg); got != cfg.MortalCoreDensityMultiplier {
		t.Fatalf("core mortal density multiplier = %v, want %v", got, cfg.MortalCoreDensityMultiplier)
	}
	innerDistSq := float64(cfg.MortalCoreRadius*cfg.MortalCoreRadius + 1)
	if got := mortalDensityMultiplier(innerDistSq, cfg); got != cfg.MortalInnerDensityMultiplier {
		t.Fatalf("inner mortal density multiplier = %v, want %v", got, cfg.MortalInnerDensityMultiplier)
	}
	outerDistSq := float64(cfg.MortalInnerRadius*cfg.MortalInnerRadius + 1)
	if got := mortalDensityMultiplier(outerDistSq, cfg); got != cfg.MortalOuterDensityMultiplier {
		t.Fatalf("outer mortal density multiplier = %v, want %v", got, cfg.MortalOuterDensityMultiplier)
	}
	wildernessDistSq := float64(cfg.MortalOuterRadius*cfg.MortalOuterRadius + 1)
	if got := mortalDensityMultiplier(wildernessDistSq, cfg); got != 0 {
		t.Fatalf("wilderness mortal density multiplier = %v, want 0", got)
	}
}

func TestSetupCreatesLayeredHighSpiritRegions(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 80
	cfg.GridHeight = 80
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	Setup(w)

	scnCfg := DefaultScenarioConfig()
	highMaxCells := 0
	veryHighSpiritCells := 0
	for _, cell := range w.Curr.Env.Cells {
		if cell.Env1 > scnCfg.SpiritMax+scnCfg.SpiritSpringMaxBonus {
			highMaxCells++
		}
		if cell.Env0 > scnCfg.SpiritMax {
			veryHighSpiritCells++
		}
	}
	if highMaxCells == 0 {
		t.Fatal("no high-capacity spirit cells created, want layered veins or blessed lands")
	}
	if veryHighSpiritCells == 0 {
		t.Fatal("no cells above default spirit max, want high-tier spirit regions")
	}
}

func TestBreakthroughUsesNewRealmQiMax(t *testing.T) {
	oldBreakthrough := DefaultRealms[0].BreakthroughBase
	DefaultRealms[0].BreakthroughBase = 2.0
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

func TestSpiritGradesScaleCultivationAndBreakthroughByCellSpirit(t *testing.T) {
	cfg := DefaultScenarioConfig()
	cases := []struct {
		spirit           float64
		name             string
		cultivationMult  float64
		breakthroughMult float64
	}{
		{spirit: 10, name: "下品", cultivationMult: 0.80, breakthroughMult: 0.85},
		{spirit: 50, name: "中品", cultivationMult: 0.92, breakthroughMult: 0.95},
		{spirit: 100, name: "上品", cultivationMult: 1.00, breakthroughMult: 1.00},
		{spirit: 150, name: "极品", cultivationMult: 1.15, breakthroughMult: 1.10},
		{spirit: 250, name: "天品", cultivationMult: 1.35, breakthroughMult: 1.25},
	}

	for _, tc := range cases {
		grade := spiritGradeForSpirit(tc.spirit, cfg)
		if grade.Name != tc.name {
			t.Fatalf("spirit %.0f grade = %s, want %s", tc.spirit, grade.Name, tc.name)
		}
		if grade.CultSpeedMultiplier != tc.cultivationMult {
			t.Fatalf("spirit %.0f cultivation multiplier = %v, want %v", tc.spirit, grade.CultSpeedMultiplier, tc.cultivationMult)
		}
		if grade.BreakthroughMultiplier != tc.breakthroughMult {
			t.Fatalf("spirit %.0f breakthrough multiplier = %v, want %v", tc.spirit, grade.BreakthroughMultiplier, tc.breakthroughMult)
		}
	}
}

func TestCultivationAbsorptionUsesSpiritGradeMultiplier(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 40)
	w.Next.Env.SetEnv1(0, 0, 100)
	w.Next.Env.SetEnv2(0, 0, DefaultScenarioConfig().SpiritRegenRate)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 0
	attrs.Num["cultivation_speed"] = 1
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&CultivationSystem{}).Tick(w)

	scnCfg := DefaultScenarioConfig()
	wantAbsorb := 40 * scnCfg.CultivationSpeed * DefaultRealms[0].CultSpeedMult * 0.80
	gotQi := w.Next.Agents.Attrs[0].Num["qi"]
	if math.Abs(gotQi-wantAbsorb) > 1e-12 {
		t.Fatalf("absorbed qi = %v, want %v", gotQi, wantAbsorb)
	}
	if gotSpirit := w.Next.Env.Env0(0, 0); math.Abs(gotSpirit-(40-wantAbsorb)) > 1e-12 {
		t.Fatalf("remaining cell spirit = %v, want %v", gotSpirit, 40-wantAbsorb)
	}
}

func TestBreakthroughProbabilityUsesSpiritGradeMultiplier(t *testing.T) {
	cfg := DefaultScenarioConfig()
	rc := DefaultRealms[0]
	attrs := engine.NewAttrBag()

	low := breakthroughProbabilityWithSpiritGrade(rc, attrs, cfg, 0, spiritGradeBreakthroughMultiplier(10, cfg))
	ordinary := breakthroughProbabilityWithSpiritGrade(rc, attrs, cfg, 0, spiritGradeBreakthroughMultiplier(100, cfg))
	high := breakthroughProbabilityWithSpiritGrade(rc, attrs, cfg, 0, spiritGradeBreakthroughMultiplier(150, cfg))

	wantOrdinary := rc.BreakthroughBase * cfg.LooseBreakthroughMultiplier
	if math.Abs(ordinary-wantOrdinary) > 1e-12 {
		t.Fatalf("ordinary spirit breakthrough probability = %v, want %v", ordinary, wantOrdinary)
	}
	if !(low < ordinary && high > ordinary) {
		t.Fatalf("quality-scaled breakthrough probabilities low/ordinary/high = %v/%v/%v, want ordered", low, ordinary, high)
	}
}

func TestSectBreakthroughProbabilityBonus(t *testing.T) {
	cfg := DefaultScenarioConfig()
	rc := DefaultRealms[0]
	trait := SectTrait{Style: "灵脉", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1.2}
	registerTestSect("灵脉宗1", trait, SectSite{Name: "灵脉宗1", Style: "灵脉", X: 5, Y: 5, Radius: 36})
	defer resetSectState()

	loose := engine.NewAttrBag()
	sect := engine.NewAttrBag()
	sect.Str["sect"] = "灵脉宗1"

	wantLoose := rc.BreakthroughBase * cfg.LooseBreakthroughMultiplier
	if got := breakthroughProbability(rc, loose, cfg, 10); got != wantLoose {
		t.Fatalf("loose breakthrough probability = %v, want %v", got, wantLoose)
	}
	want := rc.BreakthroughBase * (1 + cfg.SectBreakthroughBonus) * trait.BreakthroughMultiplier
	if got := breakthroughProbability(rc, sect, cfg, 0); got != want {
		t.Fatalf("sect breakthrough probability = %v, want %v", got, want)
	}
}

func TestSectMentorsIncreaseBreakthroughProbability(t *testing.T) {
	cfg := DefaultScenarioConfig()
	rc := DefaultRealms[0]
	registerTestSect("开山宗1", SectTrait{Style: "开山", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1}, SectSite{})
	defer resetSectState()

	sect := engine.NewAttrBag()
	sect.Str["sect"] = "开山宗1"

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
	DefaultRealms[3].BreakthroughBase = 2.0
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
	DefaultRealms[2].BreakthroughBase = 2.0
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
	resetSectState()
	defer resetSectState()

	cfg := DefaultScenarioConfig()
	if cfg.LooseBreakthroughMultiplier != 0.65 {
		t.Fatalf("loose breakthrough multiplier = %v, want 0.65", cfg.LooseBreakthroughMultiplier)
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
	if cfg.SectFormationCheckEvery != 20 {
		t.Fatalf("sect formation check interval = %d, want 20", cfg.SectFormationCheckEvery)
	}
	if cfg.SectFormationRadius != 16 {
		t.Fatalf("sect formation radius = %d, want 16", cfg.SectFormationRadius)
	}
	if cfg.SectFormationInfluenceRadius != 32 {
		t.Fatalf("sect formation influence radius = %d, want 32", cfg.SectFormationInfluenceRadius)
	}
	if cfg.SectFormationMinCultivators != 60 {
		t.Fatalf("sect formation min cultivators = %d, want 60", cfg.SectFormationMinCultivators)
	}
	if cfg.SectFormationMinSustainTicks != 160 {
		t.Fatalf("sect formation sustain ticks = %d, want 160", cfg.SectFormationMinSustainTicks)
	}
	if cfg.SectFormationMinCombatDeaths != 6 {
		t.Fatalf("sect formation combat deaths = %d, want 6", cfg.SectFormationMinCombatDeaths)
	}
	if cfg.SectFormationMinJindan != 4 {
		t.Fatalf("sect formation jindan leaders = %d, want 4", cfg.SectFormationMinJindan)
	}
	if cfg.SectFormationMinYuanying != 1 {
		t.Fatalf("sect formation yuanying leaders = %d, want 1", cfg.SectFormationMinYuanying)
	}
	if cfg.SectFormationMinSpiritMaxBonus != 70 {
		t.Fatalf("sect formation spirit max bonus = %v, want 70", cfg.SectFormationMinSpiritMaxBonus)
	}
	if cfg.SectFormationMinRegenBonus != 0.07 {
		t.Fatalf("sect formation regen bonus = %v, want 0.07", cfg.SectFormationMinRegenBonus)
	}
	if cfg.SectFormationExistingSectExclusion != 120 {
		t.Fatalf("sect formation exclusion radius = %d, want 120", cfg.SectFormationExistingSectExclusion)
	}
	if cfg.SectExpansionCheckEvery != 40 {
		t.Fatalf("sect expansion interval = %d, want 40", cfg.SectExpansionCheckEvery)
	}
	if cfg.SectExpansionSearchRadius != 96 {
		t.Fatalf("sect expansion search radius = %d, want 96", cfg.SectExpansionSearchRadius)
	}
	if cfg.SectExpansionInfluenceStep != 8 {
		t.Fatalf("sect expansion influence step = %d, want 8", cfg.SectExpansionInfluenceStep)
	}
	if cfg.SectExpansionMinMembers != 90 {
		t.Fatalf("sect expansion min members = %d, want 90", cfg.SectExpansionMinMembers)
	}
	if cfg.SectExpansionMaxSites != 4 {
		t.Fatalf("sect expansion max sites = %d, want 4", cfg.SectExpansionMaxSites)
	}
	if cfg.SectExpansionValuePerCombatPower != 0.05 {
		t.Fatalf("sect expansion combat value = %v, want 0.05", cfg.SectExpansionValuePerCombatPower)
	}
	if cfg.SectExpansionOverextensionCost != 0.35 {
		t.Fatalf("sect overextension cost = %v, want 0.35", cfg.SectExpansionOverextensionCost)
	}
	if cfg.SectAggressiveExpansionStallTicks != 400 {
		t.Fatalf("sect aggressive stall ticks = %d, want 400", cfg.SectAggressiveExpansionStallTicks)
	}
	if cfg.SectAggressiveMinDispatchFrac != 0.34 {
		t.Fatalf("sect min dispatch fraction = %v, want 0.34", cfg.SectAggressiveMinDispatchFrac)
	}
	if cfg.SectAggressiveMaxDispatchFrac != 0.67 {
		t.Fatalf("sect max dispatch fraction = %v, want 0.67", cfg.SectAggressiveMaxDispatchFrac)
	}
	if len(SectNames()) != 0 {
		t.Fatalf("initial sect count = %d, want 0", len(SectNames()))
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

func TestSectSystemFoundsSectFromHighSpiritCluster(t *testing.T) {
	resetSectState()
	defer resetSectState()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 100
	cfg.GridHeight = 100
	cfg.NumWorkers = 1
	w := engine.NewWorld(cfg)
	scnCfg := DefaultScenarioConfig()

	x, y := 10, 10
	w.Next.Env.SetEnv1(x, y, scnCfg.SpiritMax+scnCfg.BlessedLandMaxBonus)
	w.Next.Env.SetEnv2(x, y, scnCfg.SpiritRegenRate+scnCfg.BlessedLandRegenBonus)
	for i := 0; i < scnCfg.SectFormationMinCultivators; i++ {
		attrs := engine.NewAttrBag()
		if i < scnCfg.SectFormationMinJindan {
			attrs.Num["realm"] = 3
		} else {
			attrs.Num["realm"] = 1
		}
		attrs.Num["combat_power"] = 1
		w.Next.Agents.Add("cultivator", x, y, attrs)
	}
	for i := 0; i < scnCfg.SectFormationMinCombatDeaths; i++ {
		recordSectCandidateDeath(x, y)
	}

	system := &SectSystem{}
	for tick := int64(scnCfg.SectFormationCheckEvery); tick <= int64(scnCfg.SectFormationMinSustainTicks); tick += int64(scnCfg.SectFormationCheckEvery) {
		w.Clock.Tick = tick
		system.Tick(w)
	}

	names := SectNames()
	if len(names) != 1 {
		t.Fatalf("sect count = %d, want 1", len(names))
	}
	if names[0] != "灵脉宗1" {
		t.Fatalf("sect name = %q, want 灵脉宗1", names[0])
	}
	sites := SectSites()
	if len(sites) != 1 || sites[0].X != x || sites[0].Y != y || sites[0].Deaths != scnCfg.SectFormationMinCombatDeaths {
		t.Fatalf("sect sites = %+v, want one site at death cluster", sites)
	}
	for i := 0; i < scnCfg.SectFormationMinCultivators; i++ {
		if got := w.Next.Agents.Attrs[i].Str["sect"]; got != names[0] {
			t.Fatalf("agent %d sect = %q, want %q", i, got, names[0])
		}
	}

	spawnCultivator(w, x+1, y)
	newcomer := w.Next.Agents.Attrs[len(w.Next.Agents.Attrs)-1]
	if got := newcomer.Str["sect"]; got != names[0] {
		t.Fatalf("new nearby cultivator sect = %q, want %q", got, names[0])
	}
}

func TestSectSystemRequiresHighRealmFounders(t *testing.T) {
	resetSectState()
	defer resetSectState()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 100
	cfg.GridHeight = 100
	cfg.NumWorkers = 1
	w := engine.NewWorld(cfg)
	scnCfg := DefaultScenarioConfig()

	x, y := 10, 10
	w.Next.Env.SetEnv1(x, y, scnCfg.SpiritMax+scnCfg.BlessedLandMaxBonus)
	w.Next.Env.SetEnv2(x, y, scnCfg.SpiritRegenRate+scnCfg.BlessedLandRegenBonus)
	for i := 0; i < scnCfg.SectFormationMinCultivators; i++ {
		attrs := engine.NewAttrBag()
		attrs.Num["realm"] = 1
		attrs.Num["combat_power"] = 1
		w.Next.Agents.Add("cultivator", x, y, attrs)
	}
	for i := 0; i < scnCfg.SectFormationMinCombatDeaths; i++ {
		recordSectCandidateDeath(x, y)
	}

	system := &SectSystem{}
	for tick := int64(scnCfg.SectFormationCheckEvery); tick <= int64(scnCfg.SectFormationMinSustainTicks); tick += int64(scnCfg.SectFormationCheckEvery) {
		w.Clock.Tick = tick
		system.Tick(w)
	}

	if got := len(SectNames()); got != 0 {
		t.Fatalf("sect count without high-realm founders = %d, want 0", got)
	}

	w.Next.Agents.Attrs[0].Num["realm"] = 4
	w.Clock.Tick += int64(scnCfg.SectFormationCheckEvery)
	system.Tick(w)

	if got := len(SectNames()); got != 1 {
		t.Fatalf("sect count after yuanying founder = %d, want 1", got)
	}
}

func TestSectExpansionClaimsProfitableHighSpiritSite(t *testing.T) {
	resetSectState()
	defer resetSectState()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 180
	cfg.GridHeight = 180
	cfg.NumWorkers = 1
	w := engine.NewWorld(cfg)
	scnCfg := DefaultScenarioConfig()

	name := "灵脉宗1"
	trait := SectTrait{Style: "灵脉", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1.2}
	registerTestSect(name, trait, SectSite{Name: name, Style: "灵脉", Kind: "立宗", X: 10, Y: 10, Radius: scnCfg.SectFormationInfluenceRadius})

	for i := 0; i < scnCfg.SectExpansionMinMembers; i++ {
		attrs := engine.NewAttrBag()
		attrs.Str["sect"] = name
		attrs.Num["realm"] = 1
		attrs.Num["qi"] = 100
		attrs.Num["combat_power"] = 1
		w.Next.Agents.Add("cultivator", 10, 10, attrs)
	}
	loose := engine.NewAttrBag()
	loose.Num["realm"] = 1
	w.Next.Agents.Add("cultivator", 90, 10, loose)

	w.Next.Env.SetEnv1(90, 10, scnCfg.SpiritMax+scnCfg.BlessedLandMaxBonus)
	w.Next.Env.SetEnv2(90, 10, scnCfg.SpiritRegenRate+scnCfg.BlessedLandRegenBonus)

	w.Clock.Tick = int64(scnCfg.SectExpansionCheckEvery)
	(&SectSystem{}).Tick(w)

	sites := SectSites()
	if len(sites) != 2 {
		t.Fatalf("sect site count = %d, want 2", len(sites))
	}
	expansion := sites[1]
	if expansion.Kind != "扩张" || expansion.Name != name || expansion.X != 90 || expansion.Y != 10 {
		t.Fatalf("expansion site = %+v, want expansion at high-spirit target", expansion)
	}
	if expansion.NetBenefit <= 0 {
		t.Fatalf("expansion net benefit = %v, want profitable", expansion.NetBenefit)
	}
	if got := w.Next.Agents.Attrs[len(w.Next.Agents.Attrs)-1].Str["sect"]; got != name {
		t.Fatalf("loose cultivator sect after expansion = %q, want %q", got, name)
	}
	if got := w.Next.Agents.Attrs[0].Num["qi"]; got >= 100 {
		t.Fatalf("member qi after expansion = %v, want upkeep cost applied", got)
	}
}

func TestSectInfluenceRadiusGrowsWithCombatPower(t *testing.T) {
	resetSectState()
	defer resetSectState()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 120
	cfg.GridHeight = 120
	cfg.NumWorkers = 1
	w := engine.NewWorld(cfg)
	scnCfg := DefaultScenarioConfig()

	name := "开山宗1"
	trait := SectTrait{Style: "开山", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1}
	registerTestSect(name, trait, SectSite{Name: name, Style: "开山", Kind: "立宗", X: 20, Y: 20, Radius: scnCfg.SectFormationInfluenceRadius})

	for i := 0; i < scnCfg.SectExpansionMinMembers; i++ {
		attrs := engine.NewAttrBag()
		attrs.Str["sect"] = name
		attrs.Num["realm"] = 1
		attrs.Num["qi"] = 100
		attrs.Num["combat_power"] = 200
		w.Next.Agents.Add("cultivator", 20, 20, attrs)
	}

	w.Clock.Tick = int64(scnCfg.SectExpansionCheckEvery)
	(&SectSystem{}).Tick(w)

	sites := SectSites()
	wantRadius := scnCfg.SectFormationInfluenceRadius + scnCfg.SectExpansionInfluenceStep
	if len(sites) != 1 || sites[0].Radius != wantRadius {
		t.Fatalf("sect sites = %+v, want one grown home radius %d", sites, wantRadius)
	}
	if got := w.Next.Agents.Attrs[0].Num["qi"]; got >= 100 {
		t.Fatalf("member qi after influence growth = %v, want expansion cost applied", got)
	}
}

func TestSectSiteOverlapUsesCombinedRadii(t *testing.T) {
	sites := []SectSite{{Name: "宗门1", X: 10, Y: 10, Radius: 40}}
	if !hasAnySiteOverlap(60, 10, 28, 200, 200, sites) {
		t.Fatal("overlap check = false, want true when circles overlap")
	}
	if hasAnySiteOverlap(90, 10, 28, 200, 200, sites) {
		t.Fatal("overlap check = true, want false for separated territories")
	}
}

func TestStalledSectAggressivelyOccupiesUnclaimedHighSpiritSite(t *testing.T) {
	resetSectState()
	defer resetSectState()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 240
	cfg.GridHeight = 240
	cfg.NumWorkers = 1
	w := engine.NewWorld(cfg)
	scnCfg := DefaultScenarioConfig()

	name := "灵脉宗1"
	trait := SectTrait{Style: "灵脉", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1.2}
	registerTestSect(name, trait, SectSite{Name: name, Style: "灵脉", Kind: "立宗", X: 16, Y: 16, Radius: scnCfg.SectFormationInfluenceRadius})

	for i := 0; i < scnCfg.SectExpansionMinMembers; i++ {
		attrs := engine.NewAttrBag()
		attrs.Str["sect"] = name
		attrs.Num["realm"] = 1
		attrs.Num["qi"] = 100
		attrs.Num["combat_power"] = 100
		w.Next.Agents.Add("cultivator", 16, 16, attrs)
	}

	w.Next.Env.SetEnv1(128, 16, scnCfg.SpiritMax+scnCfg.BlessedLandMaxBonus)
	w.Next.Env.SetEnv2(128, 16, scnCfg.SpiritRegenRate+scnCfg.BlessedLandRegenBonus)

	system := &SectSystem{stalled: map[string]int{name: scnCfg.SectAggressiveExpansionStallTicks - scnCfg.SectExpansionCheckEvery}}
	w.Clock.Tick = int64(scnCfg.SectExpansionCheckEvery)
	system.Tick(w)

	sites := SectSites()
	if len(sites) != 2 {
		t.Fatalf("sect site count = %d, want home plus occupied site", len(sites))
	}
	occupied := sites[1]
	if occupied.Kind != "占领" || occupied.Name != name || occupied.X != 128 || occupied.Y != 16 {
		t.Fatalf("occupied site = %+v, want aggressive occupation at high-spirit target", occupied)
	}

	dispatched := 0
	for i := 0; i < scnCfg.SectExpansionMinMembers; i++ {
		if w.Next.Agents.X[i] != 16 || w.Next.Agents.Y[i] != 16 {
			dispatched++
		}
	}
	minDispatch := int(math.Ceil(float64(scnCfg.SectExpansionMinMembers) * scnCfg.SectAggressiveMinDispatchFrac))
	maxDispatch := int(math.Floor(float64(scnCfg.SectExpansionMinMembers) * scnCfg.SectAggressiveMaxDispatchFrac))
	if dispatched < minDispatch || dispatched > maxDispatch {
		t.Fatalf("dispatched members = %d, want in [%d, %d]", dispatched, minDispatch, maxDispatch)
	}
}

func TestStalledSectCanConquerWeakerSectSite(t *testing.T) {
	resetSectState()
	defer resetSectState()

	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 180
	cfg.GridHeight = 180
	cfg.NumWorkers = 1
	w := engine.NewWorld(cfg)
	scnCfg := DefaultScenarioConfig()

	attacker := "战盟宗1"
	defender := "灵脉宗2"
	sectMu.Lock()
	sectNames = append(sectNames, attacker, defender)
	sectTraits = append(sectTraits,
		SectTrait{Style: "战盟", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1},
		SectTrait{Style: "灵脉", RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1},
	)
	sectSites = append(sectSites,
		SectSite{Name: attacker, Style: "战盟", Kind: "立宗", X: 16, Y: 16, Radius: scnCfg.SectFormationInfluenceRadius},
		SectSite{Name: defender, Style: "灵脉", Kind: "立宗", X: 100, Y: 16, Radius: scnCfg.SectFormationInfluenceRadius, Potential: 1},
	)
	sectMu.Unlock()

	for i := 0; i < scnCfg.SectExpansionMinMembers; i++ {
		attrs := engine.NewAttrBag()
		attrs.Str["sect"] = attacker
		attrs.Num["realm"] = 1
		attrs.Num["qi"] = 100
		attrs.Num["combat_power"] = 100
		w.Next.Agents.Add("cultivator", 16, 16, attrs)
	}
	for i := 0; i < 20; i++ {
		attrs := engine.NewAttrBag()
		attrs.Str["sect"] = defender
		attrs.Num["realm"] = 1
		attrs.Num["combat_power"] = 10
		w.Next.Agents.Add("cultivator", 100, 16, attrs)
	}

	system := &SectSystem{stalled: map[string]int{attacker: scnCfg.SectAggressiveExpansionStallTicks - scnCfg.SectExpansionCheckEvery}}
	w.Clock.Tick = int64(scnCfg.SectExpansionCheckEvery)
	system.Tick(w)

	sites := SectSites()
	if sites[1].Name != attacker || sites[1].Kind != "攻占" {
		t.Fatalf("defender site after conquest = %+v, want owned by attacker with 攻占 kind", sites[1])
	}
	dispatched := 0
	for i := 0; i < scnCfg.SectExpansionMinMembers; i++ {
		if w.Next.Agents.X[i] != 16 || w.Next.Agents.Y[i] != 16 {
			dispatched++
		}
	}
	maxDispatch := int(math.Floor(float64(scnCfg.SectExpansionMinMembers) * scnCfg.SectAggressiveMaxDispatchFrac))
	if dispatched != maxDispatch {
		t.Fatalf("conquest dispatched members = %d, want max dispatch %d", dispatched, maxDispatch)
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
	if got := conversionSpawnSpiritFactor(0, 100, cfg); got != cfg.ConversionSpawnSpiritFloor {
		t.Fatalf("zero-spirit spawn factor = %v, want floor %v", got, cfg.ConversionSpawnSpiritFloor)
	}
	if got := conversionSpawnSpiritFactor(100, 100, cfg); got != 1 {
		t.Fatalf("max-spirit spawn factor = %v, want 1", got)
	}
}

func TestMortalSpawnStronglyPrefersHighSpiritCells(t *testing.T) {
	cfg := DefaultScenarioConfig()
	env := engine.NewGrid(2, 1)
	env.SetMortal(0, 0, 100)
	env.SetMortal(1, 0, 100)
	env.SetEnv0(0, 0, 10)
	env.SetEnv0(1, 0, 100)
	maxPop := maxMortalPop(env)
	maxSpirit := maxCurrentSpirit(env)
	rng := engine.NewRNG(42)

	highSpiritSpawns := 0
	for range 200 {
		sr := sampleMortalSpawn(rng, env, maxPop, maxSpirit, cfg)
		if sr.x == 1 {
			highSpiritSpawns++
		}
	}

	if highSpiritSpawns < 180 {
		t.Fatalf("high-spirit spawn count = %d/200, want strong clustering", highSpiritSpawns)
	}
}

func TestMovementProbabilityScalesWithCellSpirit(t *testing.T) {
	env := engine.NewGrid(1, 1)
	env.SetEnv1(0, 0, 100)

	env.SetEnv0(0, 0, 100)
	if got := movementProbability(env, 0, 0); got != 0.05 {
		t.Fatalf("movement probability at full ordinary spirit = %v, want 0.05 exploration floor", got)
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

func TestHighPotentialCoreRetainsCultivatorsEvenWhenPartlyDrained(t *testing.T) {
	cfg := DefaultScenarioConfig()
	env := engine.NewGrid(1, 1)
	env.SetEnv0(0, 0, 20)
	env.SetEnv1(0, 0, cfg.SpiritMax+cfg.BlessedLandMaxBonus)
	env.SetEnv2(0, 0, cfg.SpiritRegenRate+cfg.BlessedLandRegenBonus)

	if got := movementProbability(env, 0, 0); got != 0 {
		t.Fatalf("movement probability in drained high-potential core = %v, want 0", got)
	}
}

func TestAdjacentResourceSearchPrefersHighPotentialCore(t *testing.T) {
	cfg := DefaultScenarioConfig()
	env := engine.NewGrid(2, 1)
	env.SetEnv0(0, 0, cfg.SpiritMax)
	env.SetEnv1(0, 0, cfg.SpiritMax)
	env.SetEnv2(0, 0, cfg.SpiritRegenRate)
	env.SetEnv0(1, 0, 20)
	env.SetEnv1(1, 0, cfg.SpiritMax+cfg.BlessedLandMaxBonus)
	env.SetEnv2(1, 0, cfg.SpiritRegenRate+cfg.BlessedLandRegenBonus)

	x, y, ok := bestAdjacentSpiritPosition(env, 0, 0, 2, 1)
	if !ok || x != 1 || y != 0 {
		t.Fatalf("best adjacent = (%d,%d,%v), want high-potential core at (1,0,true)", x, y, ok)
	}
}

func TestLowQiCultivatorMovesMoreInAdequateCell(t *testing.T) {
	env := engine.NewGrid(1, 1)
	env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["qi"] = 40
	attrs.Num["qi_max"] = 100

	// Adequate cell (spirit>=25%): low qi should INCREASE movement probability.
	// qiFrac=0.4, cellSpiritFrac=0.5, base=0.5, restlessMultiplier=1.5 → 0.75.
	env.SetEnv0(0, 0, 50)
	got := movementProbabilityForCultivator(env, 0, 0, attrs)
	want := 0.75
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("movement probability for low qi in adequate cell = %v, want %v", got, want)
	}

	// Poor cell (spirit<25%): no restless bonus, pure base probability.
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

func TestCultivationCreatesRumorFromObservedSpirit(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 1
	cfg.GridHeight = 1
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	w.Next.Env.SetEnv0(0, 0, 70)
	w.Next.Env.SetEnv1(0, 0, 100)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 0
	attrs.Num["qi_max"] = 200
	attrs.Num["cultivation_speed"] = 1
	attrs.Num["moved_this_tick"] = 0
	w.Next.Agents.Add("cultivator", 0, 0, attrs)

	(&CultivationSystem{}).Tick(w)

	got := w.Next.Agents.Attrs[0].Num[rumorKeyStrength]
	if math.Abs(got-0.7) > 1e-12 {
		t.Fatalf("rumor strength = %v, want 0.7 from pre-absorb spirit", got)
	}
}

func TestVerifyRumorOnlyClearsAtRumoredLocation(t *testing.T) {
	attrs := engine.NewAttrBag()
	createRumor(&attrs, 5, 5, 80, 100)

	verifyRumorAtLocation(&attrs, 1, 1, 20, 100)
	if got := attrs.Num[rumorKeyStrength]; got != 0.8 {
		t.Fatalf("rumor strength away from target = %v, want 0.8", got)
	}

	verifyRumorAtLocation(&attrs, 5, 5, 20, 100)
	if got := attrs.Num[rumorKeyStrength]; got != 0 {
		t.Fatalf("rumor strength at stale target = %v, want cleared", got)
	}
}

func TestShareRumorUsesRelationshipEfficiency(t *testing.T) {
	from := engine.NewAttrBag()
	createRumor(&from, 2, 3, 100, 100)

	stranger := engine.NewAttrBag()
	shareRumor(&from, &stranger, rumorRelationStranger)
	if got := stranger.Num[rumorKeyStrength]; got != 0.4 {
		t.Fatalf("stranger rumor strength = %v, want 0.4", got)
	}

	differentSect := engine.NewAttrBag()
	shareRumor(&from, &differentSect, rumorRelationDifferentSect)
	if got := differentSect.Num[rumorKeyStrength]; got != 0.6 {
		t.Fatalf("different-sect rumor strength = %v, want 0.6", got)
	}

	sameSect := engine.NewAttrBag()
	shareRumor(&from, &sameSect, rumorRelationSameSect)
	if got := sameSect.Num[rumorKeyStrength]; got != 0.9 {
		t.Fatalf("same-sect rumor strength = %v, want 0.9", got)
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
