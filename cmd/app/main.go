package main

import (
	"errors"
	"gorutin/internal/client"
	"gorutin/internal/domain"
	"gorutin/internal/logic"
	"log"
	"os"
	"strings"
	"time"
)

// loadEnv простая функция для чтения .env файла без сторонних библиотек
func loadEnv() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return // Файла нет, не страшно, надеемся на системные переменные
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
			// Убираем кавычки, если есть
			value = strings.Trim(value, `"'`)
			os.Setenv(key, value)
		}
	}
}

func main() {
	loadEnv() // Загружаем переменные из файла

	token := os.Getenv("TOKEN")
	if token == "" {
		log.Println("CRITICAL: TOKEN env var is not set!")
		log.Println("Run: $env:TOKEN='your_token'")
		// Не выходим, вдруг пользователь хочет просто проверить, что бот запускается
	}
	serverURL := "https://games-test.datsteam.dev"

	log.Printf("Starting bot on %s...", serverURL)

	api := client.NewClient(serverURL, token)
	bot := logic.NewBot()

	// Начальный интервал опроса (медленный, когда игры нет)
	ticker := time.NewTicker(2000 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// 1. Пытаемся получить состояние (это самый надежный способ узнать, идет ли игра)
		state, err := api.GetGameState()
		
		if err != nil {
			var serverErr *domain.ServerError
			if errors.As(err, &serverErr) {
				// Код 23: Нет активной игры
				if serverErr.ErrCode == 23 {
					log.Println("Waiting for round start... (Next round at 19:00 MSK / 16:00 UTC)")
					ticker.Reset(5000 * time.Millisecond) // Ждем 5 сек
					continue
				}
				// Код 1: Нет токена
				if serverErr.ErrCode == 1 {
					log.Fatal("ERROR: Invalid or missing TOKEN. Please check your environment variable.")
				}
			}
			
			// Другая ошибка
			log.Printf("API Error: %v", err)
			continue
		}

		// Если ошибок нет, значит игра идет!
		// Ускоряем опрос
		ticker.Reset(400 * time.Millisecond)

		log.Printf("[%s] Units: %d | Enemies: %d | Score: %d", 
			state.Round, len(state.MyUnits), len(state.Enemies), state.RawScore)

		playerCmd := bot.CalculateTurn(state)

		if playerCmd != nil && len(playerCmd.Bombers) > 0 {
			if err := api.SendCommands(*playerCmd); err != nil {
				log.Printf("Error sending commands: %v", err)
			} else {
				// log.Printf("Sent commands for %d units", len(playerCmd.Bombers))
			}
		}
	}
}