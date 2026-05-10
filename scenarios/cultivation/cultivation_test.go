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

func TestStrongerSecondCultivatorAttacksOnFleeThreshold(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 3
	cfg.GridHeight = 3
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	weak := engine.NewAttrBag()
	weak.Num["realm"] = 1
	weak.Num["combat_power"] = 10
	strong := engine.NewAttrBag()
	strong.Num["realm"] = 1
	strong.Num["combat_power"] = 40

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
	if got := w.Next.Env.Env0(1, 1); got != before+123 {
		t.Fatalf("cell spirit = %v, want %v", got, before+123)
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

func TestMortalBirthRateRange(t *testing.T) {
	cfg := DefaultScenarioConfig()
	if cfg.MortalBirthRateMin != 0.9 {
		t.Fatalf("MortalBirthRateMin = %v, want 0.9", cfg.MortalBirthRateMin)
	}
	if cfg.MortalBirthRateMax != 1.2 {
		t.Fatalf("MortalBirthRateMax = %v, want 1.2", cfg.MortalBirthRateMax)
	}
}
