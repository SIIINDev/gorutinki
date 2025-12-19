package main

import (
	"errors"
	"fmt"
	"gorutin/internal/client"
	"gorutin/internal/domain"
	"gorutin/internal/logic"
	"gorutin/internal/ui"
	"gorutin/internal/viz"
	"log"
	"os"
	"strings"
	"time"
)

// loadEnv простая функция для чтения .env файла без сторонних библиотек
func loadEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return // Файла нет, не страшно
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, `"'`)
			os.Setenv(key, value)
		}
	}
}

func main() {
	loadEnv()

	token := os.Getenv("TOKEN")
	if token == "" {
		log.Println("CRITICAL: TOKEN env var is not set!")
		log.Println("Run: $env:TOKEN='your_token' or create .env file")
	}
	serverURL := "https://games-test.datsteam.dev"

	log.Printf("Starting bot on %s...", serverURL)

	api := client.NewClient(serverURL, token)
	bot := logic.NewBot()

	// Запускаем сервер визуализации
	vizServer := viz.NewServer()
	vizServer.Start(":8080")
	log.Println("Visualization started on http://localhost:8080")

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	
	const boosterCheckInterval = 2 * time.Second
	lastBoosterCheck := time.Time{}
	lastBoosterLog := time.Time{}
	var currentBoosters *domain.BoosterState
	unitStats := &unitTracker{}
	skillPoints := &skillPointTracker{}

	for range ticker.C {
		// 1. Пытаемся получить состояние
		state, err := api.GetGameState()
		
		if err != nil {
			// ...
			var serverErr *domain.ServerError
			if errors.As(err, &serverErr) {
				if serverErr.ErrCode == 23 {
					checkRoundsSchedule(api, ticker)
					continue
				}
				if serverErr.ErrCode == 1 {
					log.Fatal("ERROR: Invalid or missing TOKEN.")
				}
			}
			log.Printf("API Error: %v", err)
			continue
		}

		skillPoints.updateRound(state.Round, time.Now())
		if time.Since(lastBoosterLog) > 10*time.Second {
			lastBoosterLog = time.Now()
			boosters, err := api.GetAvailableBoosters()
			if err == nil {
				currentBoosters = &boosters.State
				s := boosters.State
				log.Printf("[BOOSTS] Points: %d | Speed: %d | Range: %d | Bombs: %d/%d | Armor: %d | View: %d",
					s.Points, s.Speed, s.BombRange, s.MaxBombs, s.Bombers, s.Armor, s.View)
			}
		}
		remainingPoints := skillPoints.remainingPoints(time.Now())
		if remainingPoints > 0 && time.Since(lastBoosterCheck) > boosterCheckInterval {
			lastBoosterCheck = time.Now()
			if spent, ok, boosters := tryUpgradeUnit(api, bot, state, remainingPoints); ok {
				skillPoints.spend(spent)
				if boosters != nil {
					currentBoosters = boosters
				}
			}
		}

		// 3. Логика игры
		unitStats.update(state)
		log.Printf("[%s] Units: %d | Enemies: %d | Score: %d", 
			state.Round, len(state.MyUnits), len(state.Enemies), state.RawScore)
		log.Printf("[ENEMIES] %s", formatEnemies(state.Enemies))
		log.Printf("[MOBS] %s", formatMobs(state.Mobs))
		for _, u := range state.MyUnits {
			steps := unitStats.steps[u.ID]
			log.Printf("[UNIT] id=%s alive=%t pos=%d,%d bombs=%d safe=%d steps=%d",
				u.ID, u.Alive, u.Pos.X(), u.Pos.Y(), u.BombCount, u.SafeTime, steps)
		}

		playerCmd := bot.CalculateTurn(state)

		// Обновляем данные для браузера
		vizServer.Update(state, bot.GetGrid(), currentBoosters)

		if playerCmd != nil && len(playerCmd.Bombers) > 0 {
			if err := api.SendCommands(*playerCmd); err != nil {
				log.Printf("Error sending commands: %v", err)
			}
		}
	}
}

func tryUpgradeUnit(api *client.DatsClient, bot *logic.Bot, state *domain.GameState, maxSpend int) (int, bool, *domain.BoosterState) {
	boosters, err := api.GetAvailableBoosters()
	if err != nil {
		log.Printf("Error getting boosters: %v", err)
		return 0, false, nil
	}
	if boosters == nil {
		return 0, false, nil
	}
	s := boosters.State
	log.Printf("[BOOSTS] Points: %d | Speed: %d | Range: %d | Bombs: %d | Bombers: %d | Armor: %d | View: %d",
		s.Points, s.Speed, s.BombRange, s.MaxBombs, s.Bombers, s.Armor, s.View)
	bot.UpdateBoosterState(s)
	if len(boosters.Available) == 0 {
		return 0, false, &boosters.State
	}
	filtered, mapIdx := filterBoostersByCost(boosters.Available, s.Points, maxSpend)
	if len(filtered) == 0 {
		return 0, false, &boosters.State
	}
	boosterID, ok := logic.ChooseBooster(filtered, s, state)
	if !ok {
		return 0, false, &boosters.State
	}
	originIdx := mapIdx[boosterID]
	if err := api.ActivateBooster(originIdx); err != nil {
		log.Printf("Error activating booster: %v", err)
		return 0, false, &boosters.State
	}
	if originIdx >= 0 && originIdx < len(boosters.Available) {
		ui.RecordBoosterPurchase(boosters.Available[originIdx].Type)
	}
	cost := 1
	if originIdx >= 0 && originIdx < len(boosters.Available) {
		cost = boosters.Available[originIdx].Cost
	}
	log.Printf("Activated booster id=%d", originIdx)
	return cost, true, &boosters.State
}

func checkRoundsSchedule(api *client.DatsClient, ticker *time.Ticker) {
	rounds, err := api.GetRounds()
	if err != nil {
		log.Printf("No active game. Waiting... (Error getting rounds: %v)", err)
		ticker.Reset(5 * time.Second)
		return
	}

	var activeRound *domain.RoundResponse
	var nextRound *domain.RoundResponse
	now := time.Now().UTC()

	for i := range rounds.Rounds {
		r := &rounds.Rounds[i]
		
		// Парсим время начала (формат RFC3339)
		startAt, _ := time.Parse(time.RFC3339, r.StartAt)
		// endAt нам пока не нужен для логики

		if r.Status == "active" {
			activeRound = r
			break
		}
		
		// Ищем ближайший будущий раунд
		if startAt.After(now) {
			if nextRound == nil {
				nextRound = r
			} else {
				// Если этот раунд раньше, чем уже найденный nextRound
				nextStart, _ := time.Parse(time.RFC3339, nextRound.StartAt)
				if startAt.Before(nextStart) {
					nextRound = r
				}
			}
		}
	}

	if activeRound != nil {
		log.Printf("Round '%s' is ACTIVE! Connecting...", activeRound.Name)
		ticker.Reset(100 * time.Millisecond) // Сразу пробуем подключиться
	} else if nextRound != nil {
		startAt, _ := time.Parse(time.RFC3339, nextRound.StartAt)
		wait := time.Until(startAt)
		log.Printf("No active round. Next round '%s' starts in %v (%s)", nextRound.Name, wait.Round(time.Second), startAt.Format("15:04:05 UTC"))
		
		// Если ждать долго, замедляем опрос
		if wait > 10*time.Second {
			ticker.Reset(5 * time.Second)
		} else {
			ticker.Reset(1 * time.Second)
		}
	} else {
		log.Println("No active game and no future rounds found. Waiting...")
		ticker.Reset(10 * time.Second)
	}
}

type unitTracker struct {
	round   string
	lastPos map[string]domain.Vec2d
	steps   map[string]int
}

func (t *unitTracker) update(state *domain.GameState) {
	if state == nil {
		return
	}
	if t.round != state.Round {
		t.round = state.Round
		t.lastPos = make(map[string]domain.Vec2d, len(state.MyUnits))
		t.steps = make(map[string]int, len(state.MyUnits))
		for _, u := range state.MyUnits {
			t.lastPos[u.ID] = u.Pos
			t.steps[u.ID] = 0
		}
		return
	}
	if t.lastPos == nil {
		t.lastPos = make(map[string]domain.Vec2d, len(state.MyUnits))
	}
	if t.steps == nil {
		t.steps = make(map[string]int, len(state.MyUnits))
	}
	for _, u := range state.MyUnits {
		if prev, ok := t.lastPos[u.ID]; ok {
			t.steps[u.ID] += absInt(u.Pos.X()-prev.X()) + absInt(u.Pos.Y()-prev.Y())
		}
		t.lastPos[u.ID] = u.Pos
	}
}

func formatEnemies(enemies []domain.EnemyUnit) string {
	if len(enemies) == 0 {
		return "none"
	}
	var b strings.Builder
	for i, e := range enemies {
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "id=%s pos=%d,%d safe=%d", e.ID, e.Pos.X(), e.Pos.Y(), e.SafeTime)
	}
	return b.String()
}

func formatMobs(mobs []domain.Mob) string {
	if len(mobs) == 0 {
		return "none"
	}
	var b strings.Builder
	for i, m := range mobs {
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "id=%s type=%s pos=%d,%d safe=%d", m.ID, m.Type, m.Pos.X(), m.Pos.Y(), m.SafeTime)
	}
	return b.String()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

type skillPointTracker struct {
	round string
	start time.Time
	spent int
}

func (t *skillPointTracker) updateRound(round string, now time.Time) {
	if round == "" {
		return
	}
	if t.round != round {
		t.round = round
		t.start = now
		t.spent = 0
	}
}

func (t *skillPointTracker) earnedPoints(now time.Time) int {
	if t.start.IsZero() {
		return 0
	}
	elapsed := now.Sub(t.start)
	points := int(elapsed / (90 * time.Second))
	if points > 10 {
		points = 10
	}
	if points < 0 {
		points = 0
	}
	return points
}

func (t *skillPointTracker) remainingPoints(now time.Time) int {
	earned := t.earnedPoints(now)
	remaining := earned - t.spent
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (t *skillPointTracker) spend(points int) {
	if points <= 0 {
		return
	}
	t.spent += points
	if t.spent > 10 {
		t.spent = 10
	}
}

func filterBoostersByCost(available []domain.AvailableBooster, points int, maxSpend int) ([]domain.AvailableBooster, []int) {
	filtered := make([]domain.AvailableBooster, 0, len(available))
	mapIdx := make([]int, 0, len(available))
	for i, b := range available {
		if b.Cost > points || b.Cost > maxSpend {
			continue
		}
		filtered = append(filtered, b)
		mapIdx = append(mapIdx, i)
	}
	return filtered, mapIdx
}
