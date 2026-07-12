// Package mcpparity freezes the MCP endpoint's wire behavior with golden
// files, mirroring apiparity: same harness, same normalization, sqlite
// goldens in the smoke tier and a build-tagged sqlite-vs-pgsql comparison.
package mcpparity

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/econumo/econumo/internal/test/apiparity"
)

type Step struct {
	Label string
	// REST seeding step (Method != ""): replayed via the apiparity harness.
	Method, Path string
	Body         any
	// MCP step (RPC != ""): the JSON-RPC request body posted to /mcp.
	RPC string
	// NoAuth sends the MCP request without a bearer token.
	NoAuth bool
	// CaptureID extracts data.item.id from a REST step's response for
	// fmt.Sprintf-style substitution (%s) into later RPC bodies.
	CaptureID bool
}

type Scenario struct {
	Name  string
	Steps []Step
}

var catalogue []Scenario

func register(s Scenario) { catalogue = append(catalogue, s) }

func Catalogue() []Scenario { return catalogue }

// stepResult is one step's raw (un-normalized) outcome, kept internal so Run
// (golden-normalized, dates redacted) and the engine-comparison test
// (parity-normalized, dates kept) can each apply their own normalization to
// the same underlying bytes.
type stepResult struct {
	Label, Method, Path string
	Status              int
	Body                []byte
}

// runSteps replays a scenario's steps against one harness and returns each
// step's raw status/body, in order.
func runSteps(t *testing.T, h *apiparity.Harness, s Scenario) []stepResult {
	t.Helper()
	var out []stepResult
	var captured string
	for _, st := range s.Steps {
		if st.Method != "" {
			status, body := h.Call(t, st.Method, st.Path, apiparity.OwnerToken, st.Body)
			if st.CaptureID {
				captured = extractItemID(t, body)
			}
			out = append(out, stepResult{Label: st.Label, Method: st.Method, Path: st.Path, Status: status, Body: body})
			continue
		}
		rpcBody := st.RPC
		if strings.Contains(rpcBody, "%s") {
			rpcBody = fmt.Sprintf(rpcBody, captured)
		}
		token := apiparity.OwnerToken
		if st.NoAuth {
			token = ""
		}
		status, body := postMCP(t, h.URL(), token, rpcBody)
		out = append(out, stepResult{Label: st.Label, Method: "POST", Path: "/mcp", Status: status, Body: body})
	}
	return out
}

// Run replays a scenario and returns one normalized transcript block per step.
func Run(t *testing.T, h *apiparity.Harness, s Scenario) []string {
	t.Helper()
	var out []string
	for _, r := range runSteps(t, h, s) {
		out = append(out, fmt.Sprintf("== %s %s %s -> %d\n%s", r.Label, r.Method, r.Path, r.Status, apiparity.NormalizeGolden(r.Body)))
	}
	return out
}

func postMCP(t *testing.T, baseURL, token, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, raw
}

// extractItemID pulls "data.item.id" out of a create-endpoint response, like
// apiparity's own (unexported) helper — but t.Fatal's on absence rather than
// returning "", since a mcpparity scenario step that captures an id always
// needs it for a later substitution: silently reusing "" would produce a
// confusing downstream failure instead of pinpointing the seeding call.
func extractItemID(t *testing.T, body []byte) string {
	t.Helper()
	var env struct {
		Data struct {
			Item struct {
				Id string `json:"id"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Data.Item.Id == "" {
		t.Fatalf("extractItemID: no data.item.id in %s", body)
	}
	return env.Data.Item.Id
}
