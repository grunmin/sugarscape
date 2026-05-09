package engine

type EngineConfig struct {
	GridWidth    int
	GridHeight   int
	Seed         uint64
	TicksPerYear int // how many ticks = 1 simulated year
}

func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		GridWidth:    100,
		GridHeight:   100,
		Seed:         42,
		TicksPerYear: 10,
	}
}
