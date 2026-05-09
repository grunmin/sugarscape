package engine

// Grid provides spatial partitioning and environment storage.
// Uses a flat slice for O(1) access to 1M+ cells.
type Grid struct {
	Width, Height int
	Cells         []Cell // len = Width * Height, index = y*Width + x
}

// Cell holds agent indices and typed environmental data.
// Named fields replace map[string]float64 for performance at scale.
type Cell struct {
	Agents     []int
	Env0       float64 // spirit_density
	Env1       float64 // spirit_max
	Env2       float64 // regeneration_rate
	MortalPop  float64 // mortal population in this cell
}

func NewGrid(w, h int) *Grid {
	return &Grid{
		Width:  w,
		Height: h,
		Cells:  make([]Cell, w*h),
	}
}

// idx converts (x,y) to flat index with toroidal wrap.
func (g *Grid) idx(x, y int) int {
	x = (x + g.Width) % g.Width
	y = (y + g.Height) % g.Height
	return y*g.Width + x
}

// cell returns a pointer to the cell at (x,y).
func (g *Grid) cell(x, y int) *Cell {
	return &g.Cells[g.idx(x, y)]
}

// Env0 returns the first environment slot.
func (g *Grid) Env0(x, y int) float64 { return g.cell(x, y).Env0 }

// SetEnv0 sets the first environment slot.
func (g *Grid) SetEnv0(x, y int, v float64) { g.cell(x, y).Env0 = v }

// Env1 returns the second environment slot.
func (g *Grid) Env1(x, y int) float64 { return g.cell(x, y).Env1 }

// SetEnv1 sets the second environment slot.
func (g *Grid) SetEnv1(x, y int, v float64) { g.cell(x, y).Env1 = v }

// Env2 returns the third environment slot.
func (g *Grid) Env2(x, y int) float64 { return g.cell(x, y).Env2 }

// SetEnv2 sets the third environment slot.
func (g *Grid) SetEnv2(x, y int, v float64) { g.cell(x, y).Env2 = v }

// Mortal returns mortal population at (x,y).
func (g *Grid) Mortal(x, y int) float64 { return g.cell(x, y).MortalPop }

// SetMortal sets mortal population at (x,y).
func (g *Grid) SetMortal(x, y int, v float64) { g.cell(x, y).MortalPop = v }

// AddMortal adds delta to mortal population at (x,y). Returns the new value.
func (g *Grid) AddMortal(x, y int, delta float64) float64 {
	c := g.cell(x, y)
	c.MortalPop += delta
	if c.MortalPop < 0 {
		c.MortalPop = 0
	}
	return c.MortalPop
}

// Rebuild clears agent lists and re-indexes all living agents.
func (g *Grid) Rebuild(store *AgentStore) {
	for i := range g.Cells {
		g.Cells[i].Agents = g.Cells[i].Agents[:0]
	}
	for i := range store.ID {
		if !store.Alive[i] {
			continue
		}
		x, y := store.X[i], store.Y[i]
		if x >= 0 && x < g.Width && y >= 0 && y < g.Height {
			c := g.cell(x, y)
			c.Agents = append(c.Agents, i)
		}
	}
}

// GetNeighbors returns all agent indices within distance r (Manhattan-like, square neighborhood).
// r=0 returns same-cell agents, r=1 returns 3x3 neighborhood, etc.
func (g *Grid) GetNeighbors(x, y, r int) []int {
	var out []int
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			idx := g.idx(x+dx, y+dy)
			out = append(out, g.Cells[idx].Agents...)
		}
	}
	return out
}

// CloneEnv creates a deep copy of environment data (not agent indices).
func (g *Grid) CloneEnv() *Grid {
	ng := NewGrid(g.Width, g.Height)
	for i := range g.Cells {
		ng.Cells[i].Env0 = g.Cells[i].Env0
		ng.Cells[i].Env1 = g.Cells[i].Env1
		ng.Cells[i].Env2 = g.Cells[i].Env2
		ng.Cells[i].MortalPop = g.Cells[i].MortalPop
	}
	return ng
}

// TotalMortals sums mortal population across all cells.
func (g *Grid) TotalMortals() float64 {
	var sum float64
	for i := range g.Cells {
		sum += g.Cells[i].MortalPop
	}
	return sum
}
