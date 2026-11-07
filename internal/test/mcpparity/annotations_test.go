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

// destructiveTools are the tools that delete data or overwrite it in bulk, so
// an MCP client must prompt before every call. Everything else that writes is
// additive or undoable in the app (a create, a rename, an archive that
// unarchives, a single-row update), and must say destructiveHint:false
// explicitly — see the comment on webmcp.annotate.
var destructiveTools = map[string]bool{
	"add_budget_account":       true, // turning savings off deletes the row's plans and comments
	"bulk_update_transactions": true, // overwrites the classification of up to 100 rows at once
	"delete_budget_comment":    true,
	"delete_transaction":       true,
	"merge_category":           true, // merges move every reference, then DELETE the source
	"merge_label":              true,
	"merge_payee":              true,
	"merge_tag":                true,
	"remove_budget_account":    true, // deletes the savings row's plans and comments
}

// TestEveryToolIsAnnotated guards the Claude connector-directory review
// criteria: every tool must carry a title and the applicable hint, and the
// hints drive auto-permissions in the client (a read-only tool runs without
// asking, a destructive one always prompts). The hazard this catches is
// silent: DestructiveHint defaults to TRUE in the MCP spec whenever
// ReadOnlyHint is false, so a new write tool registered with a bare
// &sdk.Tool{} would start prompting on every call and would be flagged at
// submission. Register tools through the webmcp.{ReadOnly,Write,Destructive}Tool
// helpers and this passes by construction.
func TestEveryToolIsAnnotated(t *testing.T) {
	db := dbtest.NewSQLite(t)
	h := apiparity.NewHarness(t, db)
	f := fixture.New(t, db)

	userID := f.User(fixture.User{Email: "annotations-mcp@example.test"})
	token := "eco_ses_u" + userID + "000000"
	exp := apiparity.ClockTime.Add(appuser.SessionTTL)
	f.AccessToken(fixture.AccessToken{
		UserID: userID, Kind: model.TokenKindSession,
		TokenHash: appuser.HashAccessToken(token), UserAgent: "annotations-mcp", ExpiresAt: &exp,
	})

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
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

	var env struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Annotations *struct {
					Title           string `json:"title"`
					ReadOnlyHint    bool   `json:"readOnlyHint"`
					DestructiveHint *bool  `json:"destructiveHint"`
					OpenWorldHint   *bool  `json:"openWorldHint"`
				} `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode tools/list: %v; body = %s", err, raw)
	}
	tools := env.Result.Tools
	if len(tools) == 0 {
		t.Fatal("tools/list returned no tools")
	}

	seenDestructive := map[string]bool{}
	for _, tl := range tools {
		if tl.Annotations == nil {
			t.Errorf("%s: no annotations; register it with a webmcp.*Tool helper", tl.Name)
			continue
		}
		// Anthropic reads the title; the spec allows two places for it and
		// resolves title -> annotations.title -> name, so both must agree.
		if tl.Title == "" || tl.Annotations.Title == "" {
			t.Errorf("%s: missing title (title=%q annotations.title=%q)", tl.Name, tl.Title, tl.Annotations.Title)
		}
		if tl.Title != tl.Annotations.Title {
			t.Errorf("%s: title %q != annotations.title %q", tl.Name, tl.Title, tl.Annotations.Title)
		}
		if tl.Description == "" {
			t.Errorf("%s: missing description", tl.Name)
		}
		if len(tl.Name) > 64 {
			t.Errorf("%s: name is %d chars, the directory limit is 64", tl.Name, len(tl.Name))
		}
		// Every tool's domain is the caller's own ledger; none reaches out.
		if tl.Annotations.OpenWorldHint == nil || *tl.Annotations.OpenWorldHint {
			t.Errorf("%s: openWorldHint must be false", tl.Name)
		}

		switch {
		case tl.Annotations.ReadOnlyHint:
			if destructiveTools[tl.Name] {
				t.Errorf("%s: listed as destructive but declares readOnlyHint", tl.Name)
			}
			// Meaningful only when ReadOnlyHint is false; leaving it out keeps
			// the read-only contract unambiguous.
			if tl.Annotations.DestructiveHint != nil {
				t.Errorf("%s: read-only tool must not set destructiveHint", tl.Name)
			}
		case tl.Annotations.DestructiveHint == nil:
			t.Errorf("%s: write tool leaves destructiveHint unset, which the MCP spec "+
				"reads as TRUE, so every call would prompt", tl.Name)
		case *tl.Annotations.DestructiveHint != destructiveTools[tl.Name]:
			t.Errorf("%s: destructiveHint = %v, want %v (update destructiveTools here if "+
				"the tool's effect really changed)", tl.Name, *tl.Annotations.DestructiveHint, destructiveTools[tl.Name])
		}
		if destructiveTools[tl.Name] {
			seenDestructive[tl.Name] = true
		}
	}

	for name := range destructiveTools {
		if !seenDestructive[name] {
			t.Errorf("destructiveTools names %q, which tools/list does not expose; "+
				"drop it here if the tool is gone", name)
		}
	}
}
