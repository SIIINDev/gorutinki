package logic

import (
	"gorutin/internal/domain"
	"strings"
)

var boosterPriority = [][]string{
	{"bomb_delay", "fuse"},
	{"bomb_range", "range"},
	{"bombs"},
	{"speed"},
	{"armor"},
	{"view", "vision"},
	{"acrobatics", "can_pass_bombs", "can_pass_obstacles", "can_pass_walls"},
	{"bombers", "soft_skills"},
}

var boosterPriorityDefense = [][]string{
	{"bomb_delay", "fuse"},
	{"bomb_range", "range"},
	{"bombs"},
	{"speed"},
	{"armor"},
	{"view", "vision"},
	{"acrobatics", "can_pass_bombs", "can_pass_obstacles", "can_pass_walls"},
	{"bombers", "soft_skills"},
}

const (
	minBombDelayMs = 2000
	maxSpeed       = 5
)

func ChooseBooster(available []domain.AvailableBooster, state domain.BoosterState, gs *domain.GameState) (int, bool) {
	if len(available) == 0 {
		return 0, false
	}

	priority := boosterPriority
	if gs != nil {
		if isDangerous(gs) {
			priority = boosterPriorityDefense
		} else if isAggressiveOpportunity(gs) {
			priority = boosterPriority
		}
	}

	points := state.Points
	for _, names := range priority {
		for i, b := range available {
			if b.Cost > points {
				continue
			}
			if matchBoosterType(b.Type, names) && isBoosterUseful(b.Type, state) {
				return i, true
			}
		}
	}

	return 0, false
}

func matchBoosterType(boosterType string, names []string) bool {
	bt := strings.ToLower(strings.TrimSpace(boosterType))
	return containsAny(bt, names)
}

func isBoosterUseful(boosterType string, state domain.BoosterState) bool {
	switch canonicalCategory(boosterType) {
	case "speed":
		return state.Speed < maxSpeed
	case "bomb_delay":
		return state.BombDelay > minBombDelayMs
	case "acrobatics":
		return !state.CanPassWalls
	default:
		return true
	}
}

func canonicalCategory(boosterType string) string {
	bt := strings.ToLower(strings.TrimSpace(boosterType))
	switch {
	case containsAny(bt, []string{"bombs", "pockets", "bomb_count"}):
		return "bombs"
	case containsAny(bt, []string{"bomb_range", "range"}):
		return "bomb_range"
	case containsAny(bt, []string{"bomb_delay", "delay", "fuse"}):
		return "bomb_delay"
	case containsAny(bt, []string{"speed"}):
		return "speed"
	case containsAny(bt, []string{"bombers", "soft_skills", "allies"}):
		return "bombers"
	case containsAny(bt, []string{"acrobatics", "parkour", "pass"}):
		return "acrobatics"
	case containsAny(bt, []string{"view", "vision"}):
		return "view"
	case containsAny(bt, []string{"armor"}):
		return "armor"
	default:
		return bt
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func isDangerous(gs *domain.GameState) bool {
	if gs == nil {
		return false
	}

	blocked := make(map[domain.Vec2d]struct{}, len(gs.Arena.Walls)+len(gs.Arena.Obstacles)+len(gs.Arena.Bombs))
	for _, p := range gs.Arena.Walls {
		blocked[p] = struct{}{}
	}
	for _, p := range gs.Arena.Obstacles {
		blocked[p] = struct{}{}
	}
	for _, b := range gs.Arena.Bombs {
		blocked[b.Pos] = struct{}{}
	}

	for _, u := range gs.MyUnits {
		if !u.Alive {
			continue
		}
		if isUnitInBombLine(u.Pos, gs.Arena.Bombs, blocked) {
			return true
		}
		for _, m := range gs.Mobs {
			if m.Pos == u.Pos {
				return true
			}
		}
	}

	return false
}

func isUnitInBombLine(pos domain.Vec2d, bombs []domain.Bomb, blocked map[domain.Vec2d]struct{}) bool {
	for _, b := range bombs {
		if pos == b.Pos {
			return true
		}
		if pos.X() == b.Pos.X() {
			step := 1
			if pos.Y() < b.Pos.Y() {
				step = -1
			}
			for y := b.Pos.Y() + step; y != pos.Y()+step; y += step {
				p := domain.Vec2d{b.Pos.X(), y}
				if _, ok := blocked[p]; ok && p != b.Pos {
					break
				}
				if p == pos && absInt(pos.Y()-b.Pos.Y()) <= b.Radius {
					return true
				}
			}
		} else if pos.Y() == b.Pos.Y() {
			step := 1
			if pos.X() < b.Pos.X() {
				step = -1
			}
			for x := b.Pos.X() + step; x != pos.X()+step; x += step {
				p := domain.Vec2d{x, b.Pos.Y()}
				if _, ok := blocked[p]; ok && p != b.Pos {
					break
				}
				if p == pos && absInt(pos.X()-b.Pos.X()) <= b.Radius {
					return true
				}
			}
		}
	}
	return false
}

func manhattan(a, b domain.Vec2d) int {
	return absInt(a.X()-b.X()) + absInt(a.Y()-b.Y())
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func isAggressiveOpportunity(gs *domain.GameState) bool {
	if gs == nil {
		return false
	}
	for _, u := range gs.MyUnits {
		if !u.Alive {
			continue
		}
		for _, e := range gs.Enemies {
			if manhattan(u.Pos, e.Pos) <= 3 {
				return true
			}
		}
	}
	return false
}
