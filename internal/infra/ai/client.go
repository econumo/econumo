// Package ai supplies OpenAI-compatible chat completions and Anthropic
// Messages for AI features. With Config.JSONSchema a call may carry a JSON
// schema the reply must follow; endpoints that reject schemas get the prompt
// alone, which still asks for the JSON.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
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
	// JSONSchema sends each call's reply schema (the DSN's json=1). Off,
	// schemas are never sent and the prompt alone asks for the JSON.
	JSONSchema bool
	// Sign, when set, supplies the bearer per call from the requesting
	// user's id, replacing APIKey. The econumo:// dialect signs a
	// econumo-ai:v1 token here so the AI gateway can attribute usage.
	Sign func(userID string) (string, error)
}

// Schema is the JSON schema a reply must follow. It must fit the providers'
// structured-output subset: every object closed (additionalProperties false)
// with all its properties required, optional values nullable.
type Schema struct {
	Name string
	JSON json.RawMessage
}

// Check reports where the schema leaves the structured-output subset, so a
// test can guard each one: a provider refuses such a schema with a 400 and the
// call loses its schema for the process lifetime.
func (s Schema) Check() error {
	var root any
	if err := json.Unmarshal(s.JSON, &root); err != nil {
		return fmt.Errorf("%s: %w", s.Name, err)
	}
	return checkNode(s.Name, root)
}

func checkNode(path string, node any) error {
	m, ok := node.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: not an object", path)
	}
	for _, k := range []string{"minLength", "maxLength", "minimum", "maximum", "minItems", "maxItems", "pattern", "format"} {
		if _, has := m[k]; has {
			return fmt.Errorf("%s: %s is not supported", path, k)
		}
	}
	if props, has := m["properties"].(map[string]any); has || m["type"] == "object" {
		if m["additionalProperties"] != false {
			return fmt.Errorf("%s: additionalProperties must be false", path)
		}
		required, _ := m["required"].([]any)
		if len(required) != len(props) {
			return fmt.Errorf("%s: every property must be required", path)
		}
		for _, r := range required {
			name, _ := r.(string)
			if _, ok := props[name]; !ok {
				return fmt.Errorf("%s: required %q is not a property", path, name)
			}
		}
		for name, p := range props {
			if err := checkNode(path+"."+name, p); err != nil {
				return err
			}
		}
	}
	if items, has := m["items"]; has {
		return checkNode(path+"[]", items)
	}
	return nil
}

type Client struct {
	cfg  Config
	http *http.Client
	// noSchema is set once the endpoint answered 400 to a request carrying a
	// schema (local servers, older models): later calls send none.
	noSchema atomic.Bool
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
	Model          string          `json:"model"`
	Messages       []message       `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type response struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

type anthropicRequest struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system,omitempty"`
	Messages     []message     `json:"messages"`
	OutputConfig *outputConfig `json:"output_config,omitempty"`
}

type outputConfig struct {
	Format outputFormat `json:"format"`
}

type outputFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

// Complete sends one system+user exchange on behalf of userID and returns the
// assistant text; userID only matters when Config.Sign is set. With
// Config.JSONSchema, a non-nil schema asks the provider to force the reply
// into it; when the endpoint
// answers that with a 400 the call is retried once without it, and the
// client sends no schema from then on.
// Errors carry the HTTP status only: the response body may echo the prompt
// (the user's payee strings) or the key, and errors end up in logs.
func (c *Client) Complete(ctx context.Context, userID, system, user string, schema *Schema) (string, error) {
	if schema != nil && (!c.cfg.JSONSchema || c.noSchema.Load()) {
		schema = nil
	}
	raw, status, err := c.send(ctx, userID, system, user, schema)
	if err == nil && status == http.StatusBadRequest && schema != nil {
		if c.noSchema.CompareAndSwap(false, true) {
			slog.Info("ai endpoint rejects reply schemas, sending prompts alone", "dialect", c.cfg.Dialect, "schema", schema.Name)
		}
		raw, status, err = c.send(ctx, userID, system, user, nil)
	}
	if err != nil {
		return "", err
	}
	if status/100 != 2 {
		return "", fmt.Errorf("ai: completion endpoint returned HTTP %d", status)
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

func (c *Client) send(ctx context.Context, userID, system, user string, schema *Schema) ([]byte, int, error) {
	path := "/chat/completions"
	var payload any
	if c.cfg.Dialect == "anthropic" {
		path = "/messages"
		r := anthropicRequest{Model: c.cfg.Model, MaxTokens: anthropicMaxTokens, System: system, Messages: []message{{Role: "user", Content: user}}}
		if schema != nil {
			r.OutputConfig = &outputConfig{Format: outputFormat{Type: "json_schema", Schema: schema.JSON}}
		}
		payload = r
	} else {
		r := request{Model: c.cfg.Model, Messages: []message{{Role: "system", Content: system}, {Role: "user", Content: user}}}
		if schema != nil {
			r.ResponseFormat = &responseFormat{Type: "json_schema", JSONSchema: jsonSchema{Name: schema.Name, Strict: true, Schema: schema.JSON}}
		}
		payload = r
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.Endpoint, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
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
				return nil, 0, fmt.Errorf("ai: signing the request: %w", err)
			}
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("ai: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("ai: reading response: %w", err)
	}
	return raw, resp.StatusCode, nil
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
