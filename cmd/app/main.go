package main

import (
	"errors"
	"gorutin/internal/client"
	"gorutin/internal/domain"
	"gorutin/internal/logic"
	"gorutin/internal/viz"
	// "gorutin/internal/ui"
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

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	
	lastBoosterLog := time.Time{}

	for range ticker.C {
		// 1. Пытаемся получить состояние
		state, err := api.GetGameState()
		
		if err != nil {
			// ... (обработка ошибок остается прежней)
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

		// 2. Раз в 10 секунд выводим статистику бустов
		if time.Since(lastBoosterLog) > 10*time.Second {
			lastBoosterLog = time.Now()
			boosters, err := api.GetAvailableBoosters()
			if err == nil {
				s := boosters.State
				log.Printf("[BOOSTS] Points: %d | Speed: %d | Range: %d | Bombs: %d/%d | Armor: %d | View: %d", 
					s.Points, s.Speed, s.BombRange, s.MaxBombs, s.Bombers, s.Armor, s.View)
			}
		}

		// 3. Логика игры
		log.Printf("[%s] Units: %d | Enemies: %d | Score: %d", 
			state.Round, len(state.MyUnits), len(state.Enemies), state.RawScore)

		// Бустеры (пока закомментировано, так как логика выбора еще не реализована полностью)
		/*
		if time.Since(lastBoosterCheck) > 5*time.Second {
			lastBoosterCheck = time.Now()
			boosters, err := api.GetAvailableBoosters()
			if err != nil {
				log.Printf("Error getting boosters: %v", err)
			} else {
				// TODO: Реализовать функцию ChooseBooster в logic
			}
		}
		*/

		playerCmd := bot.CalculateTurn(state)

		// Обновляем данные для браузера
		vizServer.Update(state, bot.GetGrid())

		// Визуализация (раскомментируйте, когда захотите видеть карту)
		// ui.Draw(state, bot.GetGrid())

		if playerCmd != nil && len(playerCmd.Bombers) > 0 {
			if err := api.SendCommands(*playerCmd); err != nil {
				log.Printf("Error sending commands: %v", err)
			}
		}
	}
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