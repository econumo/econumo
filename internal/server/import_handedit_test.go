package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/econumo/econumo/internal/server"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/dbtest"
	"github.com/econumo/econumo/internal/test/fixture"
	appuser "github.com/econumo/econumo/internal/user"
)

type handEditHarness struct {
	t                     *testing.T
	db                    *dbtest.DB
	ts                    *httptest.Server
	full, ingest          string
	user, account, coffee string
	tapN                  int
}

// newHandEditHarness runs the real API with an AI that files every tap under
// Coffee and creates the well-known merchant's payee.
func newHandEditHarness(t *testing.T) *handEditHarness {
	t.Helper()
	db := dbtest.New(t)
	f := fixture.New(t, db)
	h := &handEditHarness{t: t, db: db,
		full:   "eco_ses_import-handedit-test-full-00000000000000000",
		ingest: "eco_pat_import-handedit-test-ingest-0000000000000000",
	}
	h.user = f.User(fixture.User{Name: "Me"})
	exp := time.Now().UTC().Add(24 * time.Hour)
	f.AccessToken(fixture.AccessToken{UserID: h.user, Kind: "session", TokenHash: appuser.HashAccessToken(h.full), ExpiresAt: &exp})
	f.AccessToken(fixture.AccessToken{UserID: h.user, Kind: "personal", Scope: "ingest", TokenHash: appuser.HashAccessToken(h.ingest), ExpiresAt: &exp})
	h.account = f.Account(fixture.Account{UserID: h.user, CurrencyID: "dffc2a06-6f29-4704-8575-31709adee926", Name: "Card"})
	src := f.ImportSource(fixture.ImportSource{UserID: h.user, Name: "iPhone"})
	f.ImportAccountLink(fixture.ImportAccountLink{SourceID: src, ExternalAccountID: "Apple Card", AccountID: h.account})
	h.coffee = f.Category(fixture.Category{UserID: h.user, Name: "Coffee"})
	ai := &recordingCompleter{names: dbNames(db), reply: func() (string, error) {
		return `{"rows":[{"key":"0","matchText":"Blue Bottle","merchants":["Blue Bottle"],"wellKnown":true,"categoryId":"` + h.coffee + `"}]}`, nil
	}}
	h.ts = httptest.NewServer(server.BuildAPI(baseTestConfig(db.Engine), db.Raw, server.Seams{
		Avatars:          appuser.FixedAvatarPicker(appuser.DefaultAvatar),
		ImportsCompleter: ai,
		ImportsJobRunner: func(job func()) { job() },
	}))
	t.Cleanup(h.ts.Close)
	return h
}

func (h *handEditHarness) do(token, method, path string, body any) map[string]any {
	h.t.Helper()
	var r io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		r = strings.NewReader(string(raw))
	}
	req, _ := http.NewRequest(method, h.ts.URL+path, r)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil || resp.StatusCode != http.StatusOK {
		h.t.Fatalf("%s: %d %s", path, resp.StatusCode, b)
	}
	data, _ := env["data"].(map[string]any)
	return data
}

func (h *handEditHarness) mcp(tool string, args map[string]any) {
	h.t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": tool, "arguments": args}})
	req, _ := http.NewRequest(http.MethodPost, h.ts.URL+"/mcp", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+h.full)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.Contains(string(b), `"isError":true`) || strings.Contains(string(b), `"error":`) {
		h.t.Fatalf("%s: %d %s", tool, resp.StatusCode, b)
	}
}

// tap ingests one Apple Pay tap and returns its transaction.
func (h *handEditHarness) tap(payee string) map[string]any {
	h.t.Helper()
	h.tapN++
	eventID := "evt-" + strconv.Itoa(h.tapN)
	res := h.do(h.ingest, http.MethodPost, "/api/v1/import/ingest-apple-wallet-event", map[string]any{
		"account": "Apple Card", "payee": payee, "amount": "4.75", "currency": "USD",
		"occurredAt": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), "eventId": eventID,
	})
	if res["status"] != "created" {
		h.t.Fatalf("ingest: %v", res)
	}
	var txID string
	if err := h.db.Raw.QueryRow(h.db.Rebind(`SELECT CAST(transaction_id AS TEXT) FROM import_transaction_links WHERE external_transaction_id = ?`), eventID).Scan(&txID); err != nil {
		h.t.Fatal(err)
	}
	return h.transaction(txID)
}

func (h *handEditHarness) transaction(id string) map[string]any {
	h.t.Helper()
	items, _ := h.do(h.full, http.MethodGet, "/api/v1/transaction/get-transaction-list?accountId="+h.account, nil)["items"].([]any)
	for _, it := range items {
		if m := it.(map[string]any); m["id"] == id {
			return m
		}
	}
	h.t.Fatalf("transaction %s not listed", id)
	return nil
}

// update sends the transaction's full field set through update-transaction,
// changed by edit.
func (h *handEditHarness) update(tx map[string]any, edit map[string]any) {
	h.t.Helper()
	body := map[string]any{
		"id": tx["id"], "type": tx["type"], "amount": tx["amount"], "accountId": tx["accountId"], "date": tx["date"],
		"categoryId": tx["categoryId"], "payeeId": tx["payeeId"], "tagId": tx["tagId"], "description": tx["description"], "labelIds": tx["labelIds"],
	}
	for k, v := range edit {
		body[k] = v
	}
	h.do(h.full, http.MethodPost, "/api/v1/transaction/update-transaction", body)
}

func (h *handEditHarness) assignedBy(txID any) string {
	h.t.Helper()
	var by *string
	if err := h.db.Raw.QueryRow(h.db.Rebind(`SELECT assigned_by FROM import_transaction_links WHERE transaction_id = ?`), txID).Scan(&by); err != nil {
		h.t.Fatal(err)
	}
	if by == nil {
		return "NULL"
	}
	return *by
}

func (h *handEditHarness) suggestedTransactions() []string {
	h.t.Helper()
	var out []string
	items, _ := h.do(h.full, http.MethodGet, "/api/v1/import/get-suggestion-list", nil)["items"].([]any)
	for _, it := range items {
		for _, row := range it.(map[string]any)["transactions"].([]any) {
			out = append(out, row.(map[string]any)["transactionId"].(string))
		}
	}
	return out
}

// A hand edit of the category through update-transaction makes the user the
// classifier: the row loses 'ai', so putting the AI's category back no longer
// keeps it under the suggestion. A description-only edit changes nothing.
func TestHandEditClearsAssignedByAI(t *testing.T) {
	h := newHandEditHarness(t)
	tx := h.tap("Blue Bottle")
	if tx["categoryId"] != h.coffee || h.assignedBy(tx["id"]) != "ai" {
		t.Fatalf("after the AI: category %v, assigned by %s", tx["categoryId"], h.assignedBy(tx["id"]))
	}

	h.update(tx, map[string]any{"description": "with a friend"})
	if got := h.assignedBy(tx["id"]); got != "ai" {
		t.Fatalf("description edit: assigned by = %s, want ai", got)
	}
	if got := h.suggestedTransactions(); len(got) != 1 || got[0] != tx["id"] {
		t.Fatalf("suggested after a description edit = %v, want the tap", got)
	}

	tea := fixture.New(t, h.db).Category(fixture.Category{UserID: h.user, Name: "Tea"})
	tx = h.transaction(tx["id"].(string))
	h.update(tx, map[string]any{"categoryId": tea})
	if got := h.assignedBy(tx["id"]); got != "NULL" {
		t.Fatalf("category edit: assigned by = %s, want NULL", got)
	}
	tx = h.transaction(tx["id"].(string))
	h.update(tx, map[string]any{"categoryId": h.coffee})
	if got := h.suggestedTransactions(); len(got) != 0 {
		t.Fatalf("suggested after the hand edit = %v, want none", got)
	}
}

// A rule's classification is cleared by a hand payee edit the same way, and
// apply-rule, an importer write, stamps 'rule' again.
func TestHandEditClearsAssignedByRule(t *testing.T) {
	h := newHandEditHarness(t)
	payee := fixture.New(t, h.db).Payee(fixture.Payee{UserID: h.user, Name: "Corner Shop"})
	other := fixture.New(t, h.db).Payee(fixture.Payee{UserID: h.user, Name: "Kiosk"})
	ruleID := vo.NewId().String()
	h.do(h.full, http.MethodPost, "/api/v1/import/create-rule", map[string]any{
		"id": ruleID, "action": "classify", "matchField": "external_payee", "matchType": "contains", "matchValue": "corner", "categoryId": h.coffee, "payeeId": payee,
	})
	tx := h.tap("Corner Shop")
	if got := h.assignedBy(tx["id"]); got != "rule" {
		t.Fatalf("after the rule: assigned by = %s", got)
	}
	h.update(tx, map[string]any{"payeeId": other})
	if got := h.assignedBy(tx["id"]); got != "NULL" {
		t.Fatalf("payee edit: assigned by = %s, want NULL", got)
	}

	tx = h.transaction(tx["id"].(string))
	h.update(tx, map[string]any{"payeeId": nil, "categoryId": nil})
	res := h.do(h.full, http.MethodPost, "/api/v1/import/apply-rule", map[string]any{"ruleId": ruleID, "scope": "all", "includeEdited": true})
	if res["updated"] != float64(1) {
		t.Fatalf("apply-rule: %v", res)
	}
	if got := h.assignedBy(tx["id"]); got != "rule" {
		t.Fatalf("apply-rule: assigned by = %s, want rule", got)
	}
}

func TestMCPHandEditsClearAssignedBy(t *testing.T) {
	h := newHandEditHarness(t)
	tea := fixture.New(t, h.db).Category(fixture.Category{UserID: h.user, Name: "Tea"})
	single := h.tap("Blue Bottle")
	a, b := h.tap("Blue Bottle"), h.tap("Blue Bottle")
	for _, tx := range []map[string]any{single, a, b} {
		if got := h.assignedBy(tx["id"]); got != "ai" {
			t.Fatalf("after the AI: assigned by = %s", got)
		}
	}

	h.mcp("update_transaction", map[string]any{
		"id": single["id"], "type": single["type"], "amount": single["amount"], "account_id": single["accountId"], "date": single["date"],
		"category_id": tea, "payee_id": single["payeeId"],
	})
	if got := h.assignedBy(single["id"]); got != "NULL" {
		t.Fatalf("update_transaction: assigned by = %s, want NULL", got)
	}
	if got := h.assignedBy(a["id"]); got != "ai" {
		t.Fatalf("update_transaction touched another row: %s", got)
	}

	h.mcp("bulk_update_transactions", map[string]any{"ids": []any{a["id"], b["id"]}, "category_id": tea})
	for _, tx := range []map[string]any{a, b} {
		if got := h.assignedBy(tx["id"]); got != "NULL" {
			t.Fatalf("bulk_update_transactions: assigned by = %s, want NULL", got)
		}
	}
}

// A row that disagrees with the suggestion is never listed under it, even
// once a hand edit has cleared the AI's marker and left a field empty.
func TestHandEditDisagreeingRowLeavesTheSuggestion(t *testing.T) {
	h := newHandEditHarness(t)
	tx := h.tap("Blue Bottle")
	if tx["categoryId"] != h.coffee || tx["payeeId"] == nil {
		t.Fatalf("after the AI: %v", tx)
	}
	if got := h.suggestedTransactions(); len(got) != 1 {
		t.Fatalf("suggested before the edit = %v", got)
	}
	tea := fixture.New(t, h.db).Category(fixture.Category{UserID: h.user, Name: "Tea"})
	h.update(tx, map[string]any{"categoryId": tea, "payeeId": nil})
	if got := h.assignedBy(tx["id"]); got != "NULL" {
		t.Fatalf("assigned by = %s, want NULL", got)
	}
	if got := h.suggestedTransactions(); len(got) != 0 {
		t.Fatalf("suggested after setting Tea and clearing the payee = %v, want none", got)
	}
}
