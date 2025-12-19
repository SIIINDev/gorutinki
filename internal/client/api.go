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

func (c *DatsClient) GetGameState() (*domain.GameState, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/arena", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", c.Token)

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

func (c *DatsClient) SendCommands(cmd domain.PlayerCommand) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/move", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Читаем ошибку, если есть
		// var apiErr ...
		return fmt.Errorf("bad status: %s", resp.Status)
	}
	
	return nil
}

func (c *DatsClient) GetAvailableBoosters() (*domain.AvailableBoosterResponse, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/booster", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", c.Token)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	var payload domain.AvailableBoosterResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return &payload, nil
}

func (c *DatsClient) ActivateBooster(boosterID int) error {
	body := domain.BoosterCommand{Booster: boosterID}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/api/booster", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("X-Auth-Token", c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	return nil
}
