package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"
	"unsafe"

	"github.com/runmin/sugarscape/engine"
	"github.com/runmin/sugarscape/scenarios/cultivation"
)

const autoPauseEvery = 5 * time.Minute

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

	maxTicks := int64(300000)
	snapshotEvery := 20
	startTime := time.Now()
	lastPrint := time.Now()
	lastPause := time.Now()
	pausedDuration := time.Duration(0)
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	interrupted := false
	quitRequested := false

	fmt.Printf("%-6s %-6s %-8s %-12s %-8s %-8s %-8s %-8s %-8s %-10s\n",
		"tick", "year", "cultiv", "mortals", "练气", "筑基", "金丹", "元婴", "化神", "elapsed")
	fmt.Println("------ ------ -------- ------------ -------- -------- -------- -------- -------- ----------")

	for tick := int64(0); tick < maxTicks; tick++ {
		select {
		case <-interrupts:
			interrupted = true
		default:
		}
		if interrupted {
			break
		}

		world.Tick()

		if world.Clock.Tick%int64(snapshotEvery) == 0 {
			world.Stats.Snapshot(world.Curr, world.Curr.Env, world.Clock.Tick, world.Clock.Year())
		}

		if time.Since(lastPrint) >= 5*time.Second {
			printTickStats(world, startTime, pausedDuration)
			lastPrint = time.Now()
		}

		if time.Since(lastPause) >= autoPauseEvery {
			printTickStats(world, startTime, pausedDuration)
			quit, interruptedBySignal, pausedFor := autoPause(world, interrupts)
			pausedDuration += pausedFor
			lastPause = time.Now()
			lastPrint = time.Now()
			if interruptedBySignal {
				interrupted = true
				break
			}
			if quit {
				quitRequested = true
				break
			}
		}
	}

	elapsed := time.Since(startTime) - pausedDuration
	fmt.Println()
	if interrupted {
		fmt.Printf("收到中断信号，已在 tick %d 正常退出。\n", world.Clock.Tick)
	}
	if quitRequested {
		fmt.Printf("收到退出键，已在 tick %d 正常退出。\n", world.Clock.Tick)
	}
	ticksRun := world.Clock.Tick
	msPerTick := 0.0
	if ticksRun > 0 {
		msPerTick = float64(elapsed.Milliseconds()) / float64(ticksRun)
	}
	fmt.Printf("模拟完成，耗时 %s (%.2f ms/tick)\n", elapsed.Round(time.Millisecond), msPerTick)

	// Final snapshot.
	if len(world.Stats.Snapshots) == 0 ||
		world.Stats.Snapshots[len(world.Stats.Snapshots)-1].Tick != world.Clock.Tick {
		world.Stats.Snapshot(world.Curr, world.Curr.Env, world.Clock.Tick, world.Clock.Year())
	}

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
	printNotableEvents(world.Stats.DrainNotableEvents())

	printFinalSummary(world)
}

func printTickStats(w *engine.World, startTime time.Time, pausedDuration time.Duration) {
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

	elapsed := (time.Since(startTime) - pausedDuration).Round(time.Second)
	fmt.Printf("%-6d %-6.0f %-8d %-12.0f %-8d %-8d %-8d %-8d %-8d %-10s\n",
		w.Clock.Tick, w.Clock.Year(), total, w.Curr.Env.TotalMortals(),
		realms[1], realms[2], realms[3], realms[4], realms[5], elapsed)
	printNotableEvents(w.Stats.DrainNotableEvents())
}

func autoPause(w *engine.World, interrupts <-chan os.Signal) (quit bool, interrupted bool, pausedFor time.Duration) {
	start := time.Now()
	fmt.Printf("\n自动暂停: tick=%d year=%.1f。按任意键继续，按 q 退出。\n", w.Clock.Tick, w.Clock.Year())

	key, interrupted, err := readPauseKey(interrupts)
	if interrupted {
		return false, true, time.Since(start)
	}
	if err != nil {
		fmt.Printf("读取按键失败: %v。保持暂停；按 Ctrl+C 退出。\n", err)
		for {
			select {
			case <-interrupts:
				return false, true, time.Since(start)
			case <-time.After(time.Second):
			}
		}
	}
	if key == 'q' || key == 'Q' {
		return true, false, time.Since(start)
	}
	fmt.Println("继续模拟。")
	return false, false, time.Since(start)
}

func readPauseKey(interrupts <-chan os.Signal) (byte, bool, error) {
	input, cleanup, err := pauseInput()
	if err != nil {
		return 0, false, err
	}
	defer cleanup()

	fd := int(input.Fd())
	if isTerminal(input) {
		restore, err := enableRawInput(fd, false)
		if err != nil {
			return 0, false, err
		}
		defer restore()

		buf := []byte{0}
		for {
			select {
			case <-interrupts:
				return 0, true, nil
			default:
			}
			n, err := input.Read(buf)
			if err != nil {
				if err == syscall.EINTR || err == syscall.EAGAIN {
					continue
				}
				if err == io.EOF {
					time.Sleep(100 * time.Millisecond)
					continue
				}
				return 0, false, err
			}
			if n > 0 {
				return buf[0], false, nil
			}
		}
	}

	keyCh := make(chan byte, 1)
	errCh := make(chan error, 1)
	go func() {
		buf := []byte{0}
		if _, err := input.Read(buf); err != nil {
			errCh <- err
			return
		}
		keyCh <- buf[0]
	}()
	select {
	case <-interrupts:
		return 0, true, nil
	case err := <-errCh:
		return 0, false, err
	case key := <-keyCh:
		return key, false, nil
	}
}

func pauseInput() (*os.File, func(), error) {
	if isTerminal(os.Stdin) {
		return os.Stdin, func() {}, nil
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return nil, func() {}, err
	}
	return tty, func() { _ = tty.Close() }, nil
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func enableRawInput(fd int, blocking bool) (func(), error) {
	var oldState syscall.Termios
	if err := ioctlTermios(fd, syscall.TIOCGETA, &oldState); err != nil {
		return nil, err
	}
	newState := oldState
	newState.Lflag &^= syscall.ICANON | syscall.ECHO
	if blocking {
		newState.Cc[syscall.VMIN] = 1
		newState.Cc[syscall.VTIME] = 0
	} else {
		newState.Cc[syscall.VMIN] = 0
		newState.Cc[syscall.VTIME] = 1
	}
	if err := ioctlTermios(fd, syscall.TIOCSETA, &newState); err != nil {
		return nil, err
	}
	return func() {
		_ = ioctlTermios(fd, syscall.TIOCSETA, &oldState)
		fmt.Println()
	}, nil
}

func ioctlTermios(fd int, req uint, termios *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(termios)))
	if errno != 0 {
		return errno
	}
	return nil
}

func printNotableEvents(events []engine.NotableEvent) {
	type eventCount struct {
		realm  string
		kind   string
		reason string
		count  int
	}
	counts := make(map[string]eventCount)
	for _, ev := range events {
		if ev.Realm != "元婴" && ev.Realm != "化神" {
			continue
		}
		key := ev.Realm + "\x00" + ev.Kind + "\x00" + ev.Reason
		item := counts[key]
		item.realm = ev.Realm
		item.kind = ev.Kind
		item.reason = ev.Reason
		item.count++
		counts[key] = item
	}

	if len(counts) == 0 {
		return
	}

	items := make([]eventCount, 0, len(counts))
	for _, item := range counts {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if realmRank(items[i].realm) != realmRank(items[j].realm) {
			return realmRank(items[i].realm) < realmRank(items[j].realm)
		}
		if eventKindRank(items[i].kind) != eventKindRank(items[j].kind) {
			return eventKindRank(items[i].kind) < eventKindRank(items[j].kind)
		}
		return items[i].reason < items[j].reason
	})

	for _, item := range items {
		fmt.Printf("  %s%s: reason=%s count=%d\n", item.realm, item.kind, item.reason, item.count)
	}
}

func realmRank(realm string) int {
	switch realm {
	case "元婴":
		return 4
	case "化神":
		return 5
	default:
		return 99
	}
}

func eventKindRank(kind string) int {
	switch kind {
	case "诞生":
		return 0
	case "死亡":
		return 1
	default:
		return 99
	}
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
	fmt.Println("各境界平均攻击性:")
	for _, rs := range realmAggressionStats(w) {
		if rs.Count == 0 {
			fmt.Printf("  %s: n=0 avg=0.0000\n", rs.Name)
		} else {
			fmt.Printf("  %s: n=%d avg=%.4f\n", rs.Name, rs.Count, rs.Avg)
		}
	}
	fmt.Printf("总死亡: %d  总出生: %d  总突破: %d  凡人转化: %d\n",
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Deaths }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Births }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.Breakthroughs }),
		sumInt(snaps, func(dp engine.DataPoint) int { return dp.MortalConversions }))
}

type realmAggressionStat struct {
	Name  string
	Count int
	Avg   float64
}

func realmAggressionStats(w *engine.World) []realmAggressionStat {
	names := []string{"练气", "筑基", "金丹", "元婴", "化神"}
	counts := make([]int, len(names))
	sums := make([]float64, len(names))
	agents := w.Curr.Agents

	for i := range agents.ID {
		if !agents.Alive[i] || agents.Kind[i] != "cultivator" {
			continue
		}
		realm := int(agents.Attrs[i].Num["realm"])
		if realm < 1 {
			realm = 1
		}
		if realm > len(names) {
			realm = len(names)
		}
		idx := realm - 1
		counts[idx]++
		sums[idx] += agents.Attrs[i].Num["aggression"]
	}

	out := make([]realmAggressionStat, len(names))
	for i, name := range names {
		out[i] = realmAggressionStat{Name: name, Count: counts[i]}
		if counts[i] > 0 {
			out[i].Avg = sums[i] / float64(counts[i])
		}
	}
	return out
}

func sumInt(snaps []engine.DataPoint, fn func(engine.DataPoint) int) int {
	total := 0
	for _, dp := range snaps {
		total += fn(dp)
	}
	return total
}
