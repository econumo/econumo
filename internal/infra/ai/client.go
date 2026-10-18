// Package ai supplies OpenAI-compatible chat completions and Anthropic
// Messages for AI features. JSON can be requested in the prompt rather than
// through vendor-specific response formats or tools.
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

// Anthropic requires an explicit output budget.
const anthropicMaxTokens = 8192

type Config struct {
	Dialect  string // "anthropic" for Messages; empty, "openai", or "econumo" for chat completions
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
	hc := &http.Client{Timeout: 90 * time.Second}
	if cfg.Dialect == "anthropic" {
		// net/http does not strip x-api-key on cross-host redirects as it
		// does Authorization. Keep this credential at the configured endpoint.
		hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &Client{cfg: cfg, http: hc}
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

type anthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []message `json:"messages"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

// Complete sends one system+user exchange on behalf of userID and returns the
// assistant text; userID only matters when Config.Sign is set.
// Errors carry the HTTP status only: the response body may echo the prompt
// (the user's payee strings) or the key, and errors end up in logs.
func (c *Client) Complete(ctx context.Context, userID, system, user string) (string, error) {
	path := "/chat/completions"
	var payload any = request{Model: c.cfg.Model, Messages: []message{{Role: "system", Content: system}, {Role: "user", Content: user}}}
	if c.cfg.Dialect == "anthropic" {
		path = "/messages"
		payload = anthropicRequest{Model: c.cfg.Model, MaxTokens: anthropicMaxTokens, System: system, Messages: []message{{Role: "user", Content: user}}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.Endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Dialect == "anthropic" {
		req.Header.Set("anthropic-version", "2023-06-01")
		if c.cfg.APIKey != "" {
			req.Header.Set("x-api-key", c.cfg.APIKey)
		}
	} else {
		bearer := c.cfg.APIKey
		if c.cfg.Sign != nil {
			if bearer, err = c.cfg.Sign(userID); err != nil {
				return "", fmt.Errorf("ai: signing the request: %w", err)
			}
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
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
	if c.cfg.Dialect == "anthropic" {
		return parseAnthropicResponse(raw)
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

func parseAnthropicResponse(raw []byte) (string, error) {
	var out anthropicResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", errors.New("ai: malformed completion response")
	}
	switch out.StopReason {
	case "", "end_turn", "stop_sequence":
		// Some compatible gateways omit stop_reason.
	default:
		// A token limit, refusal, or tool request is not a finished suggestion.
		return "", errors.New("ai: incomplete completion response")
	}
	var content strings.Builder
	for _, block := range out.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	if strings.TrimSpace(content.String()) == "" {
		return "", errors.New("ai: completion returned no text")
	}
	return content.String(), nil
}
