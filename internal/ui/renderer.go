package ui

import (
	"fmt"
	"gorutin/internal/domain"
	"strings"
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

type roundTracker struct {
	round           string
	prevEnemies     map[string]struct{}
	prevMobs        map[string]struct{}
	prevObstacles   map[domain.Vec2d]struct{}
	prevUnitPos     map[string]domain.Vec2d
	killsEnemies    int
	killsMobs       int
	blocksDestroyed int
	distance        int
	boosters        []string
}

var tracker roundTracker

// RecordBoosterPurchase stores booster info for round summary output.
func RecordBoosterPurchase(boosterType string) {
	boosterType = strings.TrimSpace(boosterType)
	if boosterType == "" {
		return
	}
	tracker.boosters = append(tracker.boosters, boosterType)
}

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
	tracker.update(state)
	fmt.Printf("Round: %s | Score: %d | Alive Units: %d\n", state.Round, state.RawScore, len(state.MyUnits))
	fmt.Printf("Boosters: %s | Kills: E:%d M:%d | Blocks: %d | Distance: %d\n",
		tracker.boosterLabel(),
		tracker.killsEnemies,
		tracker.killsMobs,
		tracker.blocksDestroyed,
		tracker.distance,
	)
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

func (t *roundTracker) boosterLabel() string {
	if len(t.boosters) == 0 {
		return "none"
	}
	return strings.Join(t.boosters, ",")
}

func (t *roundTracker) update(state *domain.GameState) {
	if state.Round == "" || t.round != state.Round {
		t.reset(state)
		return
	}

	currentEnemies := make(map[string]struct{}, len(state.Enemies))
	for _, e := range state.Enemies {
		currentEnemies[e.ID] = struct{}{}
	}
	currentMobs := make(map[string]struct{}, len(state.Mobs))
	for _, m := range state.Mobs {
		currentMobs[m.ID] = struct{}{}
	}
	currentObstacles := make(map[domain.Vec2d]struct{}, len(state.Arena.Obstacles))
	for _, o := range state.Arena.Obstacles {
		currentObstacles[o] = struct{}{}
	}
	currentUnitPos := make(map[string]domain.Vec2d, len(state.MyUnits))
	for _, u := range state.MyUnits {
		currentUnitPos[u.ID] = u.Pos
	}

	for id := range t.prevEnemies {
		if _, ok := currentEnemies[id]; !ok {
			t.killsEnemies++
		}
	}
	for id := range t.prevMobs {
		if _, ok := currentMobs[id]; !ok {
			t.killsMobs++
		}
	}
	for pos := range t.prevObstacles {
		if _, ok := currentObstacles[pos]; !ok {
			t.blocksDestroyed++
		}
	}
	for id, pos := range currentUnitPos {
		if prev, ok := t.prevUnitPos[id]; ok {
			t.distance += absIntUI(pos.X()-prev.X()) + absIntUI(pos.Y()-prev.Y())
		}
	}

	t.prevEnemies = currentEnemies
	t.prevMobs = currentMobs
	t.prevObstacles = currentObstacles
	t.prevUnitPos = currentUnitPos
}

func (t *roundTracker) reset(state *domain.GameState) {
	t.round = state.Round
	t.killsEnemies = 0
	t.killsMobs = 0
	t.blocksDestroyed = 0
	t.distance = 0
	t.boosters = nil

	t.prevEnemies = make(map[string]struct{}, len(state.Enemies))
	for _, e := range state.Enemies {
		t.prevEnemies[e.ID] = struct{}{}
	}
	t.prevMobs = make(map[string]struct{}, len(state.Mobs))
	for _, m := range state.Mobs {
		t.prevMobs[m.ID] = struct{}{}
	}
	t.prevObstacles = make(map[domain.Vec2d]struct{}, len(state.Arena.Obstacles))
	for _, o := range state.Arena.Obstacles {
		t.prevObstacles[o] = struct{}{}
	}
	t.prevUnitPos = make(map[string]domain.Vec2d, len(state.MyUnits))
	for _, u := range state.MyUnits {
		t.prevUnitPos[u.ID] = u.Pos
	}
}

func absIntUI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
