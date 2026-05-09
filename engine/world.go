package engine

import "sort"

// Frame is a snapshot of world state at a point in time.
type Frame struct {
	Agents *AgentStore
	Env    *Grid
}

func (f *Frame) Clone() *Frame {
	return &Frame{
		Agents: f.Agents.Clone(),
		Env:    f.Env.CloneEnv(),
	}
}

// World is the top-level simulation container.
type World struct {
	Curr    *Frame
	Next    *Frame
	Grid    *Grid   // spatial index, rebuilt each tick from Next agent positions
	Clock   *Clock
	RNG     *RNG
	Config  EngineConfig
	Stats   *StatsCollector
	systems []System
}

func NewWorld(cfg EngineConfig) *World {
	store := NewAgentStore(20000)
	env := NewGrid(cfg.GridWidth, cfg.GridHeight)

	f := &Frame{Agents: store, Env: env}

	w := &World{
		Curr:   f,
		Next:   f.Clone(),
		Grid:   NewGrid(cfg.GridWidth, cfg.GridHeight),
		Clock:  NewClock(cfg.TicksPerYear),
		RNG:    NewRNG(cfg.Seed),
		Config: cfg,
		Stats:  NewStatsCollector(),
	}
	return w
}

// RegisterSystem adds a system to the simulation pipeline.
func (w *World) RegisterSystem(sys System) {
	w.systems = append(w.systems, sys)
	sort.Slice(w.systems, func(i, j int) bool {
		return w.systems[i].Priority() < w.systems[j].Priority()
	})
}

// Tick advances the simulation by one time step.
func (w *World) Tick() {
	w.Next = w.Curr.Clone()
	w.Grid.Rebuild(w.Next.Agents)

	for _, sys := range w.systems {
		sys.Tick(w)
	}

	w.Curr, w.Next = w.Next, w.Curr
	w.Clock.Advance()
}

// Run executes the simulation for a given number of ticks.
func (w *World) Run(ticks int64, snapshotEvery int) {
	for range ticks {
		w.Tick()
		if snapshotEvery > 0 && w.Clock.Tick%int64(snapshotEvery) == 0 {
			w.Stats.Snapshot(w.Curr, w.Next.Env, w.Clock.Tick, w.Clock.Year())
		}
	}
}
