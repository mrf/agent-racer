package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPClient makes REST calls to the Agent Racer backend.
type HTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewHTTPClient creates a client targeting the given base URL (e.g. "http://127.0.0.1:8080").
func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// GetStats fetches /api/stats.
func (c *HTTPClient) GetStats() (*Stats, error) {
	var s Stats
	if err := c.get("/api/stats", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// GetAchievements fetches /api/achievements.
func (c *HTTPClient) GetAchievements() ([]AchievementResponse, error) {
	var out []AchievementResponse
	if err := c.get("/api/achievements", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetChallenges fetches /api/challenges.
func (c *HTTPClient) GetChallenges() ([]ChallengeProgress, error) {
	var out []ChallengeProgress
	if err := c.get("/api/challenges", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetConfig fetches /api/config.
func (c *HTTPClient) GetConfig() (*SoundConfig, error) {
	var s SoundConfig
	if err := c.get("/api/config", &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Equip sends POST /api/equip.
func (c *HTTPClient) Equip(rewardID, slot string) (*Equipped, error) {
	body := map[string]string{"rewardId": rewardID, "slot": slot}
	var out Equipped
	if err := c.post("/api/equip", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Unequip sends POST /api/unequip.
func (c *HTTPClient) Unequip(slot string) (*Equipped, error) {
	body := map[string]string{"slot": slot}
	var out Equipped
	if err := c.post("/api/unequip", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// FocusSession sends POST /api/sessions/{id}/focus.
func (c *HTTPClient) FocusSession(sessionID string) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/sessions/"+sessionID+"/focus", nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("focus failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *HTTPClient) get(path string, out interface{}) error {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HTTPClient) post(path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.setAuth(req)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: %d %s", path, resp.StatusCode, string(respBody))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *HTTPClient) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
