package logic

import (
	"gorutin/internal/domain"
)

type Bot struct {
	State *domain.GameState
	Grid  [][]int // -1: wall/bomb, 0: empty, 1: danger
}

const (
	CellEmpty  = 0
	CellWall   = -1 // Непроходимо
	CellDanger = 2  // Опасно (зона взрыва)
	MaxPathLen = 30
)

func NewBot() *Bot {
	return &Bot{}
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
	path, bombPos := b.findAttackPlan(myPos, wantBomb)
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

func (b *Bot) findAttackPlan(start domain.Vec2d, wantBomb bool) ([]domain.Vec2d, *domain.Vec2d) {
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
				escape, ok := b.findEscapeNeighbor(current)
				if ok {
					path = append(path, escape)
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

func (b *Bot) findEscapeNeighbor(p domain.Vec2d) (domain.Vec2d, bool) {
	for _, n := range b.neighbors(p) {
		if b.isWalkable(n) {
			return n, true
		}
	}
	return domain.Vec2d{}, false
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
