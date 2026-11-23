package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

func txSuggestionList(t *testing.T, h *harness, user string) []model.ImportTransactionSuggestionResult {
	t.Helper()
	res, err := h.svc.GetTransactionSuggestionList(context.Background(), vo.MustParseId(user))
	if err != nil {
		t.Fatal(err)
	}
	return res.Items
}

func fillTxSuggestion(h *harness, user string, id vo.Id) (*model.FillImportTransactionSuggestionResult, error) {
	return h.svc.FillTransactionSuggestion(context.Background(), vo.MustParseId(user), model.FillImportTransactionSuggestionRequest{Id: id.String()})
}

func isFilled(t *testing.T, h *harness, linkID vo.Id) bool {
	t.Helper()
	filled, err := h.repo.FilledTransactionSuggestionLinks(context.Background(), []vo.Id{linkID})
	if err != nil {
		t.Fatal(err)
	}
	return filled[linkID]
}

func strIs(p *string, want string) bool {
	if want == "" {
		return p == nil
	}
	return p != nil && *p == want
}

func archiveAll(list []model.ImportNamed) {
	for i := range list {
		list[i].Archived = true
	}
}

func TestTransactionSuggestionListHidesInvalid(t *testing.T) {
	t.Run("archived category keeps the payee", func(t *testing.T) {
		h, c, _ := suggestSetup(t)
		cat := h.category(t, userA, "Coffee")
		py := h.payee(t, userA, "Blue Bottle Coffee")
		_, links := seedSuggestion(t, h, c, cat, py)
		archiveAll(h.entities.categories[vo.MustParseId(userA)])
		items := txSuggestionList(t, h, userA)
		if len(items) != 1 || items[0].Id != links[0].ID.String() || items[0].CategoryId != nil || !strIs(items[0].PayeeId, py) {
			t.Fatalf("list = %+v", items)
		}
		it := items[0]
		if it.TransactionId != links[0].TransactionID.String() || it.AccountId != acct1 || it.Type != "expense" || it.Currency != "USD" ||
			it.Amount == "" || it.Date == "" || it.ExternalPayee != "Blue Bottle" {
			t.Fatalf("item = %+v", it)
		}
	})
	t.Run("archived category and payee drop the row", func(t *testing.T) {
		h, c, _ := suggestSetup(t)
		cat := h.category(t, userA, "Coffee")
		py := h.payee(t, userA, "Blue Bottle Coffee")
		seedSuggestion(t, h, c, cat, py)
		uA := vo.MustParseId(userA)
		archiveAll(h.entities.categories[uA])
		archiveAll(h.entities.payees[uA])
		if items := txSuggestionList(t, h, userA); len(items) != 0 {
			t.Fatalf("list = %+v", items)
		}
		if got := txSuggestions(t, h, userA); len(got) != 0 {
			t.Fatalf("stale suggestion kept: %+v", got)
		}
	})
	t.Run("changed type drops the row", func(t *testing.T) {
		h, c, _ := suggestSetup(t)
		py := h.payee(t, userA, "Blue Bottle Coffee")
		_, links := seedSuggestion(t, h, c, "", py)
		h.setTx(t, links[0].TransactionID, "type", "1")
		if items := txSuggestionList(t, h, userA); len(items) != 0 {
			t.Fatalf("list = %+v", items)
		}
	})
	t.Run("a field set by hand is not offered", func(t *testing.T) {
		h, c, _ := suggestSetup(t)
		cat := h.category(t, userA, "Coffee")
		py := h.payee(t, userA, "Blue Bottle Coffee")
		mine := h.payee(t, userA, "My Cafe")
		_, links := seedSuggestion(t, h, c, cat, py)
		h.setTx(t, links[0].TransactionID, "payee_id", mine)
		items := txSuggestionList(t, h, userA)
		if len(items) != 1 || !strIs(items[0].CategoryId, cat) || items[0].PayeeId != nil {
			t.Fatalf("list = %+v", items)
		}
	})
}

func TestTransactionSuggestionListOrder(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	_, links := seedSuggestion(t, h, c, cat, "", "Blue Bottle", "Blue Bottle Mission", "Blue Bottle Hayes")
	h.setTx(t, links[0].TransactionID, "spent_at", "2026-08-18 10:00:00")
	h.setTx(t, links[1].TransactionID, "spent_at", "2026-08-19 10:00:00")
	h.setTx(t, links[2].TransactionID, "spent_at", "2026-08-17 10:00:00")
	items := txSuggestionList(t, h, userA)
	if len(items) != 3 || items[0].Id != links[1].ID.String() || items[1].Id != links[0].ID.String() || items[2].Id != links[2].ID.String() {
		t.Fatalf("list = %+v, want newest first", items)
	}
}

func TestTransactionSuggestionListRespectsSwitches(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "")

	facts[userA] = ownerFact{language: "en", fillOff: true}
	if items := txSuggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("fill off: list = %+v", items)
	}
	if got := txSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("fill off hides, never deletes: %+v", got)
	}

	facts[userA] = ownerFact{language: "en", readonly: true}
	if items := txSuggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("readonly: list = %+v", items)
	}
	if got := txSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("readonly hides, never deletes: %+v", got)
	}

	facts[userA] = ownerFact{language: "en"}
	if items := txSuggestionList(t, h, userA); len(items) != 1 {
		t.Fatalf("switched back on: list = %+v", items)
	}

	facts[userA] = ownerFact{language: "en", aiOff: true}
	if items := txSuggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("AI off: list = %+v", items)
	}
	if got := txSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("AI off deletes: %+v", got)
	}
}

func TestFillTransactionSuggestionFillsEmptyOnly(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	mine := h.payee(t, userA, "My Cafe")
	_, links := seedSuggestion(t, h, c, cat, py)
	h.setTx(t, links[0].TransactionID, "payee_id", mine)
	before, err := h.repo.GetLink(context.Background(), links[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	res, err := fillTxSuggestion(h, userA, links[0].ID)
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
	if !idIs(tx.CategoryID, cat) || !idIs(tx.PayeeID, mine) {
		t.Fatalf("transaction = %+v", tx)
	}
	if len(h.txns.updated) != 1 || len(h.txns.replaced) != 0 {
		t.Fatalf("updates = %d, label-replacing updates = %d", len(h.txns.updated), len(h.txns.replaced))
	}
	if !isFilled(t, h, links[0].ID) || len(txSuggestions(t, h, userA)) != 0 {
		t.Fatal("the suggestion must be marked filled")
	}
	after, err := h.repo.GetLink(context.Background(), links[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !sameOptID(before.AppliedCategoryID, after.AppliedCategoryID) || !sameOptID(before.AppliedPayeeID, after.AppliedPayeeID) || !sameOptID(before.AppliedRuleID, after.AppliedRuleID) {
		t.Fatalf("applied snapshot moved: before %+v, after %+v", before, after)
	}
	if items := txSuggestionList(t, h, userA); len(items) != 0 {
		t.Fatalf("a filled suggestion is not listed: %+v", items)
	}
	_, err = fillTxSuggestion(h, userA, links[0].ID)
	wantNotFound(t, err)
}

func sameOptID(a, b *vo.Id) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func TestFillTransactionSuggestionSkipsArchivedCategory(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	_, links := seedSuggestion(t, h, c, cat, py)
	archiveAll(h.entities.categories[vo.MustParseId(userA)])
	res, err := fillTxSuggestion(h, userA, links[0].ID)
	if err != nil || res.Updated != 1 {
		t.Fatalf("fill = %+v, %v", res, err)
	}
	tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	if tx.CategoryID != nil || !idIs(tx.PayeeID, py) {
		t.Fatalf("transaction = %+v", tx)
	}
}

func TestFillTransactionSuggestionNothingLeft(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	py := h.payee(t, userA, "Blue Bottle Coffee")
	other := h.category(t, userA, "Food")
	mine := h.payee(t, userA, "My Cafe")
	_, links := seedSuggestion(t, h, c, cat, py)
	h.setTx(t, links[0].TransactionID, "category_id", other)
	h.setTx(t, links[0].TransactionID, "payee_id", mine)

	res, err := fillTxSuggestion(h, userA, links[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Updated != 0 || len(h.txns.updated) != 0 {
		t.Fatalf("updated = %d, writes = %d", res.Updated, len(h.txns.updated))
	}
	tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID)
	if !idIs(tx.CategoryID, other) || !idIs(tx.PayeeID, mine) {
		t.Fatalf("transaction = %+v", tx)
	}
	if len(txSuggestions(t, h, userA)) != 0 || isFilled(t, h, links[0].ID) {
		t.Fatal("a suggestion with nothing left to fill is deleted")
	}
}

func TestFillTransactionSuggestionTypeChanged(t *testing.T) {
	h, c, _ := suggestSetup(t)
	py := h.payee(t, userA, "Blue Bottle Coffee")
	_, links := seedSuggestion(t, h, c, "", py)
	h.setTx(t, links[0].TransactionID, "type", "1")
	res, err := fillTxSuggestion(h, userA, links[0].ID)
	if err != nil || res.Updated != 0 || len(h.txns.updated) != 0 {
		t.Fatalf("fill = %+v, %v, writes = %d", res, err, len(h.txns.updated))
	}
	if tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID); tx.PayeeID != nil {
		t.Fatalf("an income row took an expense suggestion's payee: %+v", tx)
	}
	if len(txSuggestions(t, h, userA)) != 0 || isFilled(t, h, links[0].ID) {
		t.Fatal("a suggestion whose transaction changed type is deleted")
	}
}

func TestFillTransactionSuggestionNotOwner(t *testing.T) {
	t.Run("another user", func(t *testing.T) {
		h, c, _ := suggestSetup(t)
		cat := h.category(t, userA, "Coffee")
		_, links := seedSuggestion(t, h, c, cat, "")
		_, err := fillTxSuggestion(h, userB, links[0].ID)
		wantNotFound(t, err)
		if got := txSuggestions(t, h, userA); len(got) != 1 {
			t.Fatalf("the owner's suggestion must survive: %+v", got)
		}
		if got := txSuggestionList(t, h, userB); len(got) != 0 {
			t.Fatalf("another user's list = %+v", got)
		}
	})
	t.Run("the importer on a shared account", func(t *testing.T) {
		h, c, _ := suggestSetup(t)
		h.accounts.shared = true
		if _, err := h.svc.LinkAccount(context.Background(), vo.MustParseId(userA), model.LinkImportAccountRequest{SourceId: source, ExternalAccountId: "Apple Card", AccountId: acctB}); err != nil {
			t.Fatal(err)
		}
		kaffee := h.category(t, userB, "Kaffee")
		c.reply = `{"rows":[{"key":"0","matchText":"Blue Bottle","categoryId":"` + kaffee + `"}]}`
		ingest(t, h, tap)
		owned := txSuggestions(t, h, userB)
		if len(owned) != 1 {
			t.Fatalf("owner suggestions = %+v", owned)
		}
		_, err := fillTxSuggestion(h, userA, owned[0].LinkID)
		wantNotFound(t, err)
		_, err = h.svc.DismissTransactionSuggestion(context.Background(), vo.MustParseId(userA), model.DismissImportTransactionSuggestionRequest{Id: owned[0].LinkID.String()})
		wantNotFound(t, err)
		if got := txSuggestionList(t, h, userA); len(got) != 0 {
			t.Fatalf("importer list = %+v", got)
		}
		if got := txSuggestionList(t, h, userB); len(got) != 1 {
			t.Fatalf("owner list = %+v", got)
		}
	})
	t.Run("unknown and malformed ids", func(t *testing.T) {
		h, _, _ := suggestSetup(t)
		_, err := fillTxSuggestion(h, userA, vo.NewId())
		wantNotFound(t, err)
		_, err = h.svc.FillTransactionSuggestion(context.Background(), vo.MustParseId(userA), model.FillImportTransactionSuggestionRequest{Id: "nope"})
		wantNotFound(t, err)
	})
}

// Fill in leaves the applied snapshot alone, so the filled value reads as a
// hand edit to apply-rule.
func TestFilledRowSkippedByApplyRule(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	other := h.category(t, userA, "Food")
	_, links := seedSuggestion(t, h, c, cat, "")
	if res, err := fillTxSuggestion(h, userA, links[0].ID); err != nil || res.Updated != 1 {
		t.Fatalf("fill = %+v, %v", res, err)
	}
	uA := vo.MustParseId(userA)
	preview, err := h.svc.PreviewRule(context.Background(), uA, model.PreviewImportRuleRequest{ImportRuleSpec: model.ImportRuleSpec{
		Action: model.ImportRuleActionClassify, MatchField: model.ImportRuleMatchFieldExternalPayee, MatchType: model.ImportRuleMatchTypeContains,
		MatchValue: "blue bottle", CategoryId: &other,
	}, Scope: model.ImportRuleScopeAll})
	if err != nil || preview.Matched != 1 || preview.AlreadyEdited != 1 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	rule := h.f.ImportRule(fixture.ImportRule{UserID: userA, MatchValue: "blue bottle", CategoryID: other})
	applied, err := h.svc.ApplyRule(context.Background(), uA, model.ApplyImportRuleRequest{RuleId: rule, Scope: model.ImportRuleScopeAll})
	if err != nil || applied.Updated != 0 || applied.Skipped != 1 {
		t.Fatalf("apply = %+v, %v", applied, err)
	}
	if tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID); !idIs(tx.CategoryID, cat) {
		t.Fatalf("apply overwrote the filled category: %+v", tx)
	}
}

func TestDismissTransactionSuggestionDeletes(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	_, links := seedSuggestion(t, h, c, cat, "", "Blue Bottle", "Blue Bottle Mission")
	uA := vo.MustParseId(userA)

	_, err := h.svc.DismissTransactionSuggestion(context.Background(), vo.MustParseId(userB), model.DismissImportTransactionSuggestionRequest{Id: links[0].ID.String()})
	wantNotFound(t, err)

	res, err := h.svc.DismissTransactionSuggestion(context.Background(), uA, model.DismissImportTransactionSuggestionRequest{Id: links[0].ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].Id != links[1].ID.String() {
		t.Fatalf("list after dismiss = %+v", res.Items)
	}
	if got := txSuggestions(t, h, userA); len(got) != 1 || got[0].LinkID != links[1].ID {
		t.Fatalf("stored = %+v", got)
	}
	if tx, _ := h.txns.GetByID(context.Background(), *links[0].TransactionID); tx.CategoryID != nil {
		t.Fatalf("dismiss changed the transaction: %+v", tx)
	}
	_, err = h.svc.DismissTransactionSuggestion(context.Background(), uA, model.DismissImportTransactionSuggestionRequest{Id: links[0].ID.String()})
	wantNotFound(t, err)

	// a filled suggestion is settled: dismissing it reads as missing and keeps it
	if res, err := fillTxSuggestion(h, userA, links[1].ID); err != nil || res.Updated != 1 {
		t.Fatalf("fill = %+v, %v", res, err)
	}
	_, err = h.svc.DismissTransactionSuggestion(context.Background(), uA, model.DismissImportTransactionSuggestionRequest{Id: links[1].ID.String()})
	wantNotFound(t, err)
	if !isFilled(t, h, links[1].ID) {
		t.Fatal("a filled suggestion must survive a dismiss")
	}
}

func TestClearSuggestionsDropsTransactionSuggestions(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	s, _ := seedSuggestion(t, h, c, cat, "")
	// only the transaction suggestion is left to clear
	if err := h.repo.DeleteSuggestion(context.Background(), s.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ClearSuggestions(context.Background(), vo.MustParseId(userB)); err != nil {
		t.Fatal(err)
	}
	if got := txSuggestions(t, h, userA); len(got) != 1 {
		t.Fatalf("another user's clear touched these: %+v", got)
	}
	if err := h.svc.ClearSuggestions(context.Background(), vo.MustParseId(userA)); err != nil {
		t.Fatal(err)
	}
	if got := txSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("transaction suggestions kept: %+v", got)
	}
}

func TestSuggestionListOptOutDropsTransactionSuggestions(t *testing.T) {
	h, c, facts := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	seedSuggestion(t, h, c, cat, "")
	facts[userA] = ownerFact{language: "en", aiOff: true}
	if got := suggestionList(t, h, userA); len(got) != 0 {
		t.Fatalf("list = %+v", got)
	}
	if got := txSuggestions(t, h, userA); len(got) != 0 {
		t.Fatalf("transaction suggestions kept: %+v", got)
	}
}
