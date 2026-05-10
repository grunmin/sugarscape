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

	initStart := time.Now()
	world := engine.NewWorld(cfg)
	cultivation.Setup(world)
	initElapsed := time.Since(initStart)

	fmt.Println("=== 修仙世界模拟器 ===")
	fmt.Printf("世界: %d×%d  凡人/格: %.0f  种子: %d  并行: %d 核\n",
		cfg.GridWidth, cfg.GridHeight,
		scnCfg.MortalBaseDensity, cfg.Seed, cfg.NumWorkers)
	fmt.Printf("部落数: %d  凡人→修仙转化率: %.3f  初始化耗时: %v\n",
		scnCfg.NumTribes, scnCfg.MortalConvChance, initElapsed.Round(time.Millisecond))
	fmt.Println()

	maxTicks := int64(1000)
	snapshotEvery := 20
	startTime := time.Now()
	lastPrint := time.Now()

	fmt.Printf("%-6s %-6s %-8s %-12s %-8s %-8s %-8s %-8s %-8s %-10s\n",
		"tick", "year", "cultiv", "mortals", "练气", "筑基", "金丹", "元婴", "化神", "elapsed")
	fmt.Println("------ ------ -------- ------------ -------- -------- -------- -------- -------- ----------")

	for tick := int64(0); tick < maxTicks; tick++ {
		world.Tick()

		if world.Clock.Tick%int64(snapshotEvery) == 0 {
			world.Stats.Snapshot(world.Curr, world.Next.Env, world.Clock.Tick, world.Clock.Year())
		}

		if time.Since(lastPrint) >= 5*time.Second {
			printTickStats(world, startTime)
			lastPrint = time.Now()
		}
	}

	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Printf("模拟完成，耗时 %s (%.2f ms/tick)\n", elapsed.Round(time.Millisecond),
		float64(elapsed.Milliseconds())/float64(maxTicks))

	// Final snapshot.
	world.Stats.Snapshot(world.Curr, world.Next.Env, world.Clock.Tick, world.Clock.Year())

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

	printFinalSummary(world)
}

func printTickStats(w *engine.World, startTime time.Time) {
	agents := w.Curr.Agents
	realms := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	total := 0

	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		total++
		r := int(agents.Attrs[i].Num["realm"])
		if r < 1 {
			r = 1
		}
		if r > 5 {
			r = 5
		}
		realms[r]++
	}

	elapsed := time.Since(startTime).Round(time.Second)
	fmt.Printf("%-6d %-6.0f %-8d %-12.0f %-8d %-8d %-8d %-8d %-8d %-10s\n",
		w.Clock.Tick, w.Clock.Year(), total, w.Next.Env.TotalMortals(),
		realms[1], realms[2], realms[3], realms[4], realms[5], elapsed)
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
	fmt.Printf("凡人总数: %.0f\n", last.TotalMortals)
	fmt.Println("境界分布:")
	for _, name := range []string{"练气", "筑基", "金丹", "元婴", "化神"} {
		fmt.Printf("  %s: %d\n", name, last.RealmCounts[name])
	}
	fmt.Printf("平均攻击性: %.4f\n", last.AvgAggression)
	fmt.Printf("总死亡: %d  总出生: %d  总突破: %d  凡人转化: %d\n",
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Deaths }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Births }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Breakthroughs }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.MortalConversions }))
}

func sumInt(snaps []engine.DataPoint, fn func(engine.DataPoint) int) int {
	total := 0
	for _, dp := range snaps {
		total += fn(dp)
	}
	return total
}
