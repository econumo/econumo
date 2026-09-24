package imports

import (
	"testing"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func TestNormalizeText(t *testing.T) {
	for in, want := range map[string]string{
		"  STARBUCKS   #123 \t SEATTLE\n": "STARBUCKS #123 SEATTLE",
		"":                                "",
		"a":                               "a",
	} {
		if got := NormalizeText(in); got != want {
			t.Errorf("NormalizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func rule(action, field, typ, value string, cs bool) model.ImportRule {
	return model.ImportRule{ID: vo.NewId(), Action: action, MatchField: field, MatchType: typ, MatchValue: value, IsCaseSensitive: cs}
}

func TestMatchRule(t *testing.T) {
	ev := model.IngestEvent{Payee: "  Starbucks  #123", Description: "CARD PURCHASE STARBUCKS #123 SEATTLE"}
	cases := []struct {
		name string
		r    model.ImportRule
		want bool
	}{
		{"contains, case-folded", rule("classify", "external_payee", "contains", "starbucks", false), true},
		{"contains, case-sensitive miss", rule("classify", "external_payee", "contains", "starbucks", true), false},
		{"contains, case-sensitive hit", rule("classify", "external_payee", "contains", "Starbucks", true), true},
		{"exact after normalization", rule("classify", "external_payee", "exact", "starbucks #123", false), true},
		{"exact miss", rule("classify", "external_payee", "exact", "starbucks", false), false},
		{"prefix", rule("classify", "external_payee", "prefix", "STAR", false), true},
		{"prefix miss", rule("classify", "external_payee", "prefix", "#123", false), false},
		{"description field", rule("skip", "description", "prefix", "card purchase", false), true},
		{"match value is normalized too", rule("classify", "external_payee", "exact", "  starbucks   #123 ", false), true},
	}
	for _, tc := range cases {
		if got := matchRule(tc.r, ev); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
	// description falls back to the payee when the provider sends none
	if !matchRule(rule("classify", "description", "contains", "starbucks", false), model.IngestEvent{Payee: "Starbucks"}) {
		t.Error("description must fall back to payee")
	}
}

func TestRuleSet_SkipBeatsClassifyAndScopesBySource(t *testing.T) {
	src, other := vo.NewId(), vo.NewId()
	cat := vo.NewId()
	classify := rule("classify", "external_payee", "contains", "payment", false)
	classify.Priority, classify.TargetCategoryID = 0, &cat
	skip := rule("skip", "external_payee", "prefix", "PAYMENT - THANK YOU", false)
	skip.Priority = 99
	scoped := rule("skip", "external_payee", "contains", "payment", false)
	scoped.SourceID = &other
	rs := newRuleSet([]model.ImportRule{classify, skip, scoped}, src)
	if len(rs.rules) != 2 {
		t.Fatalf("source scoping: %d rules", len(rs.rules))
	}
	ev := model.IngestEvent{Payee: "PAYMENT - THANK YOU"}
	if got := rs.skip(ev); got == nil || *got != skip.ID {
		t.Fatalf("skip must win regardless of priority: %v", got)
	}
	if got := rs.skip(model.IngestEvent{Payee: "Autopay payment"}); got != nil {
		t.Fatalf("no skip rule matches: %v", got)
	}
}

func TestRuleSet_ClassifyFirstWinsPerFieldAndUnionsLabels(t *testing.T) {
	src := vo.NewId()
	catA, catB, payee, tag := vo.NewId(), vo.NewId(), vo.NewId(), vo.NewId()
	labels := make([]vo.Id, 0, 12)
	for i := 0; i < 12; i++ {
		labels = append(labels, vo.NewId())
	}
	now := time.Now()
	first := rule("classify", "external_payee", "contains", "star", false)
	first.Priority, first.TargetCategoryID, first.LabelIDs, first.CreatedAt = 1, &catA, labels[:6], now
	second := rule("classify", "external_payee", "contains", "bucks", false)
	second.Priority, second.TargetCategoryID, second.TargetPayeeID, second.LabelIDs, second.CreatedAt = 2, &catB, &payee, labels[4:12], now
	third := rule("classify", "external_payee", "exact", "nomatch", false)
	third.TargetTagID = &tag
	rs := newRuleSet([]model.ImportRule{second, first, third}, src)
	got := rs.classify(model.IngestEvent{Payee: "Starbucks"})
	if got.CategoryID == nil || *got.CategoryID != catA {
		t.Errorf("category must come from the higher-priority rule: %v", got.CategoryID)
	}
	if got.PayeeID == nil || *got.PayeeID != payee {
		t.Errorf("payee is filled by the first rule that sets it: %v", got.PayeeID)
	}
	if got.TagID != nil {
		t.Errorf("no matching rule sets a tag: %v", got.TagID)
	}
	if got.RuleID == nil || *got.RuleID != first.ID {
		t.Errorf("applied_rule_id is the first contributing rule: %v", got.RuleID)
	}
	if len(got.LabelIDs) != model.MaxImportRuleLabels {
		t.Fatalf("labels must be unioned (dedup 4,5) and capped: %d", len(got.LabelIDs))
	}
	for i := 0; i < 6; i++ {
		if got.LabelIDs[i] != labels[i] {
			t.Fatalf("label order follows priority: %v", got.LabelIDs)
		}
	}
	if empty := rs.classify(model.IngestEvent{Payee: "nothing"}); empty.RuleID != nil || empty.CategoryID != nil || len(empty.LabelIDs) != 0 {
		t.Errorf("no match must be empty: %+v", empty)
	}
}
