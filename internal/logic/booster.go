package logic

import (
	"gorutin/internal/domain"
	"strings"
)

// ChooseBooster выбирает, что купить на основе текущих скилл-поинтов
func ChooseBooster(available []domain.Booster, currentStats domain.BoosterState, state *domain.GameState) (int, bool) {
	if currentStats.Points <= 0 {
		return 0, false
	}

	// 1. Радиус бомбы (приоритет)
	if currentStats.BombRange < 3 {
		if id, ok := findBooster(available, "range", currentStats.Points); ok {
			return id, true
		}
	}

	// 2. Фитиль (уменьшение задержки)
	// База 8000, -2000 за апгрейд. Минимум (после 3 апгрейдов) = 2000.
	if currentStats.BombDelay > 2000 {
		if id, ok := findBooster(available, "delay", currentStats.Points); ok {
			return id, true
		}
	}

	// 3. Количество бомб
	if currentStats.MaxBombs < 3 {
		if id, ok := findBooster(available, "bomb", currentStats.Points); ok {
			// Важно: исключаем "bomb_delay" если ищем просто "bomb"
			// Но функция findBooster ищет подстроку. 
			// "bomb" найдется в "buff_bomb" и "buff_bomb_delay".
			// Нужно уточнить поиск.
			return id, true
		}
	}

	// 4. Еще радиус
	if currentStats.BombRange < 5 {
		if id, ok := findBooster(available, "range", currentStats.Points); ok {
			return id, true
		}
	}
	
	// 5. Еще бомбы
	if currentStats.MaxBombs < 5 {
		if id, ok := findBooster(available, "bomb", currentStats.Points); ok {
			return id, true
		}
	}

	// 6. Скорость (остальное)
	if currentStats.Speed < 3 { 
		if id, ok := findBooster(available, "speed", currentStats.Points); ok {
			return id, true
		}
	}

	// Fallback
	if id, ok := findBooster(available, "range", currentStats.Points); ok { return id, true }
	if id, ok := findBooster(available, "bomb", currentStats.Points); ok { return id, true }

	return 0, false
}

func findBooster(list []domain.Booster, typeName string, budget int) (int, bool) {
	for _, b := range list {
		t := strings.ToLower(b.Type)
		
		// Special handling to avoid confusing "bomb" with "bomb_delay"
		if typeName == "bomb" && strings.Contains(t, "delay") {
			continue
		}

		if strings.Contains(t, typeName) && b.Cost <= budget {
			if b.ID != 0 {
				return b.ID, true
			}
			return mapTypeToID(b.Type), true
		}
	}
	return 0, false
}

func mapTypeToID(t string) int {
	t = strings.ToLower(t)
	switch {
	case strings.Contains(t, "delay"): return 5
	case strings.Contains(t, "bomb"): return 1
	case strings.Contains(t, "range"): return 2
	case strings.Contains(t, "speed"): return 3
	case strings.Contains(t, "armor"): return 4
	}
	return 1 
}
