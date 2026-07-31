package mcpparity_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/test/apiparity"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

// TestReadonlyBlocksEveryTool pins a deliberate product decision: a read-only
// caller loses the MCP endpoint entirely, including read tools whose REST
// equivalents (GET) keep working. /mcp sits behind the same middleware.Auth as
// REST and every JSON-RPC call is a POST, so the read-only rule catches it with
// no MCP-specific code — and because the rule is path-based it cannot see which
// tool the body names. Reversing this means gating per-tool on the level in the
// request context; until that decision is made, this test freezes the current
// behavior so a future allowlist edit cannot quietly re-open the endpoint.
// See docs/superpowers/specs/2026-07-19-cloud-monetization-trial-access-design.md.
func TestReadonlyBlocksEveryTool(t *testing.T) {
	db := dbtest.NewSQLite(t)
	h := apiparity.NewHarness(t, db)
	f := fixture.New(t, db)

	userID := f.User(fixture.User{Email: "readonly-mcp@example.test", AccessLevel: "readonly"})
	token := "eco_ses_u" + userID + "000000"
	exp := apiparity.ClockTime.Add(appuser.SessionTTL)
	f.AccessToken(fixture.AccessToken{
		UserID: userID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(token), UserAgent: "readonly-mcp", ExpiresAt: &exp,
	})

	// A pure read and a write: both must be refused, which is the asymmetry
	// against REST (where the read's GET equivalent stays available).
	for _, tool := range []string{"list_accounts", "create_transaction"} {
		t.Run(tool, func(t *testing.T) {
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"` + tool + `","arguments":{}}}`
			req, err := http.NewRequest(http.MethodPost, h.URL()+"/mcp", bytes.NewReader([]byte(body)))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(resp.Body)

			if resp.StatusCode != http.StatusPaymentRequired {
				t.Fatalf("status = %d, want 402; body = %s", resp.StatusCode, raw)
			}
			// The 402 must arrive as the standard handled-error envelope, not a
			// JSON-RPC error: it is refused at the middleware, before the MCP
			// handler ever sees the request.
			var env struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
				Code    int    `json:"code"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("unmarshal envelope: %v (body = %s)", err, raw)
			}
			if env.Success || env.Code != http.StatusPaymentRequired {
				t.Fatalf("envelope = %+v, want success=false code=402; body = %s", env, raw)
			}
			if env.Message != "Read-only access. Write operations are disabled." {
				t.Fatalf("message = %q (frozen envelope text changed)", env.Message)
			}
		})
	}
}

// TestFullAccessReachesTools is the counterweight: the same call on a
// full-access caller must succeed, so the test above cannot pass for the wrong
// reason (a broken fixture, a bad token, or an unmounted endpoint would 401/404
// rather than 402, but a silent regression to "MCP is always blocked" would
// otherwise go unnoticed).
func TestFullAccessReachesTools(t *testing.T) {
	db := dbtest.NewSQLite(t)
	h := apiparity.NewHarness(t, db)
	f := fixture.New(t, db)

	userID := f.User(fixture.User{Email: "full-mcp@example.test"})
	token := "eco_ses_u" + userID + "000000"
	exp := apiparity.ClockTime.Add(appuser.SessionTTL)
	f.AccessToken(fixture.AccessToken{
		UserID: userID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(token), UserAgent: "full-mcp", ExpiresAt: &exp,
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_accounts","arguments":{}}}`
	req, err := http.NewRequest(http.MethodPost, h.URL()+"/mcp", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, raw)
	}
}
