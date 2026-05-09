package engine

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
)

// DataPoint holds aggregated stats at a point in time.
type DataPoint struct {
	Tick int64
	Year float64
	// Counts by kind
	KindCounts map[string]int
	// Counts by realm (realm name → count) — populated by scenario
	RealmCounts map[string]int
	// Totals
	TotalAgents int
	AvgQi       float64
	AvgAge      float64
	AvgCP       float64 // combat power
	// Events this tick
	Deaths        int
	Births        int
	Breakthroughs int
}

type StatsCollector struct {
	Snapshots     []DataPoint
	TickDeaths    int
	TickBirths    int
	TickBreakthru int
}

func NewStatsCollector() *StatsCollector {
	return &StatsCollector{}
}

// RecordDeath records an event for the current tick.
func (sc *StatsCollector) RecordDeath()  { sc.TickDeaths++ }
func (sc *StatsCollector) RecordBirth()  { sc.TickBirths++ }
func (sc *StatsCollector) RecordBreakthrough() { sc.TickBreakthru++ }

// Snapshot captures the current world state and resets tick counters.
func (sc *StatsCollector) Snapshot(f *Frame, tick int64, year float64) {
	dp := DataPoint{
		Tick:        tick,
		Year:        year,
		KindCounts:  make(map[string]int),
		RealmCounts: make(map[string]int),
		Deaths:      sc.TickDeaths,
		Births:      sc.TickBirths,
		Breakthroughs: sc.TickBreakthru,
	}

	var qiSum, ageSum, cpSum float64
	alive := 0
	for i := range f.Agents.ID {
		if !f.Agents.Alive[i] {
			continue
		}
		alive++
		kind := f.Agents.Kind[i]
		dp.KindCounts[kind]++

		realm := int(f.Agents.Attrs[i].Num["realm"])
		realmName := realmNames[realm]
		dp.RealmCounts[realmName]++

		qiSum += f.Agents.Attrs[i].Num["qi"]
		ageSum += f.Agents.Attrs[i].Num["age"]
		cpSum += f.Agents.Attrs[i].Num["combat_power"]
	}
	dp.TotalAgents = alive
	if alive > 0 {
		dp.AvgQi = qiSum / float64(alive)
		dp.AvgAge = ageSum / float64(alive)
		dp.AvgCP = cpSum / float64(alive)
	}

	sc.Snapshots = append(sc.Snapshots, dp)
	sc.TickDeaths = 0
	sc.TickBirths = 0
	sc.TickBreakthru = 0
}

var realmNames = map[int]string{
	0: "凡人",
	1: "练气",
	2: "筑基",
	3: "金丹",
	4: "元婴",
	5: "化神",
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

	// Collect all column names
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

	// Header
	header := []string{"tick", "year", "total_agents", "avg_qi", "avg_age", "avg_combat_power", "deaths", "births", "breakthroughs"}
	for _, k := range kinds {
		header = append(header, "kind_"+k)
	}
	for _, r := range realms {
		header = append(header, "realm_"+r)
	}
	if err := w.Write(header); err != nil {
		return err
	}

	// Rows
	for _, dp := range sc.Snapshots {
		row := []string{
			fmt.Sprintf("%d", dp.Tick),
			fmt.Sprintf("%.2f", dp.Year),
			fmt.Sprintf("%d", dp.TotalAgents),
			fmt.Sprintf("%.2f", dp.AvgQi),
			fmt.Sprintf("%.2f", dp.AvgAge),
			fmt.Sprintf("%.2f", dp.AvgCP),
			fmt.Sprintf("%d", dp.Deaths),
			fmt.Sprintf("%d", dp.Births),
			fmt.Sprintf("%d", dp.Breakthroughs),
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
