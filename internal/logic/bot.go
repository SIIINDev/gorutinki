package logic

import (
	"gorutin/internal/domain"
	"math/rand"
)

type Bot struct {
	State *domain.GameState
}

func NewBot() *Bot {
	return &Bot{}
}

// CalculateTurn возвращает список команд для всех юнитов
func (b *Bot) CalculateTurn(state *domain.GameState) []domain.Command {
	b.State = state
	var commands []domain.Command

	for _, unit := range state.MyUnits {
		cmd := b.processUnit(unit)
		if cmd != nil {
			commands = append(commands, *cmd)
		}
	}

	return commands
}

// processUnit принимает решение для одного конкретного юнита
func (b *Bot) processUnit(u domain.Unit) *domain.Command {
	// 1. Проверяем, не стоим ли мы в опасности (на бомбе)
	if b.isDanger(u.Pos) {
		safePos := b.findSafeSpot(u.Pos)
		if safePos != nil {
			return &domain.Command{
				UnitID: u.ID,
				Move:   []domain.Vec2d{*safePos},
			}
		}
	}

	// 2. Простая стратегия: двигаемся рандомно, но безопасно
	// В будущем здесь будет A* pathfinding к ближайшему ящику
	possibleMoves := []domain.Vec2d{
		{X: u.Pos.X + 1, Y: u.Pos.Y},
		{X: u.Pos.X - 1, Y: u.Pos.Y},
		{X: u.Pos.X, Y: u.Pos.Y + 1},
		{X: u.Pos.X, Y: u.Pos.Y - 1},
	}

	var validMoves []domain.Vec2d
	for _, m := range possibleMoves {
		if b.isValidMove(m) && !b.isDanger(m) {
			validMoves = append(validMoves, m)
		}
	}

	if len(validMoves) > 0 {
		target := validMoves[rand.Intn(len(validMoves))]
		
		// Шанс 10% поставить бомбу, если идем
		var bombs []domain.Vec2d
		if rand.Float32() < 0.1 && u.BombCount > 0 {
			bombs = append(bombs, u.Pos) // Ставим под себя и убегаем
		}

		return &domain.Command{
			UnitID: u.ID,
			Move:   []domain.Vec2d{target},
			Bomb:   bombs,
		}
	}

	return nil // Стоим на месте
}

// isDanger проверяет, находится ли клетка в радиусе взрыва любой бомбы
func (b *Bot) isDanger(pos domain.Vec2d) bool {
	for _, bomb := range b.State.Bombs {
		// Простая проверка креста
		if (pos.X == bomb.Pos.X && abs(pos.Y-bomb.Pos.Y) <= bomb.Radius) ||
		   (pos.Y == bomb.Pos.Y && abs(pos.X-bomb.Pos.X) <= bomb.Radius) {
			return true
		}
	}
	return false
}

// isValidMove проверяет границы карты и стены
func (b *Bot) isValidMove(pos domain.Vec2d) bool {
	// Границы карты (предположим 0..Size)
	if pos.X < 0 || pos.Y < 0 || pos.X >= b.State.MapSize.X || pos.Y >= b.State.MapSize.Y {
		return false
	}

	// Стены
	for _, w := range b.State.Walls {
		if w == pos {
			return false
		}
	}
	
	// Другие бомбы (нельзя ходить сквозь них без перка)
	for _, bomb := range b.State.Bombs {
		if bomb.Pos == pos {
			return false
		}
	}

	return true
}

// findSafeSpot ищет ближайшую безопасную клетку (BFS)
func (b *Bot) findSafeSpot(start domain.Vec2d) *domain.Vec2d {
	// Тут можно реализовать поиск в ширину на 2-3 хода
	// Для примера просто вернем nil, чтобы бот не усложнялся пока
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
