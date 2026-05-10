package main

import (
	"fmt"
	"io"
	"math/rand"
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
	tracker := newAgentTracker(int64(cfg.Seed)+20260510, 4)
	tracker.ensureTargets(world.Curr, world.Clock.Tick)
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
		tracker.observe(world)

		if world.Clock.Tick%int64(snapshotEvery) == 0 {
			world.Stats.Snapshot(world.Curr, world.Curr.Env, world.Clock.Tick, world.Clock.Year())
		}

		if time.Since(lastPrint) >= 5*time.Second {
			printTickStats(world, startTime, pausedDuration, tracker)
			lastPrint = time.Now()
		}

		if time.Since(lastPause) >= autoPauseEvery {
			printTickStats(world, startTime, pausedDuration, tracker)
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

func printTickStats(w *engine.World, startTime time.Time, pausedDuration time.Duration, tracker *agentTracker) {
	agents := w.Curr.Agents
	realms := map[int]int{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}
	qiStats := highRealmQiStats{}
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
		if r >= 3 {
			qiStats.add(r, agents.Attrs[i].Num["qi"])
		}
	}

	elapsed := (time.Since(startTime) - pausedDuration).Round(time.Second)
	fmt.Printf("%-6d %-6.0f %-8d %-12.0f %-8d %-8d %-8d %-8d %-8d %-10s\n",
		w.Clock.Tick, w.Clock.Year(), total, w.Curr.Env.TotalMortals(),
		realms[1], realms[2], realms[3], realms[4], realms[5], elapsed)
	printHighRealmQiStats(qiStats)
	tracker.printReport(w)
	printNotableEvents(w.Stats.DrainNotableEvents())
}

type realmQiStat struct {
	count int
	sum   float64
	max   float64
}

type highRealmQiStats [6]realmQiStat

func (s *highRealmQiStats) add(realm int, qi float64) {
	if realm < 3 || realm > 5 {
		return
	}
	stat := &s[realm]
	stat.count++
	stat.sum += qi
	if stat.count == 1 || qi > stat.max {
		stat.max = qi
	}
}

func printHighRealmQiStats(stats highRealmQiStats) {
	names := map[int]string{3: "金丹", 4: "元婴", 5: "化神"}
	fmt.Print("  高阶灵气 ")
	for realm := 3; realm <= 5; realm++ {
		stat := stats[realm]
		avg := 0.0
		if stat.count > 0 {
			avg = stat.sum / float64(stat.count)
		}
		if realm > 3 {
			fmt.Print(" | ")
		}
		fmt.Printf("%s: n=%d avg=%.1f max=%.1f", names[realm], stat.count, avg, stat.max)
	}
	fmt.Println()
}

type agentTracker struct {
	rng   *rand.Rand
	slots []agentTrackSlot
}

type agentTrackSlot struct {
	active     bool
	dead       bool
	target     trackedAgentState
	selectedAt int64
	deathTick  int64

	ticks         int64
	movedTicks    int
	moveSteps     int
	qiGain        float64
	qiLoss        float64
	breakthroughs int
	firstRealm    int
	lastRealm     int
}

type trackedAgentState struct {
	idx        int
	id         int
	x, y       int
	realm      int
	qi         float64
	qiMax      float64
	age        float64
	cp         float64
	aggression float64
	strategy   string
	sect       string
}

func newAgentTracker(seed int64, count int) *agentTracker {
	if count < 1 {
		count = 1
	}
	return &agentTracker{
		rng:   rand.New(rand.NewSource(seed)),
		slots: make([]agentTrackSlot, count),
	}
}

func (t *agentTracker) ensureTargets(f *engine.Frame, tick int64) {
	excluded := t.trackedIDs()
	for i := range t.slots {
		if t.slots[i].active || t.slots[i].dead {
			continue
		}
		state, ok := t.randomCultivator(f, excluded)
		if !ok {
			return
		}
		t.slots[i].start(state, tick)
		excluded[state.id] = true
	}
}

func (t *agentTracker) trackedIDs() map[int]bool {
	excluded := make(map[int]bool, len(t.slots))
	for i := range t.slots {
		if t.slots[i].active || t.slots[i].dead {
			excluded[t.slots[i].target.id] = true
		}
	}
	return excluded
}

func (t *agentTracker) observe(w *engine.World) {
	t.ensureTargets(w.Curr, w.Clock.Tick)
	for i := range t.slots {
		t.slots[i].observe(w)
	}
}

func (t *agentTracker) printReport(w *engine.World) {
	t.ensureTargets(w.Curr, w.Clock.Tick)
	if !t.hasReportableSlot() {
		fmt.Println("  追踪: 暂无存活修士")
		return
	}

	fmt.Printf("  追踪修士 (%d位)\n", len(t.slots))
	fmt.Printf("    %-2s %-7s %-9s %-4s %-5s %-11s %-14s %-8s %-7s %-13s %-34s\n",
		"#", "id", "状态", "境界", "年龄", "位置", "灵气", "战力", "攻击", "身份", "本段动作")
	for i := range t.slots {
		slot := &t.slots[i]
		if !slot.active && !slot.dead {
			continue
		}
		fmt.Printf("    %-2d %s\n", i+1, slot.reportLine())

		if slot.dead {
			slot.dead = false
			t.ensureTargets(w.Curr, w.Clock.Tick)
			if slot.active {
				fmt.Printf("       新目标: id=%d %s (%d,%d)\n",
					slot.target.id, realmNameForLevel(slot.target.realm), slot.target.x, slot.target.y)
			}
			continue
		}
		slot.start(slot.target, w.Clock.Tick)
	}
}

func (t *agentTracker) hasReportableSlot() bool {
	for i := range t.slots {
		if t.slots[i].active || t.slots[i].dead {
			return true
		}
	}
	return false
}

func (t *agentTracker) randomCultivator(f *engine.Frame, excluded map[int]bool) (trackedAgentState, bool) {
	var chosen trackedAgentState
	count := 0
	for i := range f.Agents.ID {
		if !f.Agents.Alive[i] || f.Agents.Kind[i] != "cultivator" || excluded[f.Agents.ID[i]] {
			continue
		}
		count++
		if t.rng.Intn(count) == 0 {
			chosen, _ = trackedStateByIndex(f, i)
		}
	}
	return chosen, count > 0
}

func (s *agentTrackSlot) start(state trackedAgentState, tick int64) {
	s.active = true
	s.dead = false
	s.target = state
	s.selectedAt = tick
	s.deathTick = 0
	s.ticks = 0
	s.movedTicks = 0
	s.moveSteps = 0
	s.qiGain = 0
	s.qiLoss = 0
	s.breakthroughs = 0
	s.firstRealm = state.realm
	s.lastRealm = state.realm
}

func (s *agentTrackSlot) observe(w *engine.World) {
	if !s.active {
		return
	}
	next, ok := trackedStateByIndex(w.Curr, s.target.idx)
	if !ok || next.id != s.target.id {
		s.ticks++
		s.dead = true
		s.active = false
		s.deathTick = w.Clock.Tick
		return
	}

	s.ticks++
	if next.x != s.target.x || next.y != s.target.y {
		s.movedTicks++
		s.moveSteps += toroidalChebyshevDistance(s.target.x, s.target.y, next.x, next.y, w.Config.GridWidth, w.Config.GridHeight)
	}
	qiDelta := next.qi - s.target.qi
	if qiDelta > 0 {
		s.qiGain += qiDelta
	} else if qiDelta < 0 {
		s.qiLoss -= qiDelta
	}
	if next.realm != s.target.realm {
		s.breakthroughs++
		s.lastRealm = next.realm
	}
	s.target = next
}

func (s *agentTrackSlot) reportLine() string {
	qiPct := 0.0
	if s.target.qiMax > 0 {
		qiPct = s.target.qi / s.target.qiMax * 100
	}
	sect := s.target.sect
	if sect == "" {
		sect = "散修"
	}
	status := "活"
	if s.dead {
		status = fmt.Sprintf("亡@%d", s.deathTick)
	}
	return fmt.Sprintf("%-7d %-9s %-4s %-5.1f (%4d,%4d) %-6.0f/%-6.0f %-5.1f%% %-8.0f %-7.3f %-13s %-34s",
		s.target.id, status, realmNameForLevel(s.target.realm), s.target.age,
		s.target.x, s.target.y, s.target.qi, s.target.qiMax, qiPct, s.target.cp,
		s.target.aggression, s.target.strategy+"/"+sect, s.actionSummary())
}

func (s *agentTrackSlot) actionSummary() string {
	summary := fmt.Sprintf("%dt 移:%d/%d 静:%d 灵:%+.1f",
		s.ticks, s.movedTicks, s.moveSteps, s.stayedTicks(), s.qiGain-s.qiLoss)
	if s.breakthroughs > 0 {
		summary += fmt.Sprintf(" 突:%s->%s", realmNameForLevel(s.firstRealm), realmNameForLevel(s.lastRealm))
	}
	if s.dead {
		summary += " 死亡"
	}
	return summary
}

func (s *agentTrackSlot) stayedTicks() int64 {
	stayed := s.ticks - int64(s.movedTicks)
	if stayed < 0 {
		return 0
	}
	return stayed
}

func trackedStateByIndex(f *engine.Frame, idx int) (trackedAgentState, bool) {
	if idx < 0 || idx >= len(f.Agents.ID) || !f.Agents.Alive[idx] || f.Agents.Kind[idx] != "cultivator" {
		return trackedAgentState{}, false
	}
	attrs := f.Agents.Attrs[idx]
	realm := int(attrs.Num["realm"])
	if realm < 1 {
		realm = 1
	}
	return trackedAgentState{
		idx:        idx,
		id:         f.Agents.ID[idx],
		x:          f.Agents.X[idx],
		y:          f.Agents.Y[idx],
		realm:      realm,
		qi:         attrs.Num["qi"],
		qiMax:      attrs.Num["qi_max"],
		age:        attrs.Num["age"],
		cp:         attrs.Num["combat_power"],
		aggression: attrs.Num["aggression"],
		strategy:   attrs.Str["strategy"],
		sect:       attrs.Str["sect"],
	}, true
}

func toroidalChebyshevDistance(x1, y1, x2, y2, width, height int) int {
	dx := toroidalAbsDelta(x1, x2, width)
	dy := toroidalAbsDelta(y1, y2, height)
	if dx > dy {
		return dx
	}
	return dy
}

func toroidalAbsDelta(from, to, size int) int {
	d := absInt(to - from)
	if size > 0 && d > size/2 {
		d = size - d
	}
	return d
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func realmNameForLevel(realm int) string {
	switch realm {
	case 1:
		return "练气"
	case 2:
		return "筑基"
	case 3:
		return "金丹"
	case 4:
		return "元婴"
	case 5:
		return "化神"
	default:
		return fmt.Sprintf("未知%d", realm)
	}
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
		if !shouldPrintNotableEvent(ev) {
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

func shouldPrintNotableEvent(ev engine.NotableEvent) bool {
	if ev.Realm == "元婴" || ev.Realm == "化神" {
		return true
	}
	return ev.Realm == "金丹" && ev.Kind == "死亡" && ev.Reason == "寿元耗尽"
}

func realmRank(realm string) int {
	switch realm {
	case "金丹":
		return 3
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
