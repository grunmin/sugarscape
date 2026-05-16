// Package server provides a web dashboard for real-time simulation visualization.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/runmin/sugarscape/engine"
	"github.com/runmin/sugarscape/scenarios/cultivation"
)

//go:embed index.html
var frontendFS embed.FS

// DashboardConfig configures the web dashboard.
type DashboardConfig struct {
	Port             int // HTTP port
	HeatmapScale     int // downsample factor (e.g., 5 means 200x200 from 1000x1000)
	UpdateEveryTicks int // send state every N ticks
	MaxEvents        int // max notable events to keep
}

// DefaultDashboardConfig returns sensible defaults.
func DefaultDashboardConfig() DashboardConfig {
	return DashboardConfig{
		Port:             8080,
		HeatmapScale:     5,
		UpdateEveryTicks: 1,
		MaxEvents:        100,
	}
}

// Dashboard is the web-based simulation control center.
type Dashboard struct {
	world *engine.World
	cfg   DashboardConfig

	mu         sync.Mutex
	paused     bool
	stepSignal chan struct{}
	speed      int // target ticks per second, 0 = unlimited

	// SSE clients
	clientsMu sync.RWMutex
	clients   map[chan []byte]struct{}

	// Tracking
	trackedIDs   map[int]bool
	trackedMu    sync.RWMutex
	trackLogSize int // max entries per tracked agent

	// Feature cache (detected once)
	features     []FeatureInfo
	featuresOnce sync.Once

	// State
	notableEvents []engine.NotableEvent
	eventsMu      sync.Mutex

	stopCh chan struct{}
	doneCh chan struct{}
}

// FeatureInfo describes a spirit feature on the map.
type FeatureInfo struct {
	Type   string  `json:"type"` // "spring", "vein", "blessed_land"
	CX     int     `json:"cx"`
	CY     int     `json:"cy"`
	Radius int     `json:"radius"`
	DX     int     `json:"dx,omitempty"` // vein direction
	DY     int     `json:"dy,omitempty"`
	Length int     `json:"length,omitempty"` // vein length
	Boost  float64 `json:"boost"`
}

// NewDashboard creates and returns a new Dashboard.
func NewDashboard(w *engine.World, cfg DashboardConfig) *Dashboard {
	if cfg.HeatmapScale < 1 {
		cfg.HeatmapScale = 5
	}
	if cfg.UpdateEveryTicks < 1 {
		cfg.UpdateEveryTicks = 1
	}
	if cfg.MaxEvents < 1 {
		cfg.MaxEvents = 100
	}
	if cfg.Port < 1 {
		cfg.Port = 8080
	}
	return &Dashboard{
		world:        w,
		cfg:          cfg,
		stepSignal:   make(chan struct{}, 1),
		speed:        10,
		clients:      make(map[chan []byte]struct{}),
		trackedIDs:   make(map[int]bool),
		trackLogSize: 200,
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
	}
}

// Start begins the simulation loop and HTTP server.
func (d *Dashboard) Start() error {
	http.HandleFunc("/api/stream", d.handleSSE)
	http.HandleFunc("/api/state", d.handleState)
	http.HandleFunc("/api/pause", d.handlePause)
	http.HandleFunc("/api/resume", d.handleResume)
	http.HandleFunc("/api/step", d.handleStep)
	http.HandleFunc("/api/speed", d.handleSpeed)
	http.HandleFunc("/api/track", d.handleTrack)
	http.HandleFunc("/api/untrack", d.handleUntrack)
	http.HandleFunc("/api/stats", d.handleStatsHistory)

	// Serve frontend
	frontendContent, err := fs.ReadFile(frontendFS, "index.html")
	if err != nil {
		return fmt.Errorf("failed to read frontend: %w", err)
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(frontendContent)
	})

	// Start simulation loop in background
	go d.simulationLoop()

	addr := fmt.Sprintf(":%d", d.cfg.Port)
	log.Printf("Dashboard starting at http://localhost%s", addr)
	return http.ListenAndServe(addr, nil)
}

// Stop gracefully stops the dashboard.
func (d *Dashboard) Stop() {
	close(d.stopCh)
	<-d.doneCh
}

// --- Simulation Loop ---

func (d *Dashboard) simulationLoop() {
	defer close(d.doneCh)

	ticker := time.NewTicker(time.Second / 10) // 10 ticks/s base
	defer ticker.Stop()

	ticksSinceUpdate := 0

	for {
		select {
		case <-d.stopCh:
			return
		case <-ticker.C:
			d.mu.Lock()
			isPaused := d.paused
			d.mu.Unlock()

			if isPaused {
				// Wait for step signal or unpause
				select {
				case <-d.stepSignal:
					// Execute one tick
				case <-d.stopCh:
					return
				default:
					continue
				}
			}

			// Advance simulation
			d.world.Tick()

			// Collect notable events
			d.eventsMu.Lock()
			newEvents := d.world.Stats.DrainNotableEvents()
			d.notableEvents = append(d.notableEvents, newEvents...)
			if len(d.notableEvents) > d.cfg.MaxEvents {
				excess := len(d.notableEvents) - d.cfg.MaxEvents
				d.notableEvents = d.notableEvents[excess:]
			}
			d.eventsMu.Unlock()

			// Snapshot stats if needed
			ticksSinceUpdate++
			if ticksSinceUpdate >= d.cfg.UpdateEveryTicks {
				ticksSinceUpdate = 0
				snapshot := d.buildSnapshot()
				d.broadcast(snapshot)
			}

			// Throttle to target speed
			if isPaused {
				// After single step, re-enter pause
				d.mu.Lock()
				d.paused = true
				d.mu.Unlock()
			}
		}
	}
}

// --- State Snapshot ---

// StateSnapshot is the complete world state sent to the frontend.
type StateSnapshot struct {
	Tick        int64            `json:"tick"`
	Year        float64          `json:"year"`
	Width       int              `json:"width"`
	Height      int              `json:"height"`
	HMWidth     int              `json:"hmWidth"`
	HMHeight    int              `json:"hmHeight"`
	Paused      bool             `json:"paused"`
	Speed       int              `json:"speed"`
	Stats       StatsData        `json:"stats"`
	SpiritMap   string           `json:"spiritMap"` // base64 encoded uint8
	MortalMap   string           `json:"mortalMap"` // base64 encoded uint16
	SpiritMax   float64          `json:"spiritMax"` // global max for scale
	MortalMax   float64          `json:"mortalMax"` // global max for scale
	SectNames   []string         `json:"sectNames"`
	SectSites   []SectSiteInfo   `json:"sectSites"`
	Cultivators []CultivatorInfo `json:"cultivators"`
	Features    []FeatureInfo    `json:"features"`
	Tracked     []TrackedAgent   `json:"tracked"`
	Events      []EventInfo      `json:"events"`
}

// StatsData holds aggregated statistics.
type StatsData struct {
	TotalAgents       int            `json:"totalAgents"`
	TotalCultivators  int            `json:"totalCultivators"`
	TotalMortals      float64        `json:"totalMortals"`
	AvgQi             float64        `json:"avgQi"`
	AvgAge            float64        `json:"avgAge"`
	AvgCP             float64        `json:"avgCP"`
	AvgAggression     float64        `json:"avgAggression"`
	Deaths            int            `json:"deaths"`
	Births            int            `json:"births"`
	Breakthroughs     int            `json:"breakthroughs"`
	MortalConversions int            `json:"mortalConversions"`
	RealmCounts       map[string]int `json:"realmCounts"`
	StrategyCounts    map[string]int `json:"strategyCounts"`
	SectStats         []SectStatData `json:"sectStats"`
}

// SectStatData holds per-sect statistics.
type SectStatData struct {
	Name        string  `json:"name"`
	Trait       string  `json:"trait"`
	SiteX       int     `json:"siteX"`
	SiteY       int     `json:"siteY"`
	Radius      int     `json:"radius"`
	FoundedTick int64   `json:"foundedTick"`
	Deaths      int     `json:"deaths"`
	Count       int     `json:"count"`
	CombatValue float64 `json:"combatValue"`
	MaxCP       float64 `json:"maxCP"`
	RealmCounts [6]int  `json:"realmCounts"`
}

// SectSiteInfo describes where a dynamically founded sect is centered.
type SectSiteInfo struct {
	Name        string `json:"name"`
	Trait       string `json:"trait"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Radius      int    `json:"radius"`
	FoundedTick int64  `json:"foundedTick"`
	Deaths      int    `json:"deaths"`
	Count       int    `json:"count"`
}

// CultivatorInfo is compact cultivator data for map rendering.
type CultivatorInfo struct {
	ID          int     `json:"id"`
	X           int     `json:"x"`
	Y           int     `json:"y"`
	Realm       int     `json:"r"`  // realm level 1-5
	QiFrac      float64 `json:"qf"` // 0-1
	SectIdx     int     `json:"si"` // -1 if none
	Strategy    int     `json:"st"` // strategy index
	CombatPower float64 `json:"cp"`
}

// TrackedAgent holds detailed info for a tracked cultivator.
type TrackedAgent struct {
	ID                   int     `json:"id"`
	X                    int     `json:"x"`
	Y                    int     `json:"y"`
	Realm                string  `json:"realm"`
	RealmLevel           int     `json:"realmLevel"`
	Qi                   float64 `json:"qi"`
	QiMax                float64 `json:"qiMax"`
	Age                  float64 `json:"age"`
	CombatPower          float64 `json:"cp"`
	Aggression           float64 `json:"aggression"`
	Strategy             string  `json:"strategy"`
	Sect                 string  `json:"sect"`
	Lifespan             float64 `json:"lifespan"`
	Alive                bool    `json:"alive"`
	BreakthroughProgress float64 `json:"btProg"` // qi/qi_max
	BreakthroughCD       float64 `json:"btCD"`
	LowSpiritYears       float64 `json:"lowSpiritYears"`
	CultivationSpeed     float64 `json:"cultSpeed"`
	MovedThisTick        bool    `json:"moved"`
}

// EventInfo is a notable event for the frontend.
type EventInfo struct {
	Tick    int64   `json:"tick"`
	Year    float64 `json:"year"`
	Kind    string  `json:"kind"`
	Realm   string  `json:"realm"`
	AgentID int     `json:"agentId"`
	X       int     `json:"x"`
	Y       int     `json:"y"`
	Reason  string  `json:"reason"`
}

// Strategy index to name mapping.
var strategyNames = []string{"aggressive", "peaceful", "merchant", "hermit", "bandit"}

func strategyIndex(name string) int {
	for i, s := range strategyNames {
		if s == name {
			return i
		}
	}
	return -1
}

func (d *Dashboard) buildSnapshot() []byte {
	w := d.world
	agents := w.Curr.Agents
	env := w.Curr.Env
	// Detect features once
	d.featuresOnce.Do(func() {
		d.features = detectFeatures(env, w.Config.GridWidth, w.Config.GridHeight)
	})

	hmW := w.Config.GridWidth / d.cfg.HeatmapScale
	if hmW < 1 {
		hmW = 1
	}
	hmH := w.Config.GridHeight / d.cfg.HeatmapScale
	if hmH < 1 {
		hmH = 1
	}

	// Build heatmaps
	spiritCells := make([]float64, hmW*hmH)
	mortalCells := make([]float64, hmW*hmH)
	spiritMaxVal := 0.0
	mortalMaxVal := 0.0

	for hmY := 0; hmY < hmH; hmY++ {
		for hmX := 0; hmX < hmW; hmX++ {
			var spiritSum, mortalSum float64
			count := 0
			for dy := 0; dy < d.cfg.HeatmapScale; dy++ {
				for dx := 0; dx < d.cfg.HeatmapScale; dx++ {
					wx := hmX*d.cfg.HeatmapScale + dx
					wy := hmY*d.cfg.HeatmapScale + dy
					if wx >= w.Config.GridWidth || wy >= w.Config.GridHeight {
						continue
					}
					idx := wy*w.Config.GridWidth + wx
					spiritSum += env.Cells[idx].Env0
					mortalSum += env.Cells[idx].MortalPop
					count++
				}
			}
			avgSpirit := spiritSum / float64(count)
			avgMortal := mortalSum / float64(count)
			cellIdx := hmY*hmW + hmX
			spiritCells[cellIdx] = avgSpirit
			mortalCells[cellIdx] = avgMortal
			if avgSpirit > spiritMaxVal {
				spiritMaxVal = avgSpirit
			}
			if avgMortal > mortalMaxVal {
				mortalMaxVal = avgMortal
			}
		}
	}

	// Encode heatmaps as base64
	spiritBytes := make([]byte, len(spiritCells))
	for i, v := range spiritCells {
		if spiritMaxVal > 0 {
			spiritBytes[i] = byte(v / spiritMaxVal * 255)
		}
	}

	mortalBytes := make([]byte, len(mortalCells)*2)
	for i, v := range mortalCells {
		var val uint16
		if mortalMaxVal > 0 {
			val = uint16(v / mortalMaxVal * 65535)
		}
		mortalBytes[i*2] = byte(val >> 8)
		mortalBytes[i*2+1] = byte(val)
	}

	spiritB64 := base64Encode(spiritBytes)
	mortalB64 := base64Encode(mortalBytes)

	// Collect cultivator info
	totalCultivators := 0
	var qiSum, ageSum, cpSum, aggSum float64
	realmCounts := map[string]int{}
	strategyCounts := map[string]int{}

	// Pre-count for sizing
	cultCount := 0
	for i := range agents.ID {
		if agents.Alive[i] && agents.Kind[i] == "cultivator" {
			cultCount++
		}
	}

	cultivators := make([]CultivatorInfo, 0, cultCount)
	sectNames := cultivation.SectNames()
	sectIndex := make(map[string]int, len(sectNames))
	for si, sn := range sectNames {
		sectIndex[sn] = si
	}

	for i := range agents.ID {
		if !agents.Alive[i] {
			continue
		}
		if agents.Kind[i] == "cultivator" {
			totalCultivators++
			realm := int(agents.Attrs[i].Num["realm"])
			if realm < 1 {
				realm = 1
			}
			if realm > 5 {
				realm = 5
			}
			realmName := realmNameForLevel(realm)
			realmCounts[realmName]++
			qi := agents.Attrs[i].Num["qi"]
			qiMax := agents.Attrs[i].Num["qi_max"]
			qiFrac := 0.0
			if qiMax > 0 {
				qiFrac = qi / qiMax
			}
			cp := agents.Attrs[i].Num["combat_power"]
			strategy := agents.Attrs[i].Str["strategy"]
			strategyCounts[strategy]++
			sect := agents.Attrs[i].Str["sect"]
			sectIdx := -1
			if sect != "" {
				if si, ok := sectIndex[sect]; ok {
					sectIdx = si
				} else {
					sectIdx = len(sectNames)
					sectIndex[sect] = sectIdx
					sectNames = append(sectNames, sect)
				}
			}

			qiSum += qi
			ageSum += agents.Attrs[i].Num["age"]
			cpSum += cp
			aggSum += agents.Attrs[i].Num["aggression"]

			cultivators = append(cultivators, CultivatorInfo{
				ID:          agents.ID[i],
				X:           agents.X[i],
				Y:           agents.Y[i],
				Realm:       realm,
				QiFrac:      qiFrac,
				SectIdx:     sectIdx,
				Strategy:    strategyIndex(strategy),
				CombatPower: cp,
			})
		}
	}

	if totalCultivators > 0 {
		qiSum /= float64(totalCultivators)
		ageSum /= float64(totalCultivators)
		cpSum /= float64(totalCultivators)
		aggSum /= float64(totalCultivators)
	}

	// Build sect stats
	sectStats := buildSectStats(agents, sectNames)
	sectSites := buildSectSites(sectStats)

	// Collect tracked agents
	d.trackedMu.RLock()
	tracked := make([]TrackedAgent, 0, len(d.trackedIDs))
	for id := range d.trackedIDs {
		ta := d.buildTrackedAgent(agents, id)
		if ta != nil {
			tracked = append(tracked, *ta)
		}
	}
	d.trackedMu.RUnlock()

	// Collect events
	d.eventsMu.Lock()
	events := make([]EventInfo, len(d.notableEvents))
	for i, ev := range d.notableEvents {
		events[i] = EventInfo{
			Tick:    ev.Tick,
			Year:    ev.Year,
			Kind:    ev.Kind,
			Realm:   ev.Realm,
			AgentID: ev.AgentID,
			X:       ev.X,
			Y:       ev.Y,
			Reason:  ev.Reason,
		}
	}
	d.eventsMu.Unlock()

	// Count deaths/births/etc from last snapshot
	deaths, births, breakthroughs, mortalConvs := 0, 0, 0, 0
	if len(d.world.Stats.Snapshots) > 0 {
		last := d.world.Stats.Snapshots[len(d.world.Stats.Snapshots)-1]
		deaths = last.Deaths
		births = last.Births
		breakthroughs = last.Breakthroughs
		mortalConvs = last.MortalConversions
	}

	d.mu.Lock()
	paused := d.paused
	speed := d.speed
	d.mu.Unlock()

	snapshot := StateSnapshot{
		Tick:        w.Clock.Tick,
		Year:        w.Clock.Year(),
		Width:       w.Config.GridWidth,
		Height:      w.Config.GridHeight,
		HMWidth:     hmW,
		HMHeight:    hmH,
		Paused:      paused,
		Speed:       speed,
		SpiritMap:   spiritB64,
		MortalMap:   mortalB64,
		SpiritMax:   spiritMaxVal,
		MortalMax:   mortalMaxVal,
		SectNames:   sectNames,
		SectSites:   sectSites,
		Cultivators: cultivators,
		Features:    d.features,
		Tracked:     tracked,
		Events:      events,
		Stats: StatsData{
			TotalAgents:       agents.Count(),
			TotalCultivators:  totalCultivators,
			TotalMortals:      env.TotalMortals(),
			AvgQi:             qiSum,
			AvgAge:            ageSum,
			AvgCP:             cpSum,
			AvgAggression:     aggSum,
			Deaths:            deaths,
			Births:            births,
			Breakthroughs:     breakthroughs,
			MortalConversions: mortalConvs,
			RealmCounts:       realmCounts,
			StrategyCounts:    strategyCounts,
			SectStats:         sectStats,
		},
	}

	data, _ := json.Marshal(snapshot)
	return data
}

func (d *Dashboard) buildTrackedAgent(agents *engine.AgentStore, id int) *TrackedAgent {
	for i := range agents.ID {
		if agents.ID[i] == id {
			if !agents.Alive[i] {
				return &TrackedAgent{
					ID:    id,
					Alive: false,
				}
			}
			realm := int(agents.Attrs[i].Num["realm"])
			if realm < 1 {
				realm = 1
			}
			rc := realmConfigForLevel(realm)
			qi := agents.Attrs[i].Num["qi"]
			qiMax := agents.Attrs[i].Num["qi_max"]
			if qiMax <= 0 {
				qiMax = rc.QiMultiplier * 100 // fallback
			}
			btProg := 0.0
			if qiMax > 0 {
				btProg = qi / qiMax
			}
			return &TrackedAgent{
				ID:                   id,
				X:                    agents.X[i],
				Y:                    agents.Y[i],
				Realm:                rc.Name,
				RealmLevel:           realm,
				Qi:                   qi,
				QiMax:                qiMax,
				Age:                  agents.Attrs[i].Num["age"],
				CombatPower:          agents.Attrs[i].Num["combat_power"],
				Aggression:           agents.Attrs[i].Num["aggression"],
				Strategy:             agents.Attrs[i].Str["strategy"],
				Sect:                 agents.Attrs[i].Str["sect"],
				Lifespan:             agents.Attrs[i].Num["lifespan"],
				Alive:                true,
				BreakthroughProgress: btProg,
				BreakthroughCD:       agents.Attrs[i].Num["breakthrough_cooldown"],
				LowSpiritYears:       agents.Attrs[i].Num["low_spirit_years"],
				CultivationSpeed:     agents.Attrs[i].Num["cultivation_speed"],
				MovedThisTick:        agents.Attrs[i].Num["moved_this_tick"] == 1,
			}
		}
	}
	return nil
}

// --- Feature Detection ---

func detectFeatures(env *engine.Grid, gridW, gridH int) []FeatureInfo {
	// Detection is expensive; do once and cache.
	// We detect cells where Env1 > 200 (well above base SpiritMax=100), indicating features.
	var features []FeatureInfo

	visited := make([]bool, len(env.Cells))
	threshold := 110.0 // above default SpiritMax of 100

	for i := range env.Cells {
		if visited[i] || env.Cells[i].Env1 <= threshold {
			continue
		}

		// Flood fill to find the cluster
		cluster := []int{}
		queue := []int{i}
		visited[i] = true
		for len(queue) > 0 {
			idx := queue[0]
			queue = queue[1:]
			cluster = append(cluster, idx)

			x := idx % gridW
			y := idx / gridW
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					nx := (x + dx + gridW) % gridW
					ny := (y + dy + gridH) % gridH
					ni := ny*gridW + nx
					if !visited[ni] && env.Cells[ni].Env1 > threshold {
						visited[ni] = true
						queue = append(queue, ni)
					}
				}
			}
		}

		if len(cluster) < 2 {
			continue
		}

		// Compute cluster center and radius
		var sumX, sumY float64
		for _, idx := range cluster {
			sumX += float64(idx % gridW)
			sumY += float64(idx / gridW)
		}
		cx := int(sumX/float64(len(cluster)) + 0.5)
		cy := int(sumY/float64(len(cluster)) + 0.5)

		// Max distance from center
		maxDist := 0
		for _, idx := range cluster {
			dx := idx%gridW - cx
			dy := idx/gridW - cy
			// Handle toroidal
			if dx > gridW/2 {
				dx -= gridW
			}
			if dx < -gridW/2 {
				dx += gridW
			}
			if dy > gridH/2 {
				dy -= gridH
			}
			if dy < -gridH/2 {
				dy += gridH
			}
			dist := dx*dx + dy*dy
			if dist > maxDist {
				maxDist = dist
			}
		}
		radius := int(sqrtF(float64(maxDist))) + 1

		// Classify feature by size and max Env1
		maxEnv1 := 0.0
		for _, idx := range cluster {
			if env.Cells[idx].Env1 > maxEnv1 {
				maxEnv1 = env.Cells[idx].Env1
			}
		}

		ftype := "spring"
		boost := maxEnv1 - 100.0
		if radius > 12 {
			ftype = "blessed_land"
		} else if len(cluster) > radius*2 {
			ftype = "vein"
		}

		features = append(features, FeatureInfo{
			Type:   ftype,
			CX:     cx,
			CY:     cy,
			Radius: radius,
			Boost:  boost,
		})
	}

	return features
}

func sqrtF(v float64) float64 {
	if v <= 0 {
		return 0
	}
	guess := v / 2
	for i := 0; i < 20; i++ {
		guess = (guess + v/guess) / 2
	}
	return guess
}

// --- Broadcast ---

func (d *Dashboard) broadcast(data []byte) {
	d.clientsMu.RLock()
	for ch := range d.clients {
		select {
		case ch <- data:
		default:
			// Client too slow, drop
		}
	}
	d.clientsMu.RUnlock()
}

// --- HTTP Handlers ---

func (d *Dashboard) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan []byte, 64)
	d.clientsMu.Lock()
	d.clients[ch] = struct{}{}
	d.clientsMu.Unlock()

	// Send initial state
	snapshot := d.buildSnapshot()
	fmt.Fprintf(w, "data: %s\n\n", string(snapshot))
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			d.clientsMu.Lock()
			delete(d.clients, ch)
			d.clientsMu.Unlock()
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}
	}
}

func (d *Dashboard) handleState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	snapshot := d.buildSnapshot()
	w.Write(snapshot)
}

func (d *Dashboard) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	d.mu.Lock()
	d.paused = true
	d.mu.Unlock()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"paused":true}`))
}

func (d *Dashboard) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	d.mu.Lock()
	d.paused = false
	d.mu.Unlock()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"paused":false}`))
}

func (d *Dashboard) handleStep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	d.mu.Lock()
	if !d.paused {
		d.paused = true
	}
	d.mu.Unlock()
	select {
	case d.stepSignal <- struct{}{}:
	default:
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"step":true}`))
}

func (d *Dashboard) handleSpeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Speed int `json:"speed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Speed < 1 {
		req.Speed = 1
	}
	if req.Speed > 100 {
		req.Speed = 100
	}
	d.mu.Lock()
	d.speed = req.Speed
	d.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(map[string]int{"speed": d.speed})
}

func (d *Dashboard) handleTrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID int `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.AgentID <= 0 {
		http.Error(w, "Invalid agent ID", http.StatusBadRequest)
		return
	}
	d.trackedMu.Lock()
	d.trackedIDs[req.AgentID] = true
	d.trackedMu.Unlock()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"tracked":true}`))
}

func (d *Dashboard) handleUntrack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID int `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	d.trackedMu.Lock()
	delete(d.trackedIDs, req.AgentID)
	d.trackedMu.Unlock()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"untracked":true}`))
}

func (d *Dashboard) handleStatsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	type HistoryPoint struct {
		Tick             int64   `json:"tick"`
		Year             float64 `json:"year"`
		TotalAgents      int     `json:"totalAgents"`
		TotalCultivators int     `json:"totalCultivators"`
		TotalMortals     float64 `json:"totalMortals"`
		AvgQi            float64 `json:"avgQi"`
		AvgAge           float64 `json:"avgAge"`
		AvgCP            float64 `json:"avgCP"`
		Deaths           int     `json:"deaths"`
		Breakthroughs    int     `json:"breakthroughs"`
	}
	history := make([]HistoryPoint, 0)
	for _, dp := range d.world.Stats.Snapshots {
		history = append(history, HistoryPoint{
			Tick:             dp.Tick,
			Year:             dp.Year,
			TotalAgents:      dp.TotalAgents,
			TotalCultivators: dp.KindCounts["cultivator"],
			TotalMortals:     dp.TotalMortals,
			AvgQi:            dp.AvgQi,
			AvgAge:           dp.AvgAge,
			AvgCP:            dp.AvgCP,
			Deaths:           dp.Deaths,
			Breakthroughs:    dp.Breakthroughs,
		})
	}
	json.NewEncoder(w).Encode(history)
}

var base64Table = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

func base64Encode(data []byte) string {
	var result []byte
	for i := 0; i < len(data); i += 3 {
		b0 := data[i]
		b1 := byte(0)
		b2 := byte(0)
		if i+1 < len(data) {
			b1 = data[i+1]
		}
		if i+2 < len(data) {
			b2 = data[i+2]
		}
		result = append(result,
			base64Table[b0>>2],
			base64Table[((b0&3)<<4)|(b1>>4)],
		)
		if i+1 < len(data) {
			result = append(result, base64Table[((b1&15)<<2)|(b2>>6)])
		} else {
			result = append(result, '=')
		}
		if i+2 < len(data) {
			result = append(result, base64Table[b2&63])
		} else {
			result = append(result, '=')
		}
	}
	return string(result)
}

// Helper to get realm config without importing cultivation package directly.
func realmConfigForLevel(level int) struct {
	Name             string
	QiMultiplier     float64
	CombatMultiplier float64
	Lifespan         float64
	BreakthroughBase float64
} {
	type rc struct {
		Name             string
		QiMultiplier     float64
		CombatMultiplier float64
		Lifespan         float64
		BreakthroughBase float64
	}
	realms := []rc{
		{Name: "凡人", QiMultiplier: 1, CombatMultiplier: 0.1, Lifespan: 70, BreakthroughBase: 0},
		{Name: "练气", QiMultiplier: 2, CombatMultiplier: 1, Lifespan: 120, BreakthroughBase: 0.10},
		{Name: "筑基", QiMultiplier: 6, CombatMultiplier: 3, Lifespan: 250, BreakthroughBase: 0.05},
		{Name: "金丹", QiMultiplier: 20, CombatMultiplier: 10, Lifespan: 500, BreakthroughBase: 0.03},
		{Name: "元婴", QiMultiplier: 60, CombatMultiplier: 30, Lifespan: 1000, BreakthroughBase: 0.02},
		{Name: "化神", QiMultiplier: 200, CombatMultiplier: 100, Lifespan: 3000, BreakthroughBase: 0.00},
	}
	if level < 0 {
		level = 0
	}
	if level >= len(realms) {
		level = len(realms) - 1
	}
	r := realms[level]
	return rc{
		Name:             r.Name,
		QiMultiplier:     r.QiMultiplier,
		CombatMultiplier: r.CombatMultiplier,
		Lifespan:         r.Lifespan,
		BreakthroughBase: r.BreakthroughBase,
	}
}

func realmNameForLevel(level int) string {
	names := map[int]string{
		0: "凡人",
		1: "练气",
		2: "筑基",
		3: "金丹",
		4: "元婴",
		5: "化神",
	}
	if n, ok := names[level]; ok {
		return n
	}
	return "未知"
}

func buildSectStats(agents *engine.AgentStore, sectNames []string) []SectStatData {
	stats := make([]SectStatData, len(sectNames))
	index := make(map[string]int, len(sectNames))
	traits := cultivation.SectTraits()
	sitesByName := make(map[string]cultivation.SectSite)
	for _, site := range cultivation.SectSites() {
		sitesByName[site.Name] = site
	}
	for i, name := range sectNames {
		stats[i].Name = name
		if i < len(traits) {
			stats[i].Trait = traits[i].Style
		}
		if site, ok := sitesByName[name]; ok {
			stats[i].Trait = site.Style
			stats[i].SiteX = site.X
			stats[i].SiteY = site.Y
			stats[i].Radius = site.Radius
			stats[i].FoundedTick = site.FoundedTick
			stats[i].Deaths = site.Deaths
		}
		index[name] = i
	}

	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		idx, ok := index[agents.Attrs[i].Str["sect"]]
		if !ok {
			continue
		}
		cp := agents.Attrs[i].Num["combat_power"]
		stats[idx].Count++
		stats[idx].CombatValue += cp * cp
		realm := int(agents.Attrs[i].Num["realm"])
		if realm < 1 {
			realm = 1
		}
		if realm > 5 {
			realm = 5
		}
		stats[idx].RealmCounts[realm]++
		if cp > stats[idx].MaxCP {
			stats[idx].MaxCP = cp
		}
	}
	return stats
}

func buildSectSites(stats []SectStatData) []SectSiteInfo {
	countByName := make(map[string]int, len(stats))
	for _, stat := range stats {
		countByName[stat.Name] = stat.Count
	}

	sites := cultivation.SectSites()
	out := make([]SectSiteInfo, 0, len(sites))
	for _, site := range sites {
		out = append(out, SectSiteInfo{
			Name:        site.Name,
			Trait:       site.Style,
			X:           site.X,
			Y:           site.Y,
			Radius:      site.Radius,
			FoundedTick: site.FoundedTick,
			Deaths:      site.Deaths,
			Count:       countByName[site.Name],
		})
	}
	return out
}
