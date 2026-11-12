package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// seedSuggestion imports one Blue Bottle tap per payee text through the AI
// job, which answers with the given category and payee; later taps join the
// open suggestion without a new call.
func seedSuggestion(t *testing.T, h *harness, c *fakeCompleter, cat, payee string, payees ...string) (model.ImportSuggestion, []model.ImportTransactionLink) {
	t.Helper()
	h.mapCard(t, "Apple Card")
	reply := `{"rows":{"0":{"matchText":"Blue Bottle"`
	if cat != "" {
		reply += `,"categoryId":"` + cat + `"`
	}
	if payee != "" {
		reply += `,"payeeId":"` + payee + `"`
	}
	c.reply = reply + `}}}`
	if len(payees) == 0 {
		payees = []string{"Blue Bottle"}
	}
	for i, p := range payees {
		if res := ingest(t, h, pushWithPayee(p, "evt-s"+string(rune('a'+i)))); res.Status != model.ImportIngestStatusCreated {
			t.Fatalf("ingest %d: %s", i, res.Status)
		}
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || len(got[0].LinkIDs) != len(payees) {
		t.Fatalf("seeded suggestions = %+v", got)
	}
	links, err := h.repo.ListLinksBySource(context.Background(), vo.MustParseId(source))
	if err != nil {
		t.Fatal(err)
	}
	return got[0], links
}

func (h *harness) setTx(t *testing.T, txID *vo.Id, column, value string) {
	t.Helper()
	if _, err := h.db.Raw.ExecContext(context.Background(), h.db.Rebind(`UPDATE transactions SET `+column+` = ? WHERE id = ?`), value, txID.String()); err != nil {
		t.Fatal(err)
	}
}

func suggestionList(t *testing.T, h *harness, user string) []model.ImportSuggestionResult {
	t.Helper()
	res, err := h.svc.GetSuggestionList(context.Background(), vo.MustParseId(user))
	if err != nil {
		t.Fatal(err)
	}
	return res.Items
}

func wantNotFound(t *testing.T, err error) {
	t.Helper()
	if _, ok := errs.AsNotFound(err); !ok {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestSuggestionListOwnerOnly(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedSuggestion(t, h, c, cat, "")
	items := suggestionList(t, h, userA)
	if len(items) != 1 || items[0].Id != s.ID.String() || items[0].CategoryId != cat || items[0].PayeeId != "" || items[0].MatchField != model.ImportRuleMatchFieldExternalPayee {
		t.Fatalf("owner list = %+v", items)
	}
	tx := items[0].Transactions
	if len(tx) != 1 || tx[0].TransactionId != links[0].TransactionID.String() || tx[0].AccountId != acct1 || tx[0].Currency != "USD" || tx[0].ExternalPayee != "Blue Bottle" || tx[0].Date == "" || tx[0].Amount == "" {
		t.Fatalf("transactions = %+v", tx)
	}

	if got := suggestionList(t, h, userB); len(got) != 0 {
		t.Fatalf("partner list = %+v", got)
	}
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "blue bottle"})
	_, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userB), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	wantNotFound(t, err)
	_, err = h.svc.DismissSuggestion(context.Background(), vo.MustParseId(userB), model.DismissImportSuggestionRequest{Id: s.ID.String()})
	wantNotFound(t, err)
	if got := openSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("the owner's suggestion must survive: %+v", got)
	}
}

func TestSuggestionListDropsFilledTransactions(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	other := h.category(t, userA, "Food")
	s, links := seedSuggestion(t, h, c, cat, py, "Blue Bottle", "Blue Bottle Mission")

	h.setTx(t, links[0].TransactionID, "category_id", other)
	h.setTx(t, links[0].TransactionID, "payee_id", py)
	items := suggestionList(t, h, userA)
	if len(items) != 1 || len(items[0].Transactions) != 1 || items[0].Transactions[0].TransactionId != links[1].TransactionID.String() {
		t.Fatalf("list = %+v", items)
	}

	// one field still empty keeps the row pending
	h.setTx(t, links[1].TransactionID, "category_id", other)
	if items = suggestionList(t, h, userA); len(items) != 1 || len(items[0].Transactions) != 1 {
		t.Fatalf("list = %+v", items)
	}
	h.setTx(t, links[1].TransactionID, "payee_id", py)
	if items = suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("list = %+v", items)
	}
	_, err := h.repo.GetSuggestion(context.Background(), s.ID)
	wantNotFound(t, err)
}

func TestSuggestionListDropsArchivedCategory(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "")
	uA := vo.MustParseId(userA)
	for i := range h.entities.categories[uA] {
		h.entities.categories[uA][i].Archived = true
	}
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("list = %+v", items)
	}
}

func TestAcceptFillsOnlyEmptyFields(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	mine := h.payee(t, userA, "My Cafe")
	s, links := seedSuggestion(t, h, c, cat, py)
	h.setTx(t, links[0].TransactionID, "payee_id", mine)
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: cat, PayeeID: py})

	res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d", res.Updated)
	}
	tx, err := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	if err != nil {
		t.Fatal(err)
	}
	if tx.CategoryID == nil || tx.CategoryID.String() != cat || tx.PayeeID == nil || tx.PayeeID.String() != mine {
		t.Fatalf("transaction = %+v", tx)
	}
	got, err := h.repo.GetSuggestion(context.Background(), s.ID)
	if err != nil || got.Status != model.ImportSuggestionStatusAccepted {
		t.Fatalf("suggestion = %+v, %v", got, err)
	}
	link, err := h.repo.GetLink(context.Background(), links[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if link.AppliedCategoryID == nil || link.AppliedCategoryID.String() != cat || link.AppliedPayeeID == nil || link.AppliedPayeeID.String() != mine || link.AppliedRuleID == nil || link.AppliedRuleID.String() != rule {
		t.Fatalf("applied = %+v", link)
	}
	if items := suggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("an accepted suggestion is not listed: %+v", items)
	}
}

func TestAcceptSkipsRowsRuleDoesNotMatch(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedSuggestion(t, h, c, cat, "", "Blue Bottle", "SQ *BLUE BOTTLE #12")
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "sq *blue bottle", CategoryID: cat})

	res, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 1 {
		t.Fatalf("updated = %d", res.Updated)
	}
	first, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	second, _ := h.txns.GetByID(context.Background(), *links[1].TransactionID)
	if first.CategoryID != nil {
		t.Fatalf("the rule does not match the first row: %+v", first)
	}
	if second.CategoryID == nil || second.CategoryID.String() != cat {
		t.Fatalf("second = %+v", second)
	}
	// accepting again is not an open suggestion any more
	_, err = h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: rule})
	wantNotFound(t, err)
}

func TestAcceptRejectsForeignRule(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, links := seedSuggestion(t, h, c, cat, "")
	foreign := h.f.ImportRule(fixture.ImportRule{UserID: userB, MatchValue: "blue bottle"})
	_, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: foreign})
	wantNotFound(t, err)

	skip := h.f.ImportRule(fixture.ImportRule{UserID: userA, Action: model.ImportRuleActionSkip, MatchValue: "blue bottle"})
	if _, err := h.svc.AcceptSuggestion(context.Background(), vo.MustParseId(userA), model.AcceptImportSuggestionRequest{Id: s.ID.String(), RuleId: skip}); err == nil {
		t.Fatal("a skip rule cannot accept a suggestion")
	}
	tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	if tx.CategoryID != nil {
		t.Fatalf("transaction = %+v", tx)
	}
	if got := openSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("the suggestion stays open: %+v", got)
	}
}

func TestDismissForgets(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, _ := seedSuggestion(t, h, c, cat, "")
	res, err := h.svc.DismissSuggestion(context.Background(), vo.MustParseId(userA), model.DismissImportSuggestionRequest{Id: s.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 0 {
		t.Fatalf("list after dismiss = %+v", res.Items)
	}
	_, err = h.repo.GetSuggestion(context.Background(), s.ID)
	wantNotFound(t, err)

	if res := ingest(t, h, pushWithPayee("Blue Bottle", "evt-later")); res.Status != model.ImportIngestStatusCreated {
		t.Fatalf("status = %s", res.Status)
	}
	if len(c.calls) != 2 {
		t.Fatalf("a dismissed merchant goes to the AI again: calls = %d", len(c.calls))
	}
	got := openSuggestions(t, h, userA)
	if len(got) != 1 || got[0].ID == s.ID {
		t.Fatalf("suggestions = %+v", got)
	}
}
