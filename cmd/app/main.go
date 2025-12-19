package main

import (
	"gorutin/internal/client"
	"gorutin/internal/logic"
	"log"
	"os"
	"time"
)

func main() {
	token := os.Getenv("TOKEN")
	if token == "" {
		log.Println("WARNING: TOKEN env var is not set. Using empty token.")
	}
	// Используем URL тестового сервера из openapi.json
	serverURL := "https://games-test.datsteam.dev" 

	log.Printf("Starting bot on %s...", serverURL)

	api := client.NewClient(serverURL, token)
	bot := logic.NewBot()

	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		state, err := api.GetGameState()
		if err != nil {
			log.Printf("Error getting state: %v", err)
			continue
		}

		log.Printf("Round: %s | Units: %d | Enemies: %d | Mobs: %d", 
			state.Round, len(state.MyUnits), len(state.Enemies), len(state.Mobs))

		// Если раунда нет или он завершен, просто ждем
		if state.Round == "" {
			continue
		}

		playerCmd := bot.CalculateTurn(state)

		if playerCmd != nil && len(playerCmd.Bombers) > 0 {
			if err := api.SendCommands(*playerCmd); err != nil {
				log.Printf("Error sending commands: %v", err)
			} else {
				log.Printf("Sent commands for %d units", len(playerCmd.Bombers))
			}
		}
	}
}