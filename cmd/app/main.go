package main

import (
	"gorutin/internal/client"
	"gorutin/internal/logic"
	"log"
	"os"
	"time"
)

func main() {
	// 1. Конфигурация
	token := os.Getenv("TOKEN")
	if token == "" {
		// Для тестов можно захардкодить, но лучше через env
		token = "YOUR_TEST_TOKEN"
	}
	serverURL := "https://games-test.datsteam.dev" // Тестовый сервер

	log.Printf("Starting bot on %s...", serverURL)

	// 2. Инициализация
	api := client.NewClient(serverURL, token)
	bot := logic.NewBot()

	// 3. Игровой цикл
	// Документация говорит о дискретизации 50мс, но лимит запросов 3 в сек.
	// Ставим безопасный интервал 350-400мс.
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		// Получаем состояние
		state, err := api.GetGameState()
		if err != nil {
			log.Printf("Error getting state: %v", err)
			continue
		}

		log.Printf("Tick %d: My Units: %d", state.Tick, len(state.MyUnits))

		// Думаем
		commands := bot.CalculateTurn(state)

		// Отправляем действия
		if len(commands) > 0 {
			if err := api.SendCommands(commands); err != nil {
				log.Printf("Error sending commands: %v", err)
			} else {
				log.Printf("Sent %d commands", len(commands))
			}
		}
	}
}
