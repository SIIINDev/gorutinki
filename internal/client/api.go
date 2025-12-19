package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gorutin/internal/domain"
	"net/http"
	"strings"
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
		Token:   strings.TrimSpace(token), // Убираем пробелы/переносы
		Client: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

func (c *DatsClient) checkError(resp *http.Response) error {
	if resp.StatusCode == 200 {
		return nil
	}

	var apiErr domain.ServerError
	if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil {
		return &apiErr // Возвращаем типизированную ошибку
	}

	// Если не удалось распарсить JSON ошибки
	return fmt.Errorf("http status %s", resp.Status)
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

	if err := c.checkError(resp); err != nil {
		return nil, err
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

	return c.checkError(resp)
}

func (c *DatsClient) GetRounds() (*domain.RoundListResponse, error) {
	req, err := http.NewRequest("GET", c.BaseURL+"/api/rounds", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Auth-Token", c.Token)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkError(resp); err != nil {
		return nil, err
	}

	var rounds domain.RoundListResponse
	if err := json.NewDecoder(resp.Body).Decode(&rounds); err != nil {
		return nil, err
	}

	return &rounds, nil
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