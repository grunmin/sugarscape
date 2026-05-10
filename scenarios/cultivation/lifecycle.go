package cultivation

import (
	"sync"

	"github.com/runmin/sugarscape/engine"
)

// LifecycleSystem handles aging, natural death, and births.
type LifecycleSystem struct{}

func (s *LifecycleSystem) Name() string  { return "LifecycleSystem" }
func (s *LifecycleSystem) Priority() int { return 7 }

func (s *LifecycleSystem) Tick(w *engine.World) {
	agents := w.Next.Agents
	env := w.Next.Env
	ticksPerYear := float64(w.Config.TicksPerYear)

	type deathReq struct {
		idx  int
		x, y int
		qi   float64
	}

	var mu sync.Mutex
	var allDeaths []deathReq

	engine.ParaForRNG(len(agents.ID), func(start, end, workerIdx int) {
		rng := engine.WorkerRNG(workerIdx)
		var localDeaths []deathReq
		for i := start; i < end; i++ {
			if !agents.Alive[i] {
				continue
			}
			kind := agents.Kind[i]
			attrs := &agents.Attrs[i]

			attrs.Num["age"] += 1.0 / ticksPerYear

			switch kind {
			case "cultivator":
				realm := int(attrs.Num["realm"])
				if realm < 1 {
					realm = 1
				}
				rc := GetRealm(realm)
				lifespan := rc.Lifespan

				x, y := agents.X[i], agents.Y[i]
				if env.Env0(x, y) < attrs.Num["qi_max"]*0.01 {
					attrs.Num["low_spirit_years"] += 1.0 / ticksPerYear
				} else {
					attrs.Num["low_spirit_years"] = 0
				}

				lowSpiritDeath := attrs.Num["low_spirit_years"] > lifespan*0.1 && rng.Float64() < 0.3
				if attrs.Num["age"] >= lifespan || lowSpiritDeath {
					localDeaths = append(localDeaths, deathReq{
						idx: i,
						x:   x,
						y:   y,
						qi:  attrs.Num["qi"],
					})
					continue
				}
			}
		}
		if len(localDeaths) > 0 {
			mu.Lock()
			allDeaths = append(allDeaths, localDeaths...)
			mu.Unlock()
		}
	})

	for _, d := range allDeaths {
		if agents.Alive[d.idx] {
			addSpirit(w.Next.Env, d.x, d.y, d.qi)
			agents.Kill(d.idx)
			w.Stats.RecordDeath()
		}
	}
}
