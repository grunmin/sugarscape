package engine

// Clock tracks simulation time.
type Clock struct {
	Tick         int64
	TicksPerYear int
}

func NewClock(ticksPerYear int) *Clock {
	return &Clock{Tick: 0, TicksPerYear: ticksPerYear}
}

func (c *Clock) Year() float64 {
	return float64(c.Tick) / float64(c.TicksPerYear)
}

func (c *Clock) Advance() {
	c.Tick++
}
