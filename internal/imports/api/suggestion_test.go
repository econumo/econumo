package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func TestSuggestionRoutes_ListAcceptDismiss(t *testing.T) {
	h := newHarness(t)
	cat := h.f.Category(fixture.Category{UserID: userA, Name: "Coffee"})
	h.entities.categories = []string{cat}
	txID := h.f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Amount: "4.75", Description: "Blue Bottle", SpentAt: now})
	link := h.f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: source, ExternalAccountID: "Apple Card", ExternalTransactionID: "t-1", TransactionID: txID,
		Status: model.ImportLinkStatusLinked, ExternalPayee: "Blue Bottle", ExternalAmount: "4.75", ExternalPostedAt: now,
	})
	sid := h.f.ImportSuggestion(fixture.ImportSuggestion{UserID: userA, MatchText: "BLUE BOTTLE", CategoryID: cat, LinkIDs: []string{link}})

	status, env := call(t, h, "GET", "/api/v1/import/get-suggestion-list", nil)
	items := env["data"].(map[string]any)["items"].([]any)
	if status != http.StatusOK || len(items) != 1 || items[0].(map[string]any)["id"] != sid {
		t.Fatalf("list: %d %v", status, env)
	}
	rows := items[0].(map[string]any)["transactions"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["transactionId"] != txID || rows[0].(map[string]any)["currency"] != "USD" {
		t.Fatalf("rows: %v", rows)
	}

	status, env = call(t, h, "POST", "/api/v1/import/accept-suggestion", map[string]any{"id": sid, "ruleId": ""})
	if status != http.StatusBadRequest || env["errors"].(map[string]any)["ruleId"] == nil {
		t.Fatalf("blank ruleId: %d %v", status, env)
	}
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat})
	status, env = call(t, h, "POST", "/api/v1/import/accept-suggestion", map[string]any{"id": sid, "ruleId": rule})
	if status != http.StatusOK || env["data"].(map[string]any)["updated"] != float64(1) {
		t.Fatalf("accept: %d %v", status, env)
	}
	tx, err := h.txns.GetByID(context.Background(), vo.MustParseId(txID))
	if err != nil || tx.CategoryID == nil || tx.CategoryID.String() != cat {
		t.Fatalf("transaction = %+v, %v", tx, err)
	}

	status, env = call(t, h, "POST", "/api/v1/import/dismiss-suggestion", map[string]any{"id": sid})
	if status != http.StatusBadRequest || env["message"] != "Import suggestion not found" {
		t.Fatalf("dismiss accepted: %d %v", status, env)
	}
	status, env = call(t, h, "POST", "/api/v1/import/dismiss-suggestion", map[string]any{"id": "nope"})
	if status != http.StatusBadRequest || env["errors"].(map[string]any)["id"] == nil {
		t.Fatalf("bad id: %d %v", status, env)
	}
}

func TestSuggestionRoutes_Dismiss(t *testing.T) {
	h := newHarness(t)
	h.entities.payees = []string{h.f.Payee(fixture.Payee{UserID: userA, Name: "Shop"})}
	txID := h.f.Transaction(fixture.Transaction{UserID: userA, AccountID: acct1, Amount: "4.75", SpentAt: now})
	link := h.f.ImportTransactionLink(fixture.ImportTransactionLink{
		SourceID: source, ExternalAccountID: "Apple Card", ExternalTransactionID: "t-1", TransactionID: txID,
		Status: model.ImportLinkStatusLinked, ExternalPayee: "Shop", ExternalAmount: "4.75", ExternalPostedAt: now,
	})
	sid := h.f.ImportSuggestion(fixture.ImportSuggestion{UserID: userA, MatchText: "SHOP", PayeeID: h.entities.payees[0], LinkIDs: []string{link}})
	status, env := call(t, h, "POST", "/api/v1/import/dismiss-suggestion", map[string]any{"id": sid})
	if status != http.StatusOK || len(env["data"].(map[string]any)["items"].([]any)) != 0 {
		t.Fatalf("dismiss: %d %v", status, env)
	}
}
