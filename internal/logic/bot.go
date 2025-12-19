package logic

import (
	"gorutin/internal/domain"
)

type Bot struct {
	State     *domain.GameState
	Grid      [][]int // -1: wall/bomb, 0: empty, 1: danger
	BombRange int
	Speed     int
}

const (
	CellEmpty  = 0
	CellWall   = -1 // Непроходимо
	CellDanger = 2  // Опасно (зона взрыва)
	MaxPathLen = 30
)

func NewBot() *Bot {
	return &Bot{BombRange: 1, Speed: 2}
}

func (b *Bot) UpdateBoosterState(state domain.BoosterState) {
	if state.BombRange > 0 {
		b.BombRange = state.BombRange
	}
	if state.Speed > 0 {
		b.Speed = state.Speed
	}
}

func (b *Bot) CalculateTurn(state *domain.GameState) *domain.PlayerCommand {
	b.State = state
	b.buildGrid()

	var unitCmds []domain.UnitCommand

	for _, unit := range state.MyUnits {
		if !unit.Alive {
			continue
		}
		cmd := b.processUnit(unit)
		if cmd != nil {
			unitCmds = append(unitCmds, *cmd)
		}
	}

	if len(unitCmds) == 0 {
		return nil
	}

	return &domain.PlayerCommand{Bombers: unitCmds}
}

// buildGrid строит карту опасностей и препятствий
func (b *Bot) buildGrid() {
	width, height := b.State.MapSize.X(), b.State.MapSize.Y()
	b.Grid = make([][]int, width)
	for x := 0; x < width; x++ {
		b.Grid[x] = make([]int, height)
	}

	// 1. Стены и препятствия
	for _, w := range b.State.Arena.Walls {
		b.setGrid(w, CellWall)
	}
	for _, o := range b.State.Arena.Obstacles {
		b.setGrid(o, CellWall) // Пока считаем ящики стенами, чтобы не врезаться
	}

	// 2. Бомбы (как препятствия) + Зоны взрыва
	for _, bomb := range b.State.Arena.Bombs {
		b.setGrid(bomb.Pos, CellWall)
		b.markExplosionZone(bomb)
	}
	
	// 3. Другие юниты тоже препятствия (чтобы не застревать)
	for _, enemy := range b.State.Enemies {
		b.setGrid(enemy.Pos, CellWall)
	}
	for _, mob := range b.State.Mobs {
		b.setGrid(mob.Pos, CellWall) // От мобов лучше держаться подальше
		// Можно добавить радиус опасности вокруг моба
	}
}

func (b *Bot) setGrid(p domain.Vec2d, val int) {
	if b.isValid(p) {
		// Если клетка уже помечена как стена, не переписываем на опасность (стена важнее)
		if b.Grid[p.X()][p.Y()] == CellWall && val != CellWall {
			return
		}
		b.Grid[p.X()][p.Y()] = val
	}
}

func (b *Bot) markExplosionZone(bomb domain.Bomb) {
	// Крест взрыва
	dirs := []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}}
	b.setGrid(bomb.Pos, CellDanger) // Сама бомба опасна

	for _, d := range dirs {
		for i := 1; i <= bomb.Radius; i++ {
			pos := domain.Vec2d{bomb.Pos.X() + d.X()*i, bomb.Pos.Y() + d.Y()*i}
			if !b.isValid(pos) {
				break
			}
			// Если стена - взрыв дальше не идет
			// ВАЖНО: Тут нужно точнее проверять стены. В текущей модели стены и ящики в Grid=-1.
			// По правилам ящики останавливают взрыв, но сами взрываются.
			// Для простоты пока считаем, что зона за стеной безопасна.
			if b.Grid[pos.X()][pos.Y()] == CellWall {
				// Если это ящик (есть в obstacle list), то он опасен (взорвется), но луч дальше не идет
				if b.isObstacle(pos) {
					b.setGrid(pos, CellDanger)
				}
				break
			}
			b.setGrid(pos, CellDanger)
		}
	}
}

func (b *Bot) processUnit(u domain.Unit) *domain.UnitCommand {
	myPos := u.Pos

	// 1. Если мы в опасности - бежим!
	if b.Grid[myPos.X()][myPos.Y()] == CellDanger {
		safePath := b.bfs(myPos, func(p domain.Vec2d) bool {
			return b.Grid[p.X()][p.Y()] == CellEmpty
		})
		if len(safePath) > 1 {
			if len(safePath)-1 > MaxPathLen {
				safePath = safePath[:MaxPathLen+1]
			}
			return &domain.UnitCommand{
				ID:   u.ID,
				Path: safePath[1:], // [0] это текущая позиция
			}
		}
	}

	// 2. Пытаемся добраться до клетки рядом с препятствием.
	// Если есть бомбы - ставим и сразу отходим.
	wantBomb := u.BombCount > 0
	path, bombPos := b.findAttackPlan(myPos, wantBomb, b.BombRange, u.ID)
	if len(path) > 1 {
		if len(path)-1 > MaxPathLen {
			return nil
		}
		cmd := domain.UnitCommand{
			ID:   u.ID,
			Path: path[1:],
		}
		if bombPos != nil {
			cmd.Bombs = []domain.Vec2d{*bombPos}
		}
		return &cmd
	}

	// 3. Если делать нечего - не двигаемся.
	return nil
}

// BFS поиск пути
func (b *Bot) bfs(start domain.Vec2d, isTarget func(domain.Vec2d) bool) []domain.Vec2d {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d) // came_from
	visited[start] = domain.Vec2d{-1, -1}

	if isTarget(start) {
		return []domain.Vec2d{start}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if isTarget(current) {
			// Восстанавливаем путь
			path := []domain.Vec2d{}
			curr := current
			for curr != (domain.Vec2d{-1, -1}) {
				path = append([]domain.Vec2d{curr}, path...)
				curr = visited[curr]
			}
			return path
		}

		// Соседи
		neighbors := []domain.Vec2d{
			{current.X() + 1, current.Y()},
			{current.X() - 1, current.Y()},
			{current.X(), current.Y() + 1},
			{current.X(), current.Y() - 1},
		}

		for _, next := range neighbors {
			if !b.isWalkable(next) {
				continue
			}
			
			if _, seen := visited[next]; !seen {
				visited[next] = current
				queue = append(queue, next)
			}
		}
	}
	return nil
}

func (b *Bot) isValid(p domain.Vec2d) bool {
	return p.X() >= 0 && p.Y() >= 0 && p.X() < b.State.MapSize.X() && p.Y() < b.State.MapSize.Y()
}

func (b *Bot) isObstacle(p domain.Vec2d) bool {
	for _, o := range b.State.Arena.Obstacles {
		if o == p {
			return true
		}
	}
	return false
}

func (b *Bot) isWalkable(p domain.Vec2d) bool {
	if !b.isValid(p) {
		return false
	}
	return b.Grid[p.X()][p.Y()] == CellEmpty
}

func (b *Bot) findAttackPlan(start domain.Vec2d, wantBomb bool, bombRange int, selfID string) ([]domain.Vec2d, *domain.Vec2d) {
	queue := []domain.Vec2d{start}
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if b.hasAdjacentObstacle(current) {
			if wantBomb && current == start {
				goto expand
			}

			path := b.reconstructPath(current, visited)
			if !wantBomb {
				if len(path)-1 <= MaxPathLen {
					return path, nil
				}
			} else {
				if b.hasFriendlyInBlast(current, bombRange, selfID) {
					goto expand
				}
				escapePath := b.findEscapePath(current, bombRange)
				if len(escapePath) > 1 {
					path = append(path, escapePath[1:]...)
					if len(path)-1 <= MaxPathLen {
						bombPos := current
						return path, &bombPos
					}
				}
			}
		}

	expand:
		for _, next := range b.neighbors(current) {
			if !b.isWalkable(next) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				queue = append(queue, next)
			}
		}
	}

	return nil, nil
}

func (b *Bot) hasAdjacentObstacle(p domain.Vec2d) bool {
	for _, n := range b.neighbors(p) {
		if b.isObstacle(n) {
			return true
		}
	}
	return false
}

func (b *Bot) neighbors(p domain.Vec2d) []domain.Vec2d {
	return []domain.Vec2d{
		{p.X() + 1, p.Y()},
		{p.X() - 1, p.Y()},
		{p.X(), p.Y() + 1},
		{p.X(), p.Y() - 1},
	}
}

func (b *Bot) reconstructPath(target domain.Vec2d, visited map[domain.Vec2d]domain.Vec2d) []domain.Vec2d {
	path := []domain.Vec2d{}
	curr := target
	for curr != (domain.Vec2d{-1, -1}) {
		path = append([]domain.Vec2d{curr}, path...)
		curr = visited[curr]
	}
	return path
}

func (b *Bot) findEscapePath(start domain.Vec2d, bombRange int) []domain.Vec2d {
	queue := []domain.Vec2d{start}
	steps := make(map[domain.Vec2d]int)
	visited := make(map[domain.Vec2d]domain.Vec2d)
	visited[start] = domain.Vec2d{-1, -1}
	steps[start] = 0

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		currentSteps := steps[current]
		arrivalSec := b.stepsToSeconds(currentSteps)
		if !b.isInBombLine(current, start, bombRange) && !b.isUnsafeDueToBombs(current, arrivalSec, start) {
			return b.reconstructPath(current, visited)
		}

		for _, next := range b.neighbors(current) {
			nextSteps := currentSteps + 1
			arrivalSec = b.stepsToSeconds(nextSteps)
			if !b.isWalkableTimed(next, arrivalSec, start) {
				continue
			}
			if _, seen := visited[next]; !seen {
				visited[next] = current
				steps[next] = nextSteps
				queue = append(queue, next)
			}
		}
	}

	return nil
}

func (b *Bot) hasFriendlyInBlast(bombPos domain.Vec2d, bombRange int, selfID string) bool {
	for _, u := range b.State.MyUnits {
		if !u.Alive || u.ID == selfID {
			continue
		}
		if b.isInBombLine(u.Pos, bombPos, bombRange) {
			return true
		}
	}
	return false
}

func (b *Bot) isInBombLine(pos domain.Vec2d, bombPos domain.Vec2d, bombRange int) bool {
	if pos == bombPos {
		return true
	}
	if pos.X() == bombPos.X() {
		if absIntBot(pos.Y()-bombPos.Y()) > bombRange {
			return false
		}
		return b.isLineClear(bombPos, pos)
	}
	if pos.Y() == bombPos.Y() {
		if absIntBot(pos.X()-bombPos.X()) > bombRange {
			return false
		}
		return b.isLineClear(bombPos, pos)
	}
	return false
}

func (b *Bot) isLineClear(a domain.Vec2d, c domain.Vec2d) bool {
	if a.X() == c.X() {
		step := 1
		if c.Y() < a.Y() {
			step = -1
		}
		for y := a.Y() + step; y != c.Y(); y += step {
			if b.isBlocked(domain.Vec2d{a.X(), y}) {
				return false
			}
		}
		return true
	}
	if a.Y() == c.Y() {
		step := 1
		if c.X() < a.X() {
			step = -1
		}
		for x := a.X() + step; x != c.X(); x += step {
			if b.isBlocked(domain.Vec2d{x, a.Y()}) {
				return false
			}
		}
		return true
	}
	return false
}

func (b *Bot) isBlocked(p domain.Vec2d) bool {
	for _, w := range b.State.Arena.Walls {
		if w == p {
			return true
		}
	}
	for _, o := range b.State.Arena.Obstacles {
		if o == p {
			return true
		}
	}
	for _, bm := range b.State.Arena.Bombs {
		if bm.Pos == p {
			return true
		}
	}
	return false
}

func absIntBot(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (b *Bot) stepsToSeconds(steps int) float64 {
	speed := b.Speed
	if speed <= 0 {
		speed = 2
	}
	return float64(steps) / float64(speed)
}

func (b *Bot) isWalkableTimed(p domain.Vec2d, arrivalSec float64, ownBombPos domain.Vec2d) bool {
	if !b.isValid(p) {
		return false
	}
	if b.Grid[p.X()][p.Y()] == CellWall {
		return false
	}
	return !b.isUnsafeDueToBombs(p, arrivalSec, ownBombPos)
}

func (b *Bot) isUnsafeDueToBombs(p domain.Vec2d, arrivalSec float64, ownBombPos domain.Vec2d) bool {
	for _, bm := range b.State.Arena.Bombs {
		if bm.Pos == ownBombPos {
			continue
		}
		if b.isInBombLine(p, bm.Pos, bm.Radius) && bm.Timer > arrivalSec {
			return true
		}
	}
	return false
}
