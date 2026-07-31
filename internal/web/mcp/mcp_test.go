package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	webmcp "github.com/econumo/econumo/internal/web/mcp"
)

func rpc(t *testing.T, url string, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("non-JSON response %q: %v", raw, err)
		}
	}
	return resp.StatusCode, out
}

type pingIn struct {
	Msg string `json:"msg" jsonschema:"message to echo"`
}
type pingOut struct {
	Echo string `json:"echo"`
}

func TestNewHandler_InitializeAndToolError(t *testing.T) {
	register := webmcp.Compose(func(s *sdk.Server) {
		sdk.AddTool(s, &sdk.Tool{Name: "ping", Description: "echo"},
			func(ctx context.Context, req *sdk.CallToolRequest, in pingIn) (*sdk.CallToolResult, pingOut, error) {
				if in.Msg == "boom-domain" {
					return nil, pingOut{}, webmcp.MapErr(ctx, errs.NewValidation("Category name must be 3-64 characters"))
				}
				if in.Msg == "boom-infra" {
					return nil, pingOut{}, webmcp.MapErr(ctx, errors.New("pq: connection refused on 10.0.0.5"))
				}
				return nil, pingOut{Echo: in.Msg}, nil
			})
	})
	ts := httptest.NewServer(webmcp.NewHandler(register))
	defer ts.Close()

	status, out := rpc(t, ts.URL, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if status != 200 {
		t.Fatalf("initialize status %d", status)
	}
	if !bytes.Contains(mustJSON(t, out), []byte(`"name":"econumo"`)) {
		t.Fatalf("serverInfo missing econumo: %v", out)
	}

	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ping","arguments":{"msg":"hi"}}}`)
	if !bytes.Contains(mustJSON(t, out), []byte(`"echo":"hi"`)) {
		t.Fatalf("tool call: %v", out)
	}

	// Domain error: isError tool result carrying the exact message.
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ping","arguments":{"msg":"boom-domain"}}}`)
	if s := string(mustJSON(t, out)); !strings.Contains(s, `"isError":true`) || !strings.Contains(s, "Category name must be 3-64 characters") {
		t.Fatalf("domain error: %s", s)
	}

	// Infra error: isError with the sanitized message only.
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ping","arguments":{"msg":"boom-infra"}}}`)
	if s := string(mustJSON(t, out)); !strings.Contains(s, "Internal error") || strings.Contains(s, "10.0.0.5") {
		t.Fatalf("infra error leaked or missing: %s", s)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestUserIDMissing(t *testing.T) {
	if _, err := webmcp.UserID(context.Background()); err == nil {
		t.Fatal("want error on missing user")
	}
	_ = vo.Id{}
}

func TestPromptsListAndGet(t *testing.T) {
	ts := httptest.NewServer(webmcp.NewHandler(webmcp.Compose()))
	defer ts.Close()

	// Test: prompts/list returns both names
	_, out := rpc(t, ts.URL, `{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":{}}`)
	outJSON := string(mustJSON(t, out))
	if !strings.Contains(outJSON, "log-expense") {
		t.Fatalf("prompts/list missing log-expense: %s", outJSON)
	}
	if !strings.Contains(outJSON, "budget-review") {
		t.Fatalf("prompts/list missing budget-review: %s", outJSON)
	}

	// Test: prompts/get log-expense with description contains both description and create_transaction
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"log-expense","arguments":{"description":"5 coffee"}}}`)
	outJSON = string(mustJSON(t, out))
	if !strings.Contains(outJSON, "5 coffee") {
		t.Fatalf("log-expense prompt missing description: %s", outJSON)
	}
	if !strings.Contains(outJSON, "create_transaction") {
		t.Fatalf("log-expense prompt missing create_transaction: %s", outJSON)
	}

	// Test: prompts/get budget-review without arguments contains get_budget and "the current month"
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":3,"method":"prompts/get","params":{"name":"budget-review","arguments":{}}}`)
	outJSON = string(mustJSON(t, out))
	if !strings.Contains(outJSON, "get_budget") {
		t.Fatalf("budget-review prompt missing get_budget: %s", outJSON)
	}
	if !strings.Contains(outJSON, "the current month") {
		t.Fatalf("budget-review prompt missing 'the current month': %s", outJSON)
	}
}

// TestBudgetStructurePrompts covers the two prompts that build and reconcile a
// budget's structure. Beyond rendering, it asserts each names the tools it tells
// the model to call: a prompt citing a tool that no longer exists still renders
// fine and only fails in front of a user.
func TestBudgetStructurePrompts(t *testing.T) {
	ts := httptest.NewServer(webmcp.NewHandler(webmcp.Compose()))
	defer ts.Close()

	_, out := rpc(t, ts.URL, `{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":{}}`)
	listJSON := string(mustJSON(t, out))
	for _, name := range []string{"budget-setup", "budget-update"} {
		if !strings.Contains(listJSON, name) {
			t.Fatalf("prompts/list missing %s: %s", name, listJSON)
		}
	}

	// budget-setup with no name: must still render, and must drive the full
	// create path (survey -> folders -> envelopes -> limits).
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"budget-setup","arguments":{}}}`)
	setupJSON := string(mustJSON(t, out))
	for _, want := range []string{
		"list_categories", "list_tags", "list_transactions",
		"create_budget", "create_folder", "create_envelope", "set_limit", "get_budget",
		"Base expenses", "Additional expenses",
	} {
		if !strings.Contains(setupJSON, want) {
			t.Errorf("budget-setup prompt missing %q", want)
		}
	}

	// The supplied name must reach the rendered text.
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":3,"method":"prompts/get","params":{"name":"budget-setup","arguments":{"name":"Household 2027"}}}`)
	if named := string(mustJSON(t, out)); !strings.Contains(named, "Household 2027") {
		t.Errorf("budget-setup prompt dropped the supplied name: %s", named)
	}

	// budget-update defaults the month and must warn about update_envelope's
	// replace-the-whole-set semantics, the one footgun in the reconcile path.
	_, out = rpc(t, ts.URL, `{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"budget-update","arguments":{}}}`)
	updateJSON := string(mustJSON(t, out))
	for _, want := range []string{
		"the current month", "get_budget", "update_envelope", "set_limit", "category_ids",
	} {
		if !strings.Contains(updateJSON, want) {
			t.Errorf("budget-update prompt missing %q", want)
		}
	}
}
