package cultivation

import "github.com/runmin/sugarscape/engine"

const (
	moveTargetActiveKey = "move_target_active"
	moveTargetXKey      = "move_target_x"
	moveTargetYKey      = "move_target_y"
)

// SetAgentMoveTarget gives a living cultivator an explicit movement target.
func SetAgentMoveTarget(agents *engine.AgentStore, agentID, x, y, gridW, gridH int) bool {
	idx, ok := agentIndexByID(agents, agentID)
	if !ok {
		return false
	}
	attrs := &agents.Attrs[idx]
	attrs.Num[moveTargetActiveKey] = 1
	attrs.Num[moveTargetXKey] = float64(normalizeCoord(x, gridW))
	attrs.Num[moveTargetYKey] = float64(normalizeCoord(y, gridH))
	return true
}

// ClearAgentMoveTarget removes an explicit movement target from a cultivator.
func ClearAgentMoveTarget(agents *engine.AgentStore, agentID int) bool {
	idx, ok := agentIndexByID(agents, agentID)
	if !ok {
		return false
	}
	clearMoveTarget(&agents.Attrs[idx])
	return true
}

// MoveTargetFor returns the explicit movement target stored on an agent.
func MoveTargetFor(attrs engine.AttrBag) (int, int, bool) {
	if attrs.Num[moveTargetActiveKey] < 0.5 {
		return 0, 0, false
	}
	return int(attrs.Num[moveTargetXKey]), int(attrs.Num[moveTargetYKey]), true
}

func hasActiveMoveTarget(attrs engine.AttrBag, x, y int) bool {
	tx, ty, ok := MoveTargetFor(attrs)
	return ok && (tx != x || ty != y)
}

func clearMoveTarget(attrs *engine.AttrBag) {
	delete(attrs.Num, moveTargetActiveKey)
	delete(attrs.Num, moveTargetXKey)
	delete(attrs.Num, moveTargetYKey)
}

func agentIndexByID(agents *engine.AgentStore, agentID int) (int, bool) {
	for i := range agents.ID {
		if agents.ID[i] == agentID && agents.Alive[i] && agents.Kind[i] == "cultivator" {
			return i, true
		}
	}
	return 0, false
}

func normalizeCoord(v, size int) int {
	if size <= 0 {
		return 0
	}
	v %= size
	if v < 0 {
		v += size
	}
	return v
}
