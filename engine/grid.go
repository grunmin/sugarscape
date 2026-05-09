package engine

// Grid provides spatial partitioning and environment storage.
// Agent indices are rebuilt each tick; environment data persists across ticks.
type Grid struct {
	Width, Height int
	Cells         [][]Cell
}

type Cell struct {
	Agents []int
	Env    map[string]float64
}

func NewGrid(w, h int) *Grid {
	cells := make([][]Cell, h)
	for i := range cells {
		cells[i] = make([]Cell, w)
	}
	return &Grid{Width: w, Height: h, Cells: cells}
}

// Rebuild clears agent lists and re-indexes all living agents.
func (g *Grid) Rebuild(store *AgentStore) {
	for r := range g.Cells {
		for c := range g.Cells[r] {
			g.Cells[r][c].Agents = g.Cells[r][c].Agents[:0]
		}
	}
	for i := range store.ID {
		if !store.Alive[i] {
			continue
		}
		x, y := store.X[i], store.Y[i]
		if x >= 0 && x < g.Width && y >= 0 && y < g.Height {
			g.Cells[y][x].Agents = append(g.Cells[y][x].Agents, i)
		}
	}
}

// GetNeighbors returns all agent indices in the 3x3 neighborhood (toroidal wrap).
func (g *Grid) GetNeighbors(x, y int) []int {
	var out []int
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx := (x + dx + g.Width) % g.Width
			ny := (y + dy + g.Height) % g.Height
			out = append(out, g.Cells[ny][nx].Agents...)
		}
	}
	return out
}

// GetEnv returns an environmental value at (x,y). Returns 0 if unset.
func (g *Grid) GetEnv(x, y int, key string) float64 {
	if x < 0 || y < 0 || x >= g.Width || y >= g.Height {
		return 0
	}
	return g.Cells[y][x].Env[key]
}

// SetEnv sets an environmental value at (x,y).
func (g *Grid) SetEnv(x, y int, key string, val float64) {
	if x < 0 || y < 0 || x >= g.Width || y >= g.Height {
		return
	}
	cell := &g.Cells[y][x]
	if cell.Env == nil {
		cell.Env = make(map[string]float64)
	}
	cell.Env[key] = val
}

// CloneEnv creates a deep copy of environment data only (not agent indices).
func (g *Grid) CloneEnv() *Grid {
	ng := NewGrid(g.Width, g.Height)
	for r := range g.Cells {
		for c := range g.Cells[r] {
			if g.Cells[r][c].Env != nil {
				ng.Cells[r][c].Env = make(map[string]float64, len(g.Cells[r][c].Env))
				for k, v := range g.Cells[r][c].Env {
					ng.Cells[r][c].Env[k] = v
				}
			}
		}
	}
	return ng
}
