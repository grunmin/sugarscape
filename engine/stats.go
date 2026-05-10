package engine

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"sync"
)

// DataPoint holds aggregated stats at a point in time.
type DataPoint struct {
	Tick int64
	Year float64
	// Counts by kind
	KindCounts map[string]int
	// Counts by realm (realm name → count)
	RealmCounts map[string]int
	// Totals
	TotalAgents   int
	TotalMortals  float64
	AvgQi         float64
	AvgAge        float64
	AvgCP         float64 // combat power
	AvgAggression float64
	// Events this tick
	Deaths            int
	Births            int
	Breakthroughs     int
	MortalConversions int
}

type NotableEvent struct {
	Tick    int64
	Year    float64
	Kind    string
	Realm   string
	AgentID int
	X, Y    int
	Reason  string
}

type StatsCollector struct {
	mu             sync.Mutex
	Snapshots      []DataPoint
	TickDeaths     int
	TickBirths     int
	TickBreakthru  int
	TickMortalConv int
	NotableEvents  []NotableEvent
}

func NewStatsCollector() *StatsCollector {
	return &StatsCollector{}
}

func (sc *StatsCollector) RecordDeath() {
	sc.mu.Lock()
	sc.TickDeaths++
	sc.mu.Unlock()
}

func (sc *StatsCollector) RecordBirth() {
	sc.mu.Lock()
	sc.TickBirths++
	sc.mu.Unlock()
}

func (sc *StatsCollector) RecordBreakthrough() {
	sc.mu.Lock()
	sc.TickBreakthru++
	sc.mu.Unlock()
}

func (sc *StatsCollector) RecordMortalConversion() {
	sc.mu.Lock()
	sc.TickMortalConv++
	sc.mu.Unlock()
}

func (sc *StatsCollector) RecordNotableEvent(ev NotableEvent) {
	sc.mu.Lock()
	sc.NotableEvents = append(sc.NotableEvents, ev)
	sc.mu.Unlock()
}

func (sc *StatsCollector) DrainNotableEvents() []NotableEvent {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if len(sc.NotableEvents) == 0 {
		return nil
	}
	events := append([]NotableEvent(nil), sc.NotableEvents...)
	sc.NotableEvents = sc.NotableEvents[:0]
	return events
}

var realmNames = map[int]string{
	0: "凡人",
	1: "练气",
	2: "筑基",
	3: "金丹",
	4: "元婴",
	5: "化神",
}

// Snapshot captures the current world state and resets tick counters.
func (sc *StatsCollector) Snapshot(f *Frame, env *Grid, tick int64, year float64) {
	sc.mu.Lock()
	deaths := sc.TickDeaths
	births := sc.TickBirths
	breakthroughs := sc.TickBreakthru
	mortalConversions := sc.TickMortalConv
	sc.TickDeaths = 0
	sc.TickBirths = 0
	sc.TickBreakthru = 0
	sc.TickMortalConv = 0
	sc.mu.Unlock()

	dp := DataPoint{
		Tick:              tick,
		Year:              year,
		KindCounts:        make(map[string]int),
		RealmCounts:       make(map[string]int),
		TotalMortals:      env.TotalMortals(),
		Deaths:            deaths,
		Births:            births,
		Breakthroughs:     breakthroughs,
		MortalConversions: mortalConversions,
	}

	var qiSum, ageSum, cpSum, aggSum float64
	alive := 0
	for i := range f.Agents.ID {
		if !f.Agents.Alive[i] {
			continue
		}
		alive++
		kind := f.Agents.Kind[i]
		dp.KindCounts[kind]++

		if kind == "cultivator" {
			realm := int(f.Agents.Attrs[i].Num["realm"])
			realmName := realmNames[realm]
			dp.RealmCounts[realmName]++
			qiSum += f.Agents.Attrs[i].Num["qi"]
			ageSum += f.Agents.Attrs[i].Num["age"]
			cpSum += f.Agents.Attrs[i].Num["combat_power"]
			aggSum += f.Agents.Attrs[i].Num["aggression"]
		}
	}
	dp.TotalAgents = alive
	cultCount := dp.KindCounts["cultivator"]
	if cultCount > 0 {
		dp.AvgQi = qiSum / float64(cultCount)
		dp.AvgAge = ageSum / float64(cultCount)
		dp.AvgCP = cpSum / float64(cultCount)
		dp.AvgAggression = aggSum / float64(cultCount)
	}

	sc.Snapshots = append(sc.Snapshots, dp)
}

// ExportCSV writes all snapshots to a CSV file.
func (sc *StatsCollector) ExportCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	kindSet := make(map[string]struct{})
	realmSet := make(map[string]struct{})
	for _, dp := range sc.Snapshots {
		for k := range dp.KindCounts {
			kindSet[k] = struct{}{}
		}
		for k := range dp.RealmCounts {
			realmSet[k] = struct{}{}
		}
	}
	kinds := sortedKeys(kindSet)
	realms := sortedKeys(realmSet)

	header := []string{"tick", "year", "total_agents", "total_mortals", "avg_qi", "avg_age", "avg_combat_power", "avg_aggression",
		"deaths", "births", "breakthroughs", "mortal_conversions"}
	for _, k := range kinds {
		header = append(header, "kind_"+k)
	}
	for _, r := range realms {
		header = append(header, "realm_"+r)
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, dp := range sc.Snapshots {
		row := []string{
			fmt.Sprintf("%d", dp.Tick),
			fmt.Sprintf("%.2f", dp.Year),
			fmt.Sprintf("%d", dp.TotalAgents),
			fmt.Sprintf("%.0f", dp.TotalMortals),
			fmt.Sprintf("%.2f", dp.AvgQi),
			fmt.Sprintf("%.2f", dp.AvgAge),
			fmt.Sprintf("%.2f", dp.AvgCP),
			fmt.Sprintf("%.4f", dp.AvgAggression),
			fmt.Sprintf("%d", dp.Deaths),
			fmt.Sprintf("%d", dp.Births),
			fmt.Sprintf("%d", dp.Breakthroughs),
			fmt.Sprintf("%d", dp.MortalConversions),
		}
		for _, k := range kinds {
			row = append(row, fmt.Sprintf("%d", dp.KindCounts[k]))
		}
		for _, r := range realms {
			row = append(row, fmt.Sprintf("%d", dp.RealmCounts[r]))
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
