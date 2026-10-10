// Package ai is the OpenAI-compatible chat-completions client behind
// import-rule suggestions. It speaks the lowest common denominator of the
// API (model + messages + temperature, no response_format, no tools) so
// the same client works against api.openai.com, Ollama, vLLM, and gateways.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxResponseBytes bounds what a misbehaving endpoint can make us buffer.
const maxResponseBytes = 1 << 20

type Config struct {
	Endpoint string // base URL including the API prefix, e.g. https://api.openai.com/v1
	APIKey   string // empty for keyless local servers
	Model    string
	// Sign, when set, supplies the bearer per call from the requesting
	// user's id, replacing APIKey. The econumo:// dialect signs a
	// econumo-ai:v1 token here so the AI gateway can attribute usage.
	Sign func(userID string) (string, error)
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 90 * time.Second}}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Temperature is omitempty and nothing sets it: the reasoning models
// (gpt-5 family, o-series) reject any non-default temperature with a 400,
// and the body of that 400 is deliberately never logged — so sending the
// field at all would break the exact DSN .env.example documents, invisibly.
// Omitted, every server applies its own default.
type request struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

type response struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

// Complete sends one system+user exchange on behalf of userID and returns the
// assistant text; userID only matters when Config.Sign is set.
// Errors carry the HTTP status only: the response body may echo the prompt
// (the user's payee strings) or the key, and errors end up in logs.
func (c *Client) Complete(ctx context.Context, userID, system, user string) (string, error) {
	body, err := json.Marshal(request{Model: c.cfg.Model, Messages: []message{{Role: "system", Content: system}, {Role: "user", Content: user}}})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.Endpoint, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	bearer := c.cfg.APIKey
	if c.cfg.Sign != nil {
		if bearer, err = c.cfg.Sign(userID); err != nil {
			return "", fmt.Errorf("ai: signing the request: %w", err)
		}
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("ai: reading response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("ai: completion endpoint returned HTTP %d", resp.StatusCode)
	}
	var out response
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("ai: malformed completion response")
	}
	if len(out.Choices) == 0 {
		return "", errors.New("ai: completion returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}
