package ui

import (
	"fmt"
	"gorutin/internal/domain"
	//"strings"
)

// Symbols - набор символов для отрисовки
const (
	SymEmpty   = ".."
	SymWall    = "##"
	SymBox     = "[]"
	SymBomb    = "()"
	SymMe      = "ME"
	SymEnemy   = "E "
	SymMob     = "M "
	SymDanger  = "XX" // Зона взрыва (в режиме дебага)
)

func Draw(state *domain.GameState, debugGrid [][]int) {
	// Очистка экрана
	fmt.Print("\033[H\033[2J")

	if state == nil {
		return
	}

	w, h := state.MapSize.X(), state.MapSize.Y()
	
	// 1. Создаем и заполняем глобальную сетку объектами
	grid := make([][]string, w)
	for x := 0; x < w; x++ {
		grid[x] = make([]string, h)
		for y := 0; y < h; y++ {
			grid[x][y] = SymEmpty
		}
	}

	// Стены и препятствия
	for _, p := range state.Arena.Walls { set(grid, p, SymWall) }
	for _, p := range state.Arena.Obstacles { set(grid, p, SymBox) }

	// Опасные зоны
	if debugGrid != nil {
		for x := 0; x < w; x++ {
			for y := 0; y < h; y++ {
				if debugGrid[x][y] == 2 && grid[x][y] == SymEmpty {
					grid[x][y] = SymDanger
				}
			}
		}
	}

	// Динамические объекты
	for _, b := range state.Arena.Bombs { set(grid, b.Pos, SymBomb) }
	for _, u := range state.Enemies { set(grid, u.Pos, SymEnemy) }
	for _, m := range state.Mobs { set(grid, m.Pos, SymMob) }
	
	// Свои юниты
	for _, u := range state.MyUnits {
		if u.Alive {
			set(grid, u.Pos, SymMe)
		}
	}

	// 2. Выводим информацию
	fmt.Printf("Round: %s | Score: %d | Alive Units: %d\n", state.Round, state.RawScore, len(state.MyUnits))
	fmt.Println("------------------------------------------------")

	// 3. Рисуем вид для каждого юнита
	radius := 7 // Радиус обзора для отрисовки

	for _, u := range state.MyUnits {
		if !u.Alive {
			continue
		}

		fmt.Printf("Unit %s [Pos: %d,%d, Bombs: %d]\n", u.ID, u.Pos.X(), u.Pos.Y(), u.BombCount)
		
		// Рамка сверху
		for k := 0; k < (radius*2+1)+2; k++ { fmt.Print("=") }
		fmt.Println()

		for dy := -radius; dy <= radius; dy++ {
			y := u.Pos.Y() + dy
			fmt.Print("|")
			for dx := -radius; dx <= radius; dx++ {
				x := u.Pos.X() + dx

				// Проверка границ карты
				if x < 0 || y < 0 || x >= w || y >= h {
					fmt.Print("  ") // За границей карты пусто
				} else {
					fmt.Print(grid[x][y])
				}
			}
			fmt.Println("|")
		}
		
		// Рамка снизу
		for k := 0; k < (radius*2+1)+2; k++ { fmt.Print("=") }
		fmt.Println()
		fmt.Println() // Отступ между юнитами
	}
}

func set(grid [][]string, p domain.Vec2d, s string) {
	if p.X() >= 0 && p.X() < len(grid) && p.Y() >= 0 && p.Y() < len(grid[0]) {
		grid[p.X()][p.Y()] = s
	}
}
