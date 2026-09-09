package imports

import (
	"sort"
	"strings"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// NormalizeText trims and collapses whitespace runs to one space. Case
// folding is applied separately (matchRule), so a case-sensitive rule still
// sees normalized spacing. web/src/lib/importMatch.ts mirrors this exactly:
// the count the rule prompt shows must equal what the server later matches.
func NormalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ruleText picks the event text a match_field reads. external_payee is the
// text that becomes the transaction description; description is the
// provider's memo, which only SimpleFIN supplies — Apple Wallet events fall
// back to the payee so a description rule still has something to match.
func ruleText(field string, ev model.IngestEvent) string {
	if field == model.ImportRuleMatchFieldDescription && ev.Description != "" {
		return ev.Description
	}
	return ev.Payee
}

func matchRule(r model.ImportRule, ev model.IngestEvent) bool {
	text, value := NormalizeText(ruleText(r.MatchField, ev)), NormalizeText(r.MatchValue)
	if !r.IsCaseSensitive {
		text, value = strings.ToLower(text), strings.ToLower(value)
	}
	if value == "" {
		return false
	}
	switch r.MatchType {
	case model.ImportRuleMatchTypeExact:
		return text == value
	case model.ImportRuleMatchTypePrefix:
		return strings.HasPrefix(text, value)
	case model.ImportRuleMatchTypeContains:
		return strings.Contains(text, value)
	}
	return false
}

// ruleSet is one user's rules, filtered to a source, ordered by priority.
type ruleSet struct{ rules []model.ImportRule }

func newRuleSet(rules []model.ImportRule, sourceID vo.Id) ruleSet {
	kept := make([]model.ImportRule, 0, len(rules))
	for _, r := range rules {
		if r.SourceID == nil || *r.SourceID == sourceID {
			kept = append(kept, r)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		a, b := kept[i], kept[j]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return a.ID.String() < b.ID.String()
	})
	return ruleSet{rules: kept}
}

// skip returns the first matching skip rule. Skip beats classify whatever
// the priorities: skipping makes classification moot (spec, Part 2).
func (rs ruleSet) skip(ev model.IngestEvent) *vo.Id {
	for _, r := range rs.rules {
		if r.Action == model.ImportRuleActionSkip && matchRule(r, ev) {
			id := r.ID
			return &id
		}
	}
	return nil
}

// classify folds every matching classify rule in priority order: the first
// rule to set a field wins it, labels are unioned (a set), and applied_rule_id
// is the first rule that contributed anything.
func (rs ruleSet) classify(ev model.IngestEvent) model.ImportClassification {
	var out model.ImportClassification
	seen := map[vo.Id]bool{}
	for i := range rs.rules {
		r := &rs.rules[i]
		if r.Action != model.ImportRuleActionClassify || !matchRule(*r, ev) {
			continue
		}
		contributed := false
		if out.CategoryID == nil && r.TargetCategoryID != nil {
			out.CategoryID, contributed = r.TargetCategoryID, true
		}
		if out.PayeeID == nil && r.TargetPayeeID != nil {
			out.PayeeID, contributed = r.TargetPayeeID, true
		}
		if out.TagID == nil && r.TargetTagID != nil {
			out.TagID, contributed = r.TargetTagID, true
		}
		for _, l := range r.LabelIDs {
			if seen[l] || len(out.LabelIDs) >= model.MaxImportRuleLabels {
				continue
			}
			seen[l] = true
			out.LabelIDs = append(out.LabelIDs, l)
			contributed = true
		}
		if contributed && out.RuleID == nil {
			id := r.ID
			out.RuleID = &id
		}
	}
	return out
}
