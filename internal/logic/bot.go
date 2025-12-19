package logic

import (
	"gorutin/internal/domain"
	"math"
)

type Bot struct {
	State *domain.GameState
	Grid  [][]int // -1: wall/bomb, 0: empty, 1: danger
}

const (
	CellEmpty  = 0
	CellWall   = -1 // Непроходимо
	CellDanger = 2  // Опасно (зона взрыва)
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
			return &domain.UnitCommand{
				ID:   u.ID,
				Path: safePath[1:], // [0] это текущая позиция
			}
		}
	}

	// 2. Если есть бомба - ищем ящик, чтобы взорвать
	if u.BombCount > 0 {
		targetPath := b.bfs(myPos, func(p domain.Vec2d) bool {
			// Ищем соседнюю с ящиком клетку
			for _, d := range []domain.Vec2d{{0, 1}, {0, -1}, {1, 0}, {-1, 0}} {
				neighbor := domain.Vec2d{p.X() + d.X(), p.Y() + d.Y()}
				if b.isObstacle(neighbor) {
					return true
				}
			}
			return false
		})

		if len(targetPath) > 1 {
			// Если мы пришли на позицию для атаки
			if len(targetPath) == 1 { // Мы уже на месте (в теории bfs вернет [start] если start подходит)
				// Ставим бомбу
				return &domain.UnitCommand{
					ID:    u.ID,
					Bombs: []domain.Vec2d{myPos},
				}
			}
			
			// Если следующий шаг приведет к цели, и мы хотим поставить бомбу
			// Но пока просто идем к цели
			// Ограничиваем путь 1-2 шагами, так как ситуация меняется
			limit := 2
			if len(targetPath) < limit {
				limit = len(targetPath)
			}
			
			// Если мы дошли до позиции, откуда можно взорвать ящик
			// Проверим, является ли текущая позиция хорошей для установки?
			// Пока простая логика: дошли до конца найденного пути - ставим
			if len(targetPath) <= 2 {
				// Почти пришли. 
				// Можно поставить бомбу прямо здесь, если это безопасно (есть путь отхода)
				// TODO: Проверка пути отхода
				return &domain.UnitCommand{
					ID:    u.ID,
					Bombs: []domain.Vec2d{myPos}, // Пытаемся поставить
					// Path: []domain.Vec2d{targetPath[1]}, // И отойти?
				}
			}

			return &domain.UnitCommand{
				ID:   u.ID,
				Path: targetPath[1:limit],
			}
		}
	}

	// 3. Просто рандомное движение, если делать нечего (или безопасное место)
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
			if !b.isValid(next) {
				continue
			}
			// Проверяем проходимость: Стены нельзя, Опасность - нежелательно (но если спасаемся, то можно фильтровать)
			// Сейчас считаем, что ходить можно только по пустым клеткам (Grid != Wall)
			// Если мы в опасности, то мы ИЩЕМ безопасную (target func проверит Grid==Empty)
			// Но промежуточные клетки могут быть Dangerous? Лучше избегать.
			if b.Grid[next.X()][next.Y()] == CellWall {
				continue
			}
			// В бомбы ходить нельзя
			
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