package engine

type EngineConfig struct {
	GridWidth    int
	GridHeight   int
	Seed         uint64
	TicksPerYear int // how many ticks = 1 simulated year
	NumWorkers   int // 0 = auto-detect, 1 = serial
}

func DefaultEngineConfig() EngineConfig {
	return EngineConfig{
		GridWidth:    1000,
		GridHeight:   1000,
		Seed:         42,
		TicksPerYear: 10,
		NumWorkers:   8,
	}
}
