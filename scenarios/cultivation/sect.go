package cultivation

import (
	"fmt"
	"sync"

	"github.com/runmin/sugarscape/engine"
)

type SectSystem struct {
	candidates map[int]*sectCandidate
}

func (s *SectSystem) Name() string  { return "SectSystem" }
func (s *SectSystem) Priority() int { return 6 }

type SectSite struct {
	Name        string
	Style       string
	X, Y        int
	Radius      int
	FoundedTick int64
	Deaths      int
}

type sectCandidate struct {
	x, y           int
	count          int
	sustainedTicks int
	combatDeaths   int
	potential      float64
}

type sectDeathPoint struct{ x, y int }

var (
	sectMu            sync.Mutex
	sectNames         []string
	sectTraits        []SectTrait
	sectSites         []SectSite
	pendingSectDeaths []sectDeathPoint
)

func resetSectState() {
	sectMu.Lock()
	defer sectMu.Unlock()
	sectNames = nil
	sectTraits = nil
	sectSites = nil
	pendingSectDeaths = nil
}

func recordSectCandidateDeath(x, y int) {
	sectMu.Lock()
	pendingSectDeaths = append(pendingSectDeaths, sectDeathPoint{x: x, y: y})
	sectMu.Unlock()
}

func drainSectCandidateDeaths() []sectDeathPoint {
	sectMu.Lock()
	defer sectMu.Unlock()
	if len(pendingSectDeaths) == 0 {
		return nil
	}
	deaths := append([]sectDeathPoint(nil), pendingSectDeaths...)
	pendingSectDeaths = pendingSectDeaths[:0]
	return deaths
}

func (s *SectSystem) Tick(w *engine.World) {
	cfg := DefaultScenarioConfig()
	interval := cfg.SectFormationCheckEvery
	if interval < 1 {
		interval = 1
	}
	if w.Clock.Tick%int64(interval) != 0 {
		return
	}
	if s.candidates == nil {
		s.candidates = make(map[int]*sectCandidate)
	}

	agents := w.Next.Agents
	env := w.Next.Env
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	radius := cfg.SectFormationRadius
	if radius < 1 {
		radius = 1
	}

	deathsByKey := make(map[int]int)
	for _, death := range drainSectCandidateDeaths() {
		key, _, _ := sectClusterKey(death.x, death.y, radius, gridW, gridH)
		deathsByKey[key]++
	}

	type clusterNow struct {
		count     int
		sumX      int
		sumY      int
		potential float64
	}
	current := make(map[int]*clusterNow)
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" || agents.Attrs[i].Str["sect"] != "" {
			continue
		}
		x, y := agents.X[i], agents.Y[i]
		cell := env.Cells[y*gridW+x]
		if !isSectFormationCell(cell, cfg) {
			continue
		}
		key, _, _ := sectClusterKey(x, y, radius, gridW, gridH)
		c := current[key]
		if c == nil {
			c = &clusterNow{}
			current[key] = c
		}
		c.count++
		c.sumX += x
		c.sumY += y
		potential := sectCellPotential(cell, cfg)
		if potential > c.potential {
			c.potential = potential
		}
	}

	seen := make(map[int]bool, len(current))
	for key, now := range current {
		seen[key] = true
		c := s.candidates[key]
		if c == nil {
			_, cx, cy := sectClusterKey(now.sumX/now.count, now.sumY/now.count, radius, gridW, gridH)
			c = &sectCandidate{x: cx, y: cy}
			s.candidates[key] = c
		}
		c.count = now.count
		c.x = now.sumX / now.count
		c.y = now.sumY / now.count
		c.potential = now.potential
		if now.count >= cfg.SectFormationMinCultivators {
			c.sustainedTicks += interval
		} else if c.sustainedTicks > 0 {
			c.sustainedTicks -= interval
		}
		c.combatDeaths += deathsByKey[key]

		if s.shouldFoundSect(c, cfg, gridW, gridH) {
			s.foundSect(w, c, cfg)
			delete(s.candidates, key)
		}
	}

	for key, c := range s.candidates {
		if seen[key] {
			continue
		}
		c.count = 0
		if c.sustainedTicks > 0 {
			c.sustainedTicks -= interval
			if c.sustainedTicks < 0 {
				c.sustainedTicks = 0
			}
		}
		if c.sustainedTicks == 0 && c.combatDeaths == 0 {
			delete(s.candidates, key)
		}
	}
}

func (s *SectSystem) shouldFoundSect(c *sectCandidate, cfg ScenarioConfig, gridW, gridH int) bool {
	if c.count < cfg.SectFormationMinCultivators {
		return false
	}
	if c.sustainedTicks < cfg.SectFormationMinSustainTicks {
		return false
	}
	if c.combatDeaths < cfg.SectFormationMinCombatDeaths {
		return false
	}
	return !hasNearbySect(c.x, c.y, cfg.SectFormationExistingSectExclusion, gridW, gridH)
}

func (s *SectSystem) foundSect(w *engine.World, c *sectCandidate, cfg ScenarioConfig) {
	name, trait := newSectIdentity(c, cfg)
	site := SectSite{
		Name:        name,
		Style:       trait.Style,
		X:           c.x,
		Y:           c.y,
		Radius:      cfg.SectFormationInfluenceRadius,
		FoundedTick: w.Clock.Tick + 1,
		Deaths:      c.combatDeaths,
	}

	sectMu.Lock()
	sectNames = append(sectNames, name)
	sectTraits = append(sectTraits, trait)
	sectSites = append(sectSites, site)
	sectMu.Unlock()

	assignNearbyLooseCultivators(w.Next.Agents, name, trait, c.x, c.y, cfg.SectFormationInfluenceRadius, w.Config.GridWidth, w.Config.GridHeight)
	w.Stats.RecordNotableEvent(engine.NotableEvent{
		Tick:   w.Clock.Tick + 1,
		Year:   float64(w.Clock.Tick+1) / float64(w.Config.TicksPerYear),
		Kind:   "立宗",
		Realm:  trait.Style,
		X:      c.x,
		Y:      c.y,
		Reason: fmt.Sprintf("%s 成立：%d人聚集，%d场战死", name, c.count, c.combatDeaths),
	})
}

func isSectFormationCell(cell engine.Cell, cfg ScenarioConfig) bool {
	return cell.Env1 >= cfg.SpiritMax+cfg.SectFormationMinSpiritMaxBonus ||
		cell.Env2 >= cfg.SpiritRegenRate+cfg.SectFormationMinRegenBonus
}

func sectCellPotential(cell engine.Cell, cfg ScenarioConfig) float64 {
	maxPotential := cfg.SpiritMax + cfg.BlessedLandMaxBonus
	if maxPotential <= 0 {
		return 0
	}
	potential := cell.Env1 / maxPotential
	regen := 0.0
	if cfg.BlessedLandRegenBonus > 0 {
		regen = (cell.Env2 - cfg.SpiritRegenRate) / cfg.BlessedLandRegenBonus
	}
	if regen > potential {
		potential = regen
	}
	if potential < 0 {
		return 0
	}
	if potential > 1 {
		return 1
	}
	return potential
}

func sectClusterKey(x, y, radius, gridW, gridH int) (int, int, int) {
	bin := radius * 2
	if bin < 1 {
		bin = 1
	}
	bx := engine.Wrap(x, gridW) / bin
	by := engine.Wrap(y, gridH) / bin
	cx := (bx*bin + bin/2) % gridW
	cy := (by*bin + bin/2) % gridH
	return by*(gridW/bin+1) + bx, cx, cy
}

func hasNearbySect(x, y, radius, gridW, gridH int) bool {
	if radius <= 0 {
		return false
	}
	for _, site := range SectSites() {
		if toroidalDistanceSq(x, y, site.X, site.Y, gridW, gridH) <= radius*radius {
			return true
		}
	}
	return false
}

func newSectIdentity(c *sectCandidate, cfg ScenarioConfig) (string, SectTrait) {
	style := "开山"
	trait := SectTrait{
		Style:                  style,
		RecruitMultiplier:      1.15,
		PowerRecruitMultiplier: 1.0,
		BreakthroughMultiplier: 1.05,
		AggressionBias:         0.02,
	}
	if c.combatDeaths >= cfg.SectFormationMinCombatDeaths*2 {
		style = "战盟"
		trait = SectTrait{Style: style, RecruitMultiplier: 0.95, PowerRecruitMultiplier: 1.30, BreakthroughMultiplier: 1.05, AggressionBias: 0.10}
	} else if c.potential >= 0.75 {
		style = "灵脉"
		trait = SectTrait{Style: style, RecruitMultiplier: 1.05, PowerRecruitMultiplier: 1.10, BreakthroughMultiplier: 1.20, AggressionBias: -0.03}
	} else if c.count >= cfg.SectFormationMinCultivators*2 {
		style = "外门"
		trait = SectTrait{Style: style, RecruitMultiplier: 1.35, PowerRecruitMultiplier: 0.90, BreakthroughMultiplier: 1.00, AggressionBias: 0.03}
	}

	sectMu.Lock()
	next := len(sectNames) + 1
	sectMu.Unlock()
	return fmt.Sprintf("%s宗%d", style, next), trait
}

func assignNearbyLooseCultivators(agents *engine.AgentStore, name string, trait SectTrait, x, y, radius, gridW, gridH int) {
	if radius <= 0 {
		return
	}
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" || agents.Attrs[i].Str["sect"] != "" {
			continue
		}
		if toroidalDistanceSq(x, y, agents.X[i], agents.Y[i], gridW, gridH) > radius*radius {
			continue
		}
		agents.Attrs[i].Str["sect"] = name
		agents.Attrs[i].Num["aggression"] = clampNorm(agents.Attrs[i].Num["aggression"]+trait.AggressionBias, 0, 1)
	}
}

func nearestSectAt(x, y, gridW, gridH int, cfg ScenarioConfig) (string, SectTrait, bool) {
	bestIdx := -1
	bestDist := int(^uint(0) >> 1)
	for i, site := range SectSites() {
		radius := site.Radius
		if radius <= 0 {
			radius = cfg.SectFormationInfluenceRadius
		}
		dist := toroidalDistanceSq(x, y, site.X, site.Y, gridW, gridH)
		if dist <= radius*radius && dist < bestDist {
			bestIdx = i
			bestDist = dist
		}
	}
	if bestIdx < 0 {
		return "", SectTrait{}, false
	}
	names := SectNames()
	traits := SectTraits()
	if bestIdx >= len(names) {
		return "", SectTrait{}, false
	}
	trait := SectTrait{RecruitMultiplier: 1, PowerRecruitMultiplier: 1, BreakthroughMultiplier: 1}
	if bestIdx < len(traits) {
		trait = traits[bestIdx]
	}
	return names[bestIdx], trait, true
}

func toroidalDistanceSq(ax, ay, bx, by, gridW, gridH int) int {
	dx := toroidalDelta(ax, bx, gridW)
	dy := toroidalDelta(ay, by, gridH)
	return dx*dx + dy*dy
}

func SectNames() []string {
	sectMu.Lock()
	defer sectMu.Unlock()
	names := make([]string, len(sectNames))
	copy(names, sectNames)
	return names
}

func SectTraits() []SectTrait {
	sectMu.Lock()
	defer sectMu.Unlock()
	traits := make([]SectTrait, len(sectTraits))
	copy(traits, sectTraits)
	return traits
}

func SectSites() []SectSite {
	sectMu.Lock()
	defer sectMu.Unlock()
	sites := make([]SectSite, len(sectSites))
	copy(sites, sectSites)
	return sites
}
