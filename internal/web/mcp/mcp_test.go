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

func TestJSONTextNoHTMLEscaping(t *testing.T) {
	got, err := webmcp.JSONText(map[string]string{"a": "x<y>/z"})
	if err != nil || got != `{"a":"x<y>/z"}` {
		t.Fatalf("JSONText = %q, %v", got, err)
	}
}

func TestUserIDMissing(t *testing.T) {
	if _, err := webmcp.UserID(context.Background()); err == nil {
		t.Fatal("want error on missing user")
	}
	_ = vo.Id{}
}
