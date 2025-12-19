package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gorutin/internal/domain"
	"net/http"
	"time"
)

type DatsClient struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

func NewClient(url, token string) *DatsClient {
	return &DatsClient{
		BaseURL: url,
		Token:   token,
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

// GetGameState получает текущее состояние мира
func (c *DatsClient) GetGameState() (*domain.GameState, error) {
	// TODO: Проверьте точный эндпоинт в Swagger
	req, err := http.NewRequest("GET", c.BaseURL+"/play/zombidef/units", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var state domain.GameState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return nil, err
	}

	return &state, nil
}

// SendCommands отправляет команды для юнитов
func (c *DatsClient) SendCommands(cmds []domain.Command) error {
	if len(cmds) == 0 {
		return nil
	}

	data, err := json.Marshal(struct {
		Commands []domain.Command `json:"commands"`
	}{Commands: cmds})
	if err != nil {
		return err
	}

	// TODO: Проверьте точный эндпоинт отправки команд
	req, err := http.NewRequest("POST", c.BaseURL+"/play/zombidef/command", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	return nil
}
