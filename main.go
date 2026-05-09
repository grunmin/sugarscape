package main

import (
	"fmt"
	"os"
	"time"

	"github.com/runmin/sugarscape/engine"
	"github.com/runmin/sugarscape/scenarios/cultivation"
)

func main() {
	cfg := engine.DefaultEngineConfig()
	scnCfg := cultivation.DefaultScenarioConfig()

	// Create world.
	world := engine.NewWorld(cfg)

	// Setup cultivation scenario.
	cultivation.Setup(world)

	// Print header.
	fmt.Println("=== 修仙世界模拟器 ===")
	fmt.Printf("世界: %d×%d  初始修仙者: %d  初始妖兽: %d  种子: %d\n",
		cfg.GridWidth, cfg.GridHeight,
		scnCfg.InitialCultivators, scnCfg.InitialBeasts, cfg.Seed)
	fmt.Println()

	maxTicks := int64(500)
	snapshotEvery := 10
	startTime := time.Now()

	// Print stats header.
	fmt.Printf("%-6s %-6s %-8s %-6s %-6s %-6s %-6s %-6s %-6s %-6s\n",
		"tick", "year", "total", "练气", "筑基", "金丹", "元婴", "化神", "avg_qi", "avg_cp")
	fmt.Println("------ ------ -------- ------ ------ ------ ------ ------ ------ ------")

	// Run simulation.
	for tick := int64(0); tick < maxTicks; tick++ {
		world.Tick()

		if world.Clock.Tick%int64(snapshotEvery) == 0 {
			world.Stats.Snapshot(world.Curr, world.Clock.Tick, world.Clock.Year())
		}

		// Print live stats every 50 ticks.
		if world.Clock.Tick%50 == 0 {
			printTickStats(world)
		}
	}

	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Printf("模拟完成，耗时 %s\n", elapsed)

	// Final snapshot.
	world.Stats.Snapshot(world.Curr, world.Clock.Tick, world.Clock.Year())

	// Export CSV.
	outPath := "output/stats.csv"
	if err := os.MkdirAll("output", 0755); err != nil {
		fmt.Printf("无法创建 output 目录: %v\n", err)
		os.Exit(1)
	}
	if err := world.Stats.ExportCSV(outPath); err != nil {
		fmt.Printf("CSV 导出失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("统计数据已导出到 %s (%d 条记录)\n", outPath, len(world.Stats.Snapshots))

	// Final summary.
	printFinalSummary(world)
}

func printTickStats(w *engine.World) {
	agents := w.Curr.Agents
	realms := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	total := 0
	var qiSum, cpSum float64

	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		total++
		r := int(agents.Attrs[i].Num["realm"])
		if r < 1 {
			r = 1
		}
		realms[r]++
		qiSum += agents.Attrs[i].Num["qi"]
		cpSum += agents.Attrs[i].Num["combat_power"]
	}

	avgQi := 0.0
	avgCP := 0.0
	if total > 0 {
		avgQi = qiSum / float64(total)
		avgCP = cpSum / float64(total)
	}

	fmt.Printf("%-6d %-6.0f %-8d %-6d %-6d %-6d %-6d %-6d %-6.0f %-6.0f\n",
		w.Clock.Tick, w.Clock.Year(), total,
		realms[1], realms[2], realms[3], realms[4], realms[5],
		avgQi, avgCP)
}

func printFinalSummary(w *engine.World) {
	snaps := w.Stats.Snapshots
	if len(snaps) == 0 {
		return
	}
	last := snaps[len(snaps)-1]
	first := snaps[0]

	fmt.Println()
	fmt.Println("=== 最终摘要 ===")
	fmt.Printf("模拟年数: %.0f 年\n", last.Year-first.Year)
	fmt.Printf("最终修仙者数量: %d\n", last.KindCounts["cultivator"])
	fmt.Printf("最终妖兽数量: %d\n", last.KindCounts["spirit_beast"])
	fmt.Println("境界分布:")
	for _, name := range []string{"练气", "筑基", "金丹", "元婴", "化神"} {
		fmt.Printf("  %s: %d\n", name, last.RealmCounts[name])
	}
	fmt.Printf("总死亡: %d  总出生: %d  总突破: %d\n",
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Deaths }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Births }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Breakthroughs }))
}

func sumInt(snaps []engine.DataPoint, fn func(engine.DataPoint) int) int {
	total := 0
	for _, dp := range snaps {
		total += fn(dp)
	}
	return total
}
