package imports_test

import (
	"context"
	"testing"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
	"github.com/econumo/econumo/internal/test/fixture"
)

// A filled row's values are the AI's, not the rule's snapshot, so they read as
// a hand edit to apply-rule.
func TestFilledRowSkippedByApplyRule(t *testing.T) {
	h, c, _ := suggestSetup(t)
	cat := h.category(t, userA, "Coffee")
	other := h.category(t, userA, "Food")
	_, links := seedSuggestion(t, h, c, cat, "")
	fillIn(t, h, links[0].ID)
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
