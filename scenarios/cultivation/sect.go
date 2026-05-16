package cultivation

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/runmin/sugarscape/engine"
)

type SectSystem struct {
	candidates map[int]*sectCandidate
	stalled    map[string]int
}

func (s *SectSystem) Name() string  { return "SectSystem" }
func (s *SectSystem) Priority() int { return 6 }

type SectSite struct {
	Name        string
	Style       string
	Kind        string
	X, Y        int
	Radius      int
	FoundedTick int64
	Deaths      int
	Potential   float64
	NetBenefit  float64
}

type sectCandidate struct {
	x, y           int
	count          int
	sustainedTicks int
	combatDeaths   int
	jindanCount    int
	yuanyingCount  int
	potential      float64
}

type sectExpansionPlan struct {
	name       string
	trait      SectTrait
	x, y       int
	potential  float64
	value      float64
	cost       float64
	netBenefit float64
}

type sectInfluencePlan struct {
	siteIndex  int
	name       string
	trait      SectTrait
	x, y       int
	oldRadius  int
	newRadius  int
	value      float64
	cost       float64
	netBenefit float64
}

type sectAggressivePlan struct {
	name          string
	trait         SectTrait
	kind          string
	targetSect    string
	x, y          int
	radius        int
	dispatchCount int
	value         float64
	cost          float64
	netBenefit    float64
	attackerPower float64
	defenderPower float64
	siteIndex     int
	potential     float64
}

type sectDeathPoint struct{ x, y int }

type sectSiteRef struct {
	index int
	site  SectSite
}

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
	if s.stalled == nil {
		s.stalled = make(map[string]int)
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
		count         int
		jindanCount   int
		yuanyingCount int
		sumX          int
		sumY          int
		potential     float64
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
		realm := int(agents.Attrs[i].Num["realm"])
		if realm >= 4 {
			c.yuanyingCount++
		} else if realm == 3 {
			c.jindanCount++
		}
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
		c.jindanCount = now.jindanCount
		c.yuanyingCount = now.yuanyingCount
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

	s.expandSects(w, cfg)
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
	if c.yuanyingCount < cfg.SectFormationMinYuanying && c.jindanCount < cfg.SectFormationMinJindan {
		return false
	}
	return !hasNearbySect(c.x, c.y, cfg.SectFormationExistingSectExclusion, gridW, gridH)
}

func (s *SectSystem) foundSect(w *engine.World, c *sectCandidate, cfg ScenarioConfig) {
	name, trait := newSectIdentity(c, cfg)
	site := SectSite{
		Name:        name,
		Style:       trait.Style,
		Kind:        "立宗",
		X:           c.x,
		Y:           c.y,
		Radius:      cfg.SectFormationInfluenceRadius,
		FoundedTick: w.Clock.Tick + 1,
		Deaths:      c.combatDeaths,
		Potential:   c.potential,
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
		Reason: fmt.Sprintf("%s 成立：%d人聚集，金丹%d，元婴%d，%d场战死", name, c.count, c.jindanCount, c.yuanyingCount, c.combatDeaths),
	})
}

func (s *SectSystem) expandSects(w *engine.World, cfg ScenarioConfig) {
	interval := cfg.SectExpansionCheckEvery
	if interval <= 0 || w.Clock.Tick%int64(interval) != 0 {
		return
	}
	if s.stalled == nil {
		s.stalled = make(map[string]int)
	}

	statsByName := sectStatsByName(w.Next.Agents)
	for _, name := range SectNames() {
		stat := statsByName[name]
		if stat.Count < cfg.SectExpansionMinMembers {
			continue
		}

		if plan, ok := bestSectInfluenceGrowth(w, stat, cfg); ok && plan.netBenefit > cfg.SectExpansionNetBenefitThreshold {
			applySectInfluenceGrowth(w, plan, cfg)
			s.stalled[name] = 0
			continue
		}

		if plan, ok := bestSectExpansion(w, name, stat.Count, cfg); ok && plan.netBenefit > cfg.SectExpansionNetBenefitThreshold {
			applySectExpansion(w, plan, cfg)
			s.stalled[name] = 0
			continue
		}

		s.stalled[name] += interval
		if s.stalled[name] < cfg.SectAggressiveExpansionStallTicks {
			continue
		}
		if plan, ok := bestAggressiveExpansion(w, stat, statsByName, cfg); ok && plan.netBenefit > cfg.SectExpansionNetBenefitThreshold {
			applyAggressiveExpansion(w, plan, cfg)
			s.stalled[name] = 0
		}
	}
}

func sectStatsByName(agents *engine.AgentStore) map[string]SectStat {
	stats := CalculateSectStats(agents)
	out := make(map[string]SectStat, len(stats))
	for _, stat := range stats {
		out[stat.Name] = stat
	}
	return out
}

func bestSectInfluenceGrowth(w *engine.World, stat SectStat, cfg ScenarioConfig) (sectInfluencePlan, bool) {
	refs := sectSiteRefsForName(stat.Name)
	if len(refs) == 0 {
		return sectInfluencePlan{}, false
	}
	step := cfg.SectExpansionInfluenceStep
	if step <= 0 {
		step = 8
	}
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	best := sectInfluencePlan{name: stat.Name, trait: sectTraitForName(stat.Name)}
	found := false
	for _, ref := range refs {
		oldRadius := ref.site.Radius
		if oldRadius <= 0 {
			oldRadius = cfg.SectFormationInfluenceRadius
		}
		newRadius := oldRadius + step
		value := influenceGrowthValue(w.Next.Env, ref.site.X, ref.site.Y, oldRadius, newRadius, stat, cfg, gridW, gridH)
		cost := influenceGrowthCost(stat.Count, len(refs), newRadius, cfg)
		net := value - cost
		if !found || net > best.netBenefit {
			best = sectInfluencePlan{
				siteIndex:  ref.index,
				name:       stat.Name,
				trait:      sectTraitForName(stat.Name),
				x:          ref.site.X,
				y:          ref.site.Y,
				oldRadius:  oldRadius,
				newRadius:  newRadius,
				value:      value,
				cost:       cost,
				netBenefit: net,
			}
			found = true
		}
	}
	return best, found
}

func influenceGrowthValue(env *engine.Grid, x, y, oldRadius, newRadius int, stat SectStat, cfg ScenarioConfig, gridW, gridH int) float64 {
	sampleStep := cfg.SectFormationRadius / 4
	if sampleStep < 4 {
		sampleStep = 4
	}
	potentialSum := 0.0
	samples := 0
	oldSq := oldRadius * oldRadius
	newSq := newRadius * newRadius
	for dy := -newRadius; dy <= newRadius; dy += sampleStep {
		for dx := -newRadius; dx <= newRadius; dx += sampleStep {
			distSq := dx*dx + dy*dy
			if distSq <= oldSq || distSq > newSq {
				continue
			}
			cx := engine.Wrap(x+dx, gridW)
			cy := engine.Wrap(y+dy, gridH)
			potentialSum += sectCellPotential(env.Cells[cy*gridW+cx], cfg)
			samples++
		}
	}
	avgPotential := 0.0
	if samples > 0 {
		avgPotential = potentialSum / float64(samples)
	}
	combatValue := math.Sqrt(stat.CombatValue) * cfg.SectExpansionValuePerCombatPower
	return combatValue + avgPotential*cfg.SectExpansionValuePerPotential
}

func influenceGrowthCost(memberCount, siteCount, newRadius int, cfg ScenarioConfig) float64 {
	return cfg.SectExpansionBaseCost +
		float64(siteCount-1)*cfg.SectExpansionSiteCost +
		float64(memberCount)*cfg.SectExpansionMemberUpkeepCost +
		float64(newRadius)*cfg.SectExpansionOverextensionCost
}

func applySectInfluenceGrowth(w *engine.World, plan sectInfluencePlan, cfg ScenarioConfig) {
	sectMu.Lock()
	if plan.siteIndex >= 0 && plan.siteIndex < len(sectSites) &&
		sectSites[plan.siteIndex].Name == plan.name &&
		sectSites[plan.siteIndex].X == plan.x &&
		sectSites[plan.siteIndex].Y == plan.y {
		sectSites[plan.siteIndex].Radius = plan.newRadius
		sectSites[plan.siteIndex].NetBenefit = plan.netBenefit
	}
	sectMu.Unlock()

	assignNearbyLooseCultivators(w.Next.Agents, plan.name, plan.trait, plan.x, plan.y, plan.newRadius, w.Config.GridWidth, w.Config.GridHeight)
	applySectExpansionCost(w.Next.Agents, plan.name, plan.cost)
	eventTick := w.Clock.Tick + 1
	w.Stats.RecordNotableEvent(engine.NotableEvent{
		Tick:   eventTick,
		Year:   float64(eventTick) / float64(w.Config.TicksPerYear),
		Kind:   "扩张",
		Realm:  plan.trait.Style,
		X:      plan.x,
		Y:      plan.y,
		Reason: fmt.Sprintf("%s 势力范围扩张：r%d -> r%d，收益 %.1f，成本 %.1f，净 %.1f", plan.name, plan.oldRadius, plan.newRadius, plan.value, plan.cost, plan.netBenefit),
	})
}

func bestSectExpansion(w *engine.World, name string, memberCount int, cfg ScenarioConfig) (sectExpansionPlan, bool) {
	sites := SectSites()
	owned := make([]SectSite, 0, 4)
	for _, site := range sites {
		if site.Name == name {
			owned = append(owned, site)
		}
	}
	if len(owned) == 0 {
		return sectExpansionPlan{}, false
	}
	if cfg.SectExpansionMaxSites > 0 && len(owned) >= cfg.SectExpansionMaxSites {
		return sectExpansionPlan{}, false
	}

	searchRadius := cfg.SectExpansionSearchRadius
	if searchRadius <= 0 {
		searchRadius = cfg.SectFormationInfluenceRadius * 2
	}
	influenceRadius := cfg.SectExpansionInfluenceRadius
	if influenceRadius <= 0 {
		influenceRadius = cfg.SectFormationInfluenceRadius
	}
	step := cfg.SectFormationRadius / 4
	if step < 4 {
		step = 4
	}

	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	env := w.Next.Env
	best := sectExpansionPlan{name: name, trait: sectTraitForName(name)}
	found := false

	for _, site := range owned {
		for dy := -searchRadius; dy <= searchRadius; dy += step {
			for dx := -searchRadius; dx <= searchRadius; dx += step {
				distSq := dx*dx + dy*dy
				if distSq == 0 || distSq > searchRadius*searchRadius {
					continue
				}
				x := engine.Wrap(site.X+dx, gridW)
				y := engine.Wrap(site.Y+dy, gridH)
				if hasAnySiteOverlap(x, y, influenceRadius, gridW, gridH, sites) {
					continue
				}
				cell := env.Cells[y*gridW+x]
				potential := sectCellPotential(cell, cfg)
				if potential < cfg.SectExpansionMinPotential || !isSectExpansionCell(cell, cfg) {
					continue
				}

				localMembers := countSectMembersNear(w.Next.Agents, name, x, y, influenceRadius, gridW, gridH)
				conflict := expansionConflictPressure(w.Next.Agents, sites, name, x, y, searchRadius, gridW, gridH)
				distanceRatio := math.Sqrt(float64(distSq)) / float64(searchRadius)
				value := potential*cfg.SectExpansionValuePerPotential +
					float64(localMembers)*cfg.SectExpansionValuePerLocalMember
				cost := cfg.SectExpansionBaseCost +
					float64(len(owned))*cfg.SectExpansionSiteCost +
					float64(memberCount)*cfg.SectExpansionMemberUpkeepCost +
					distanceRatio*cfg.SectExpansionDistanceCost +
					conflict*cfg.SectExpansionConflictCost +
					float64(influenceRadius)*cfg.SectExpansionOverextensionCost
				net := value - cost
				if !found || net > best.netBenefit {
					best.x = x
					best.y = y
					best.potential = potential
					best.value = value
					best.cost = cost
					best.netBenefit = net
					found = true
				}
			}
		}
	}

	return best, found
}

func isSectExpansionCell(cell engine.Cell, cfg ScenarioConfig) bool {
	return cell.Env1 >= cfg.SpiritMax+cfg.SpiritSpringMaxBonus ||
		cell.Env2 >= cfg.SpiritRegenRate+cfg.SpiritSpringRegenBonus
}

func hasAnySiteOverlap(x, y, radius, gridW, gridH int, sites []SectSite) bool {
	if radius <= 0 {
		return false
	}
	for _, site := range sites {
		siteRadius := site.Radius
		if siteRadius <= 0 {
			siteRadius = radius
		}
		minDist := radius + siteRadius
		if toroidalDistanceSq(x, y, site.X, site.Y, gridW, gridH) <= minDist*minDist {
			return true
		}
	}
	return false
}

func countSectMembersNear(agents *engine.AgentStore, sect string, x, y, radius, gridW, gridH int) int {
	count := 0
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" || agents.Attrs[i].Str["sect"] != sect {
			continue
		}
		if toroidalDistanceSq(x, y, agents.X[i], agents.Y[i], gridW, gridH) <= radius*radius {
			count++
		}
	}
	return count
}

func expansionConflictPressure(agents *engine.AgentStore, sites []SectSite, sect string, x, y, radius, gridW, gridH int) float64 {
	pressure := 0.0
	for _, site := range sites {
		if site.Name == sect {
			continue
		}
		if toroidalDistanceSq(x, y, site.X, site.Y, gridW, gridH) <= radius*radius {
			pressure += 20
		}
	}
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		otherSect := agents.Attrs[i].Str["sect"]
		if otherSect == "" || otherSect == sect {
			continue
		}
		if toroidalDistanceSq(x, y, agents.X[i], agents.Y[i], gridW, gridH) <= radius*radius {
			pressure++
		}
	}
	return pressure
}

func applySectExpansion(w *engine.World, plan sectExpansionPlan, cfg ScenarioConfig) {
	radius := cfg.SectExpansionInfluenceRadius
	if radius <= 0 {
		radius = cfg.SectFormationInfluenceRadius
	}
	site := SectSite{
		Name:        plan.name,
		Style:       plan.trait.Style,
		Kind:        "扩张",
		X:           plan.x,
		Y:           plan.y,
		Radius:      radius,
		FoundedTick: w.Clock.Tick + 1,
		Potential:   plan.potential,
		NetBenefit:  plan.netBenefit,
	}

	sectMu.Lock()
	sectSites = append(sectSites, site)
	sectMu.Unlock()

	assignNearbyLooseCultivators(w.Next.Agents, plan.name, plan.trait, plan.x, plan.y, radius, w.Config.GridWidth, w.Config.GridHeight)
	applySectExpansionCost(w.Next.Agents, plan.name, plan.cost)
	eventTick := w.Clock.Tick + 1
	w.Stats.RecordNotableEvent(engine.NotableEvent{
		Tick:   eventTick,
		Year:   float64(eventTick) / float64(w.Config.TicksPerYear),
		Kind:   "扩张",
		Realm:  plan.trait.Style,
		X:      plan.x,
		Y:      plan.y,
		Reason: fmt.Sprintf("%s 扩张：潜力 %.2f，收益 %.1f，成本 %.1f，净 %.1f", plan.name, plan.potential, plan.value, plan.cost, plan.netBenefit),
	})
}

func bestAggressiveExpansion(w *engine.World, stat SectStat, statsByName map[string]SectStat, cfg ScenarioConfig) (sectAggressivePlan, bool) {
	best := sectAggressivePlan{name: stat.Name, trait: sectTraitForName(stat.Name)}
	found := false
	if plan, ok := bestAggressiveOccupation(w, stat, cfg); ok {
		best = plan
		found = true
	}
	if plan, ok := bestAggressiveConquest(w, stat, statsByName, cfg); ok && (!found || plan.netBenefit > best.netBenefit) {
		best = plan
		found = true
	}
	return best, found
}

func bestAggressiveOccupation(w *engine.World, stat SectStat, cfg ScenarioConfig) (sectAggressivePlan, bool) {
	owned := sectSiteRefsForName(stat.Name)
	if len(owned) == 0 {
		return sectAggressivePlan{}, false
	}
	radius := cfg.SectExpansionInfluenceRadius
	if radius <= 0 {
		radius = cfg.SectFormationInfluenceRadius
	}
	step := cfg.SectFormationRadius / 2
	if step < 8 {
		step = 8
	}
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	env := w.Next.Env
	sites := SectSites()
	dispatchCount := sectDispatchCount(stat.Count, cfg, false)
	best := sectAggressivePlan{name: stat.Name, trait: sectTraitForName(stat.Name), kind: "占领", radius: radius, dispatchCount: dispatchCount}
	found := false
	for y := 0; y < gridH; y += step {
		for x := 0; x < gridW; x += step {
			if hasAnySiteOverlap(x, y, radius, gridW, gridH, sites) {
				continue
			}
			cell := env.Cells[y*gridW+x]
			if !isSectExpansionCell(cell, cfg) {
				continue
			}
			potential := sectCellPotential(cell, cfg)
			if potential < cfg.SectExpansionMinPotential {
				continue
			}
			distRatio := nearestOwnedSiteDistanceRatio(x, y, owned, gridW, gridH)
			value := potential*cfg.SectExpansionValuePerPotential +
				math.Sqrt(stat.CombatValue)*cfg.SectExpansionValuePerCombatPower
			cost := aggressiveExpansionCost(stat.Count, len(owned), radius, distRatio, cfg)
			net := value - cost
			if !found || net > best.netBenefit {
				best.x = x
				best.y = y
				best.potential = potential
				best.value = value
				best.cost = cost
				best.netBenefit = net
				found = true
			}
		}
	}
	if found {
		best.attackerPower = topSectMemberPower(w.Next.Agents, stat.Name, dispatchCount)
	}
	return best, found
}

func bestAggressiveConquest(w *engine.World, stat SectStat, statsByName map[string]SectStat, cfg ScenarioConfig) (sectAggressivePlan, bool) {
	owned := sectSiteRefsForName(stat.Name)
	if len(owned) == 0 {
		return sectAggressivePlan{}, false
	}
	gridW, gridH := w.Config.GridWidth, w.Config.GridHeight
	dispatchCount := sectDispatchCount(stat.Count, cfg, true)
	attackerPower := topSectMemberPower(w.Next.Agents, stat.Name, dispatchCount)
	best := sectAggressivePlan{name: stat.Name, trait: sectTraitForName(stat.Name), kind: "攻占", dispatchCount: dispatchCount}
	found := false
	for _, target := range SectSites() {
		if target.Name == stat.Name {
			continue
		}
		targetStat := statsByName[target.Name]
		targetRadius := target.Radius
		if targetRadius <= 0 {
			targetRadius = cfg.SectFormationInfluenceRadius
		}
		defenderPower := localSectCombatValueNear(w.Next.Agents, target.Name, target.X, target.Y, targetRadius, gridW, gridH)
		if defenderPower <= 0 {
			defenderPower = targetStat.CombatValue
		}
		if defenderPower > 0 && attackerPower < defenderPower*cfg.SectConquestPowerAdvantage {
			continue
		}
		distRatio := nearestOwnedSiteDistanceRatio(target.X, target.Y, owned, gridW, gridH)
		value := target.Potential*cfg.SectExpansionValuePerPotential +
			float64(targetStat.Count)*cfg.SectExpansionValuePerLocalMember +
			math.Sqrt(stat.CombatValue)*cfg.SectExpansionValuePerCombatPower
		cost := aggressiveExpansionCost(stat.Count, len(owned), targetRadius, distRatio, cfg) +
			math.Sqrt(defenderPower)*cfg.SectExpansionValuePerCombatPower
		net := value - cost
		if !found || net > best.netBenefit {
			best.targetSect = target.Name
			best.x = target.X
			best.y = target.Y
			best.radius = targetRadius
			best.value = value
			best.cost = cost
			best.netBenefit = net
			best.attackerPower = attackerPower
			best.defenderPower = defenderPower
			best.siteIndex = siteIndexFor(target)
			best.potential = target.Potential
			found = true
		}
	}
	return best, found
}

func aggressiveExpansionCost(memberCount, siteCount, radius int, distanceRatio float64, cfg ScenarioConfig) float64 {
	base := cfg.SectExpansionBaseCost +
		float64(siteCount)*cfg.SectExpansionSiteCost +
		float64(memberCount)*cfg.SectExpansionMemberUpkeepCost +
		distanceRatio*cfg.SectExpansionDistanceCost +
		float64(radius)*cfg.SectExpansionOverextensionCost
	return base * cfg.SectAggressiveCostMultiplier
}

func applyAggressiveExpansion(w *engine.World, plan sectAggressivePlan, cfg ScenarioConfig) {
	dispatchSectForce(w.Next.Agents, plan.name, plan.dispatchCount, plan.x, plan.y, w.Config.GridWidth, w.Config.GridHeight)
	applySectExpansionCost(w.Next.Agents, plan.name, plan.cost)

	switch plan.kind {
	case "攻占":
		applySectConquest(plan)
		assignNearbyLooseCultivators(w.Next.Agents, plan.name, plan.trait, plan.x, plan.y, plan.radius, w.Config.GridWidth, w.Config.GridHeight)
	default:
		appendSectSite(SectSite{
			Name:        plan.name,
			Style:       plan.trait.Style,
			Kind:        plan.kind,
			X:           plan.x,
			Y:           plan.y,
			Radius:      plan.radius,
			FoundedTick: w.Clock.Tick + 1,
			Potential:   plan.potential,
			NetBenefit:  plan.netBenefit,
		})
		assignNearbyLooseCultivators(w.Next.Agents, plan.name, plan.trait, plan.x, plan.y, plan.radius, w.Config.GridWidth, w.Config.GridHeight)
	}

	eventTick := w.Clock.Tick + 1
	reason := fmt.Sprintf("%s 远征%s：派出%d人，收益 %.1f，成本 %.1f，净 %.1f", plan.name, plan.kind, plan.dispatchCount, plan.value, plan.cost, plan.netBenefit)
	if plan.kind == "攻占" {
		reason = fmt.Sprintf("%s 远征攻占 %s：派出%d人，攻方 %.0f，守方 %.0f，收益 %.1f，成本 %.1f，净 %.1f",
			plan.name, plan.targetSect, plan.dispatchCount, plan.attackerPower, plan.defenderPower, plan.value, plan.cost, plan.netBenefit)
	}
	w.Stats.RecordNotableEvent(engine.NotableEvent{
		Tick:   eventTick,
		Year:   float64(eventTick) / float64(w.Config.TicksPerYear),
		Kind:   "远征",
		Realm:  plan.trait.Style,
		X:      plan.x,
		Y:      plan.y,
		Reason: reason,
	})
}

func applySectExpansionCost(agents *engine.AgentStore, sect string, cost float64) {
	if cost <= 0 {
		return
	}
	members := 0
	for i := range agents.ID {
		if agents.Alive[i] && agents.Kind[i] == "cultivator" && agents.Attrs[i].Str["sect"] == sect {
			members++
		}
	}
	if members == 0 {
		return
	}
	perMember := cost / float64(members)
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" || agents.Attrs[i].Str["sect"] != sect {
			continue
		}
		agents.Attrs[i].Num["qi"] -= perMember
		if agents.Attrs[i].Num["qi"] < 0 {
			agents.Attrs[i].Num["qi"] = 0
		}
	}
}

func sectDispatchCount(memberCount int, cfg ScenarioConfig, heavy bool) int {
	if memberCount <= 0 {
		return 0
	}
	minFrac := cfg.SectAggressiveMinDispatchFrac
	if minFrac <= 0 {
		minFrac = 1.0 / 3.0
	}
	maxFrac := cfg.SectAggressiveMaxDispatchFrac
	if maxFrac <= 0 || maxFrac < minFrac {
		maxFrac = 2.0 / 3.0
	}
	frac := minFrac
	if heavy {
		frac = maxFrac
	}
	count := int(math.Ceil(float64(memberCount) * frac))
	maxCount := int(math.Floor(float64(memberCount) * maxFrac))
	if maxCount < 1 {
		maxCount = 1
	}
	if count > maxCount {
		count = maxCount
	}
	if count < 1 {
		count = 1
	}
	return count
}

func topSectMemberPower(agents *engine.AgentStore, sect string, count int) float64 {
	indices := sortedSectMembersByPower(agents, sect)
	if count > len(indices) {
		count = len(indices)
	}
	power := 0.0
	for _, idx := range indices[:count] {
		cp := agents.Attrs[idx].Num["combat_power"]
		power += cp * cp
	}
	return power
}

func dispatchSectForce(agents *engine.AgentStore, sect string, count, x, y, gridW, gridH int) {
	if count <= 0 {
		return
	}
	indices := sortedSectMembersByPower(agents, sect)
	if count > len(indices) {
		count = len(indices)
	}
	for n, idx := range indices[:count] {
		dx := n%5 - 2
		dy := (n/5)%5 - 2
		agents.X[idx] = engine.Wrap(x+dx, gridW)
		agents.Y[idx] = engine.Wrap(y+dy, gridH)
		agents.Attrs[idx].Num["moved_this_tick"] = 1
	}
}

func sortedSectMembersByPower(agents *engine.AgentStore, sect string) []int {
	indices := make([]int, 0)
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" || agents.Attrs[i].Str["sect"] != sect {
			continue
		}
		indices = append(indices, i)
	}
	sort.Slice(indices, func(i, j int) bool {
		a := agents.Attrs[indices[i]].Num["combat_power"]
		b := agents.Attrs[indices[j]].Num["combat_power"]
		if a == b {
			return indices[i] < indices[j]
		}
		return a > b
	})
	return indices
}

func localSectCombatValueNear(agents *engine.AgentStore, sect string, x, y, radius, gridW, gridH int) float64 {
	if radius <= 0 {
		radius = 1
	}
	radiusSq := radius * radius
	value := 0.0
	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" || agents.Attrs[i].Str["sect"] != sect {
			continue
		}
		if toroidalDistanceSq(x, y, agents.X[i], agents.Y[i], gridW, gridH) > radiusSq {
			continue
		}
		cp := agents.Attrs[i].Num["combat_power"]
		value += cp * cp
	}
	return value
}

func nearestOwnedSiteDistanceRatio(x, y int, owned []sectSiteRef, gridW, gridH int) float64 {
	if len(owned) == 0 {
		return 1
	}
	best := int(^uint(0) >> 1)
	for _, ref := range owned {
		dist := toroidalDistanceSq(x, y, ref.site.X, ref.site.Y, gridW, gridH)
		if dist < best {
			best = dist
		}
	}
	maxDist := math.Hypot(float64(gridW)/2, float64(gridH)/2)
	if maxDist <= 0 {
		return 0
	}
	ratio := math.Sqrt(float64(best)) / maxDist
	if ratio > 1 {
		return 1
	}
	return ratio
}

func sectSiteRefsForName(name string) []sectSiteRef {
	sectMu.Lock()
	defer sectMu.Unlock()
	refs := make([]sectSiteRef, 0)
	for i, site := range sectSites {
		if site.Name == name {
			refs = append(refs, sectSiteRef{index: i, site: site})
		}
	}
	return refs
}

func appendSectSite(site SectSite) {
	sectMu.Lock()
	sectSites = append(sectSites, site)
	sectMu.Unlock()
}

func applySectConquest(plan sectAggressivePlan) {
	sectMu.Lock()
	defer sectMu.Unlock()
	if plan.siteIndex >= 0 && plan.siteIndex < len(sectSites) && sectSites[plan.siteIndex].Name == plan.targetSect {
		sectSites[plan.siteIndex].Name = plan.name
		sectSites[plan.siteIndex].Style = plan.trait.Style
		sectSites[plan.siteIndex].Kind = plan.kind
		sectSites[plan.siteIndex].NetBenefit = plan.netBenefit
	}
}

func siteIndexFor(target SectSite) int {
	sectMu.Lock()
	defer sectMu.Unlock()
	for i, site := range sectSites {
		if site.Name == target.Name && site.X == target.X && site.Y == target.Y && site.FoundedTick == target.FoundedTick {
			return i
		}
	}
	return -1
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
	var bestSite SectSite
	found := false
	bestDist := int(^uint(0) >> 1)
	for _, site := range SectSites() {
		radius := site.Radius
		if radius <= 0 {
			radius = cfg.SectFormationInfluenceRadius
		}
		dist := toroidalDistanceSq(x, y, site.X, site.Y, gridW, gridH)
		if dist <= radius*radius && dist < bestDist {
			bestSite = site
			found = true
			bestDist = dist
		}
	}
	if !found {
		return "", SectTrait{}, false
	}
	return bestSite.Name, sectTraitForName(bestSite.Name), true
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
