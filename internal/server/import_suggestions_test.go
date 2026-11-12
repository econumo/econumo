package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

func TestImportSuggestionsEndToEnd(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Name: "Me"})
	exp := time.Now().UTC().Add(24 * time.Hour)
	full := "eco_pat_import-suggestions-test-full-00000000000000"
	ingest := "eco_pat_import-suggestions-test-ingest-000000000000"
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", TokenHash: appuser.HashAccessToken(full), ExpiresAt: &exp})
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", Scope: "ingest", TokenHash: appuser.HashAccessToken(ingest), ExpiresAt: &exp})
	account := f.Account(fixture.Account{UserID: user, CurrencyID: "dffc2a06-6f29-4704-8575-31709adee926", Name: "Card"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	f.ImportAccountLink(fixture.ImportAccountLink{SourceID: src, ExternalAccountID: "Apple Card", AccountID: account})
	cat := f.Category(fixture.Category{UserID: user, Name: "Coffee"})

	ai := &recordingCompleter{reply: func() (string, error) {
		return `{"rows":{"0":{"matchText":"Blue Bottle","categoryId":"` + cat + `"}}}`, nil
	}}
	ts := httptest.NewServer(server.BuildAPI(baseTestConfig(db.Engine), db.Raw, server.Seams{
		Avatars:          appuser.FixedAvatarPicker(appuser.DefaultAvatar),
		ImportsCompleter: ai,
		ImportsJobRunner: func(job func()) { job() },
	}))
	t.Cleanup(ts.Close)

	do := func(token, method, path string, body any) (int, map[string]any) {
		t.Helper()
		var r io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			r = strings.NewReader(string(raw))
		}
		req, _ := http.NewRequest(method, ts.URL+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		var env map[string]any
		if err := json.Unmarshal(b, &env); err != nil {
			t.Fatalf("%s: %s", path, b)
		}
		if data, ok := env["data"].(map[string]any); ok && resp.StatusCode == http.StatusOK {
			return resp.StatusCode, data
		}
		return resp.StatusCode, env
	}

	occurred := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	status, res := do(ingest, http.MethodPost, "/api/v1/import/ingest-apple-wallet-event", map[string]any{
		"account": "Apple Card", "payee": "Blue Bottle", "amount": "4.75", "currency": "USD", "occurredAt": occurred, "eventId": "evt-1",
	})
	if status != http.StatusOK || res["status"] != "created" {
		t.Fatalf("ingest: %d %v", status, res)
	}
	if len(ai.prompts) != 1 {
		t.Fatalf("AI calls = %d", len(ai.prompts))
	}

	status, list := do(full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)
	items, _ := list["items"].([]any)
	if status != http.StatusOK || len(items) != 1 {
		t.Fatalf("list: %d %v", status, list)
	}
	item := items[0].(map[string]any)
	txID := item["transactions"].([]any)[0].(map[string]any)["transactionId"].(string)
	if item["categoryId"] != cat || item["matchField"] != "external_payee" {
		t.Fatalf("item = %v", item)
	}

	ruleID := vo.NewId().String()
	status, res = do(full, http.MethodPost, "/api/v1/import/create-rule", map[string]any{
		"id": ruleID, "action": "classify", "matchField": "external_payee", "matchType": "contains", "matchValue": item["matchText"], "categoryId": cat,
	})
	if status != http.StatusOK {
		t.Fatalf("create-rule: %d %v", status, res)
	}
	status, res = do(full, http.MethodPost, "/api/v1/import/accept-suggestion", map[string]any{"id": item["id"], "ruleId": ruleID})
	if status != http.StatusOK || res["updated"] != float64(1) {
		t.Fatalf("accept: %d %v", status, res)
	}
	var got string
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COALESCE(category_id, '') FROM transactions WHERE id = ?`), txID).Scan(&got); err != nil || got != cat {
		t.Fatalf("category = %q, %v", got, err)
	}
	status, list = do(full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)
	if items, _ = list["items"].([]any); status != http.StatusOK || len(items) != 0 {
		t.Fatalf("list after accept: %d %v", status, list)
	}
}
