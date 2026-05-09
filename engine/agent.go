package engine

import "math"

// AttrBag holds arbitrary typed attributes for an agent.
// Num is for numeric stats (qi, combat_power, etc.).
// Str is for string tags (sect, strategy, etc.).
type AttrBag struct {
	Num map[string]float64
	Str map[string]string
}

func NewAttrBag() AttrBag {
	return AttrBag{
		Num: make(map[string]float64),
		Str: make(map[string]string),
	}
}

func (a AttrBag) Clone() AttrBag {
	b := AttrBag{
		Num: make(map[string]float64, len(a.Num)),
		Str: make(map[string]string, len(a.Str)),
	}
	for k, v := range a.Num {
		b.Num[k] = v
	}
	for k, v := range a.Str {
		b.Str[k] = v
	}
	return b
}

// AgentStore is a struct-of-arrays for agent data.
type AgentStore struct {
	ID    []int
	Kind  []string
	X, Y  []int
	Alive []bool
	Attrs []AttrBag

	freeIDs []int
	nextID  int
}

func NewAgentStore(capacity int) *AgentStore {
	return &AgentStore{
		ID:      make([]int, 0, capacity),
		Kind:    make([]string, 0, capacity),
		X:       make([]int, 0, capacity),
		Y:       make([]int, 0, capacity),
		Alive:   make([]bool, 0, capacity),
		Attrs:   make([]AttrBag, 0, capacity),
		freeIDs: nil,
		nextID:  1,
	}
}

// Add inserts a new agent and returns its slice index.
func (as *AgentStore) Add(kind string, x, y int, attrs AttrBag) int {
	var idx int
	if len(as.freeIDs) > 0 {
		last := len(as.freeIDs) - 1
		idx = as.freeIDs[last]
		as.freeIDs = as.freeIDs[:last]
		as.ID[idx] = as.nextID
		as.Kind[idx] = kind
		as.X[idx] = x
		as.Y[idx] = y
		as.Alive[idx] = true
		as.Attrs[idx] = attrs
	} else {
		idx = len(as.ID)
		as.ID = append(as.ID, as.nextID)
		as.Kind = append(as.Kind, kind)
		as.X = append(as.X, x)
		as.Y = append(as.Y, y)
		as.Alive = append(as.Alive, true)
		as.Attrs = append(as.Attrs, attrs)
	}
	as.nextID++
	return idx
}

// Kill marks an agent as dead and recycles its slot.
func (as *AgentStore) Kill(idx int) {
	if !as.Alive[idx] {
		return
	}
	as.Alive[idx] = false
	as.freeIDs = append(as.freeIDs, idx)
}

// Count returns the number of living agents.
func (as *AgentStore) Count() int {
	n := 0
	for _, a := range as.Alive {
		if a {
			n++
		}
	}
	return n
}

// CountKind returns the number of living agents of a given kind.
func (as *AgentStore) CountKind(kind string) int {
	n := 0
	for i := range as.Alive {
		if as.Alive[i] && as.Kind[i] == kind {
			n++
		}
	}
	return n
}

// Clone creates a deep copy.
func (as *AgentStore) Clone() *AgentStore {
	c := &AgentStore{
		ID:      make([]int, len(as.ID)),
		Kind:    make([]string, len(as.Kind)),
		X:       make([]int, len(as.X)),
		Y:       make([]int, len(as.Y)),
		Alive:   make([]bool, len(as.Alive)),
		Attrs:   make([]AttrBag, len(as.Attrs)),
		freeIDs: make([]int, len(as.freeIDs)),
		nextID:  as.nextID,
	}
	copy(c.ID, as.ID)
	copy(c.Kind, as.Kind)
	copy(c.X, as.X)
	copy(c.Y, as.Y)
	copy(c.Alive, as.Alive)
	copy(c.freeIDs, as.freeIDs)
	for i := range as.Attrs {
		c.Attrs[i] = as.Attrs[i].Clone()
	}
	return c
}

// Clamp clamps a float64 to [lo, hi].
func Clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// Wrap wraps x to [0, n) like a torus.
func Wrap(x, n int) int {
	return ((x % n) + n) % n
}
