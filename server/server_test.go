package server

import (
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
