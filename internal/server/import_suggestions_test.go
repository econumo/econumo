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

	ai := &recordingCompleter{names: dbNames(db), reply: func() (string, error) {
		return `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`, nil
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
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COALESCE(CAST(category_id AS TEXT), '') FROM transactions WHERE id = ?`), txID).Scan(&got); err != nil || got != cat {
		t.Fatalf("category = %q, %v", got, err)
	}
	status, list = do(full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)
	if items, _ = list["items"].([]any); status != http.StatusOK || len(items) != 0 {
		t.Fatalf("list after accept: %d %v", status, list)
	}
}

// update-ai off: the open suggestion disappears and later imports ask nothing.
func TestImportSuggestionsStopWhenUserOptsOutOfAI(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Name: "Me"})
	f.DefaultOptions(user)
	exp := time.Now().UTC().Add(24 * time.Hour)
	full := "eco_pat_import-suggestions-optout-full-000000000000"
	ingest := "eco_pat_import-suggestions-optout-ingest-0000000000"
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", TokenHash: appuser.HashAccessToken(full), ExpiresAt: &exp})
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", Scope: "ingest", TokenHash: appuser.HashAccessToken(ingest), ExpiresAt: &exp})
	account := f.Account(fixture.Account{UserID: user, CurrencyID: "dffc2a06-6f29-4704-8575-31709adee926", Name: "Card"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	f.ImportAccountLink(fixture.ImportAccountLink{SourceID: src, ExternalAccountID: "Apple Card", AccountID: account})
	cat := f.Category(fixture.Category{UserID: user, Name: "Coffee"})
	ai := &recordingCompleter{names: dbNames(db), reply: func() (string, error) {
		return `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + cat + `"}]}`, nil
	}}
	ts := httptest.NewServer(server.BuildAPI(baseTestConfig(db.Engine), db.Raw, server.Seams{
		Avatars:          appuser.FixedAvatarPicker(appuser.DefaultAvatar),
		ImportsCompleter: ai,
		ImportsJobRunner: func(job func()) { job() },
	}))
	t.Cleanup(ts.Close)
	do := func(token, method, path string, body any) map[string]any {
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
		if err := json.Unmarshal(b, &env); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, b)
		}
		data, _ := env["data"].(map[string]any)
		return data
	}
	tap := func(payee, id string) {
		do(ingest, http.MethodPost, "/api/v1/import/ingest-apple-wallet-event", map[string]any{
			"account": "Apple Card", "payee": payee, "amount": "4.75", "currency": "USD",
			"occurredAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), "eventId": id,
		})
	}

	tap("Blue Bottle", "evt-1")
	if items, _ := do(full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)["items"].([]any); len(ai.prompts) != 1 || len(items) != 1 {
		t.Fatalf("before opting out: calls = %d, items = %v", len(ai.prompts), items)
	}
	do(full, http.MethodPost, "/api/v1/user/update-ai", map[string]any{"enabled": false})
	var stored int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM import_suggestions WHERE user_id = ?`), user).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("turning AI off kept %d suggestions, %v", stored, err)
	}
	if items, _ := do(full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)["items"].([]any); len(items) != 0 {
		t.Fatalf("list after opting out = %v", items)
	}
	tap("Starbucks", "evt-2")
	if len(ai.prompts) != 1 {
		t.Fatalf("AI asked after opting out: %d calls", len(ai.prompts))
	}
	var open int
	if err := db.Raw.QueryRow(db.Rebind(`SELECT COUNT(*) FROM import_suggestions WHERE user_id = ?`), user).Scan(&open); err != nil || open != 0 {
		t.Fatalf("stored suggestions = %d, %v", open, err)
	}
}

// Step 1 sends no payees; step 2 sends only the candidates resembling the
// merchant step 1 named, and the suggestion carries the payee it picked.
func TestImportSuggestionsTwoStepPayee(t *testing.T) {
	db := dbtest.New(t)
	f := fixture.New(t, db)
	user := f.User(fixture.User{Name: "Me"})
	exp := time.Now().UTC().Add(24 * time.Hour)
	full := "eco_pat_import-suggestions-twostep-full-0000000000"
	ingest := "eco_pat_import-suggestions-twostep-ingest-00000000"
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", TokenHash: appuser.HashAccessToken(full), ExpiresAt: &exp})
	f.AccessToken(fixture.AccessToken{UserID: user, Kind: "personal", Scope: "ingest", TokenHash: appuser.HashAccessToken(ingest), ExpiresAt: &exp})
	account := f.Account(fixture.Account{UserID: user, CurrencyID: "dffc2a06-6f29-4704-8575-31709adee926", Name: "Card"})
	src := f.ImportSource(fixture.ImportSource{UserID: user, Name: "iPhone"})
	f.ImportAccountLink(fixture.ImportAccountLink{SourceID: src, ExternalAccountID: "Apple Card", AccountID: account})
	cat := f.Category(fixture.Category{UserID: user, Name: "Cafes"})
	f.Category(fixture.Category{UserID: user, Name: "Wages", Type: 1})
	bb := f.Payee(fixture.Payee{UserID: user, Name: "Blue Bottle Coffee"})
	f.Payee(fixture.Payee{UserID: user, Name: "Starbucks"})
	step := 0
	ai := &recordingCompleter{names: dbNames(db), reply: func() (string, error) {
		step++
		if step == 1 {
			return `{"rows":[{"key":"0","matchText":"Blue Bottle","merchant":"Blue Bottle","categoryId":"` + cat + `"}]}`, nil
		}
		return `{"rows":[{"key":"0","payeeId":"` + bb + `"}]}`, nil
	}}
	ts := httptest.NewServer(server.BuildAPI(baseTestConfig(db.Engine), db.Raw, server.Seams{
		Avatars:          appuser.FixedAvatarPicker(appuser.DefaultAvatar),
		ImportsCompleter: ai,
		ImportsJobRunner: func(job func()) { job() },
	}))
	t.Cleanup(ts.Close)
	do := func(token, method, path string, body any) map[string]any {
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
		if err := json.Unmarshal(b, &env); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: %d %s", path, resp.StatusCode, b)
		}
		data, _ := env["data"].(map[string]any)
		return data
	}
	do(ingest, http.MethodPost, "/api/v1/import/ingest-apple-wallet-event", map[string]any{
		"account": "Apple Card", "payee": "Blue Bottle", "amount": "4.75", "currency": "USD",
		"occurredAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), "eventId": "evt-1",
	})
	if len(ai.prompts) != 2 {
		t.Fatalf("AI calls = %d, want 2", len(ai.prompts))
	}
	if p := ai.prompts[0]; strings.Contains(p, "Blue Bottle Coffee") || strings.Contains(p, "Starbucks") || strings.Contains(p, "Wages") || !strings.Contains(p, "Cafes") {
		t.Fatalf("step 1 prompt = %s", p)
	}
	if p := ai.prompts[1]; strings.Contains(p, bb) || !strings.Contains(p, `{"key":"p1","name":"Blue Bottle Coffee"}`) || strings.Contains(p, "Starbucks") || strings.Contains(p, "Cafes") {
		t.Fatalf("step 2 prompt = %s", p)
	}
	items, _ := do(full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items = %v", items)
	}
	if item := items[0].(map[string]any); item["payeeId"] != bb || item["categoryId"] != cat {
		t.Fatalf("item = %v", item)
	}
}
