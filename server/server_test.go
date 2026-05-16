package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/runmin/sugarscape/engine"
	"github.com/runmin/sugarscape/scenarios/cultivation"
)

func TestExportAnalysisSnapshotWritesJSONAndCSV(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 20
	cfg.GridHeight = 20
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	cultivation.Setup(w)
	w.Tick()

	dir := t.TempDir()
	d := NewDashboard(w, DashboardConfig{
		HeatmapScale:     5,
		UpdateEveryTicks: 1,
		MaxEvents:        10,
		AnalysisDir:      dir,
	})

	if err := d.exportAnalysisSnapshot(1, time.Minute); err != nil {
		t.Fatalf("export analysis snapshot: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "analysis_001m_tick_*.json"))
	if err != nil {
		t.Fatalf("glob json: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("json export count = %d, want 1", len(matches))
	}
	if info, err := os.Stat(matches[0]); err != nil || info.Size() == 0 {
		t.Fatalf("json export stat = (%v, %v), want non-empty file", info, err)
	}

	matches, err = filepath.Glob(filepath.Join(dir, "analysis_001m_tick_*.csv"))
	if err != nil {
		t.Fatalf("glob csv: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("csv export count = %d, want 1", len(matches))
	}
	if info, err := os.Stat(matches[0]); err != nil || info.Size() == 0 {
		t.Fatalf("csv export stat = (%v, %v), want non-empty file", info, err)
	}
}

func TestSnapshotIncludesSectNamesAndCultivatorIndexes(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 10
	cfg.GridHeight = 10
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	a := engine.NewAttrBag()
	a.Num["realm"] = 1
	a.Num["qi"] = 50
	a.Num["qi_max"] = 100
	a.Num["combat_power"] = 10
	a.Str["sect"] = "青云宗"
	b := engine.NewAttrBag()
	b.Num["realm"] = 2
	b.Num["qi"] = 80
	b.Num["qi_max"] = 100
	b.Num["combat_power"] = 20
	b.Str["sect"] = "赤霞宗"
	w.Next.Agents.Add("cultivator", 1, 1, a)
	w.Next.Agents.Add("cultivator", 2, 2, b)
	w.Curr = w.Next

	d := NewDashboard(w, DashboardConfig{HeatmapScale: 5, UpdateEveryTicks: 1, MaxEvents: 10})
	var snapshot StateSnapshot
	if err := json.Unmarshal(d.buildSnapshot(), &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	if len(snapshot.SectNames) != 2 {
		t.Fatalf("sect names = %v, want two names from cultivators", snapshot.SectNames)
	}
	if len(snapshot.Stats.SectStats) != 2 {
		t.Fatalf("sect stats = %+v, want two stats", snapshot.Stats.SectStats)
	}
	for _, c := range snapshot.Cultivators {
		if c.SectIdx < 0 {
			t.Fatalf("cultivator %+v has no sect index", c)
		}
		if c.SectIdx >= len(snapshot.SectNames) {
			t.Fatalf("cultivator %+v has out-of-range sect index for %v", c, snapshot.SectNames)
		}
	}
}

func TestTrackedSnapshotIncludesMoveTarget(t *testing.T) {
	cfg := engine.DefaultEngineConfig()
	cfg.GridWidth = 10
	cfg.GridHeight = 10
	cfg.NumWorkers = 1

	w := engine.NewWorld(cfg)
	attrs := engine.NewAttrBag()
	attrs.Num["realm"] = 1
	attrs.Num["qi"] = 50
	attrs.Num["qi_max"] = 100
	attrs.Num["combat_power"] = 10
	idx := w.Next.Agents.Add("cultivator", 1, 1, attrs)
	id := w.Next.Agents.ID[idx]
	cultivation.SetAgentMoveTarget(w.Next.Agents, id, 5, 6, cfg.GridWidth, cfg.GridHeight)
	w.Curr = w.Next

	d := NewDashboard(w, DashboardConfig{HeatmapScale: 5, UpdateEveryTicks: 1, MaxEvents: 10})
	d.trackedIDs[id] = true
	var snapshot StateSnapshot
	if err := json.Unmarshal(d.buildSnapshot(), &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	if len(snapshot.Tracked) != 1 {
		t.Fatalf("tracked count = %d, want 1", len(snapshot.Tracked))
	}
	got := snapshot.Tracked[0]
	if !got.HasMoveTarget || got.MoveTargetX != 5 || got.MoveTargetY != 6 {
		t.Fatalf("tracked move target = (%v,%d,%d), want active target at (5,6)", got.HasMoveTarget, got.MoveTargetX, got.MoveTargetY)
	}
}

func TestDashboardFiltersLifecycleEventsExceptTrackedAgents(t *testing.T) {
	w := engine.NewWorld(engine.DefaultEngineConfig())
	d := NewDashboard(w, DashboardConfig{HeatmapScale: 5, UpdateEveryTicks: 1, MaxEvents: 10})
	d.trackedIDs[2] = true

	events := []engine.NotableEvent{
		{Kind: "死亡", Realm: "筑基", AgentID: 1, Reason: "战斗死亡"},
		{Kind: "诞生", Realm: "元婴", AgentID: 2, Reason: "金丹 -> 元婴"},
		{Kind: "立宗", Reason: "青云宗成立"},
	}

	filtered := d.filterNotableEvents(events)
	if len(filtered) != 2 {
		t.Fatalf("filtered events = %d, want tracked lifecycle event plus world event", len(filtered))
	}
	if filtered[0].AgentID != 2 || filtered[0].Kind != "诞生" {
		t.Fatalf("first filtered event = %+v, want tracked birth event", filtered[0])
	}
	if filtered[1].Kind != "立宗" {
		t.Fatalf("second filtered event = %+v, want sect world event", filtered[1])
	}
}
