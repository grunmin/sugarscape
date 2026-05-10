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
	ticksPerYear := float64(w.Config.TicksPerYear)

	type deathReq struct {
		idx  int
		x, y int
		qi   float64
	}

	var mu sync.Mutex
	var allDeaths []deathReq

	engine.ParaFor(len(agents.ID), func(start, end int) {
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

				if attrs.Num["age"] >= lifespan {
					localDeaths = append(localDeaths, deathReq{
						idx: i,
						x:   agents.X[i],
						y:   agents.Y[i],
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
