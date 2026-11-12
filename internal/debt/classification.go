package debt

import (
	"context"
	"errors"
	"slices"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

const (
	KindCategory = "category"
	KindPayee    = "payee"
	KindTag      = "tag"
	KindLabel    = "label"
)

// ErrRulesChangedConcurrently: a debt rule naming the classification was
// committed after Prepare read them, so its budget is not locked. Taking that
// lock after the classification row would invert the order every debt write
// keeps; the delete or merge fails instead (a 5xx, so clients retry it).
var ErrRulesChangedConcurrently = errors.New("debt: a rule naming the classification changed meanwhile; try again")

// A classification's owner may delete or merge it whatever their access to
// the debts that name it (a former participant, a guest): the debt rules
// follow, and nothing is refused (ErrRulesChangedConcurrently is a race to
// retry, not a refusal). These run inside the delete's or the
// merge's transaction and take no access check.

// budgetsNaming are the budgets of the debts with a rule naming any of ids,
// in id order: the order every caller locks them in.
func (s *Service) budgetsNaming(ctx context.Context, kind string, ids ...vo.Id) ([]vo.Id, error) {
	var out []vo.Id
	for _, id := range ids {
		debts, err := s.repo.ListByClassification(ctx, kind, id)
		if err != nil {
			return nil, err
		}
		for _, d := range debts {
			if !slices.ContainsFunc(out, d.BudgetID.Equal) {
				out = append(out, d.BudgetID)
			}
		}
	}
	slices.SortFunc(out, compareIDs)
	return out, nil
}

// LockForClassification locks the budgets of the debts whose rules name any
// of ids. A delete or a merge takes them first, so its lock order matches
// every debt write's: the budget rows first, then budget element rows.
func (s *Service) LockForClassification(ctx context.Context, kind string, ids ...vo.Id) error {
	budgets, err := s.budgetsNaming(ctx, kind, ids...)
	if err != nil {
		return err
	}
	for _, b := range budgets {
		if err := s.repo.LockBudget(ctx, b); err != nil {
			return err
		}
	}
	return nil
}

// ReleaseClassification takes id out of every debt rule before the
// classification is deleted: a rule on a deleted category goes (a debt may be
// left with none and then matches nothing), a deleted payee, tag or label no
// longer narrows its rule. Rules that become duplicates or conflict are then
// resolved, the older debt keeping its rule. Plans stay where they are, and
// payments already split keep their splits until the next sync.
func (s *Service) ReleaseClassification(ctx context.Context, kind string, id vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		budgets, err := s.heldNaming(ctx, kind, id)
		if err != nil {
			return err
		}
		for _, b := range budgets {
			if _, err := s.rewriteRules(ctx, b, func(r model.DebtRule) (model.DebtRule, bool, bool) {
				return releaseRule(kind, id, r)
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func releaseRule(kind string, id vo.Id, r model.DebtRule) (out model.DebtRule, keep, changed bool) {
	drop := func(p **vo.Id) bool {
		if *p == nil || !(*p).Equal(id) {
			return false
		}
		*p = nil
		return true
	}
	switch kind {
	case KindCategory:
		if r.CategoryID.Equal(id) {
			return r, false, true
		}
		return r, true, false
	case KindPayee:
		changed = drop(&r.PayeeID)
	case KindTag:
		changed = drop(&r.TagID)
	case KindLabel:
		changed = drop(&r.LabelID)
	}
	return r, true, changed
}

// ReassignClassification moves the debt rules naming oldID to newID for a
// merge, after the merge has re-pointed the transactions and merged the
// budget rows; rules that become duplicates or conflict are resolved like a
// delete's. For a category, every whole-category rule on the target takes the
// merged category row's plans, whether it moved or was there already. The
// budgets involved are then synced, in the merge's transaction.
func (s *Service) ReassignClassification(ctx context.Context, kind string, oldID, newID vo.Id) error {
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		budgets, err := s.heldNaming(ctx, kind, oldID, newID)
		if err != nil {
			return err
		}
		for _, b := range budgets {
			debts, err := s.rewriteRules(ctx, b, func(r model.DebtRule) (model.DebtRule, bool, bool) {
				return reassignRule(kind, oldID, newID, r)
			})
			if err != nil {
				return err
			}
			if kind != KindCategory {
				continue
			}
			for _, d := range debts {
				for _, r := range d.Rules {
					if r.CategoryID.Equal(newID) && model.IsWholeCategoryRule(d, r, debts) {
						if err := s.moveToDebt(ctx, d, newID); err != nil {
							return err
						}
					}
				}
			}
		}
		for _, b := range budgets {
			if err := s.syncBudgetID(ctx, b); err != nil {
				return err
			}
		}
		return nil
	})
}

func reassignRule(kind string, oldID, newID vo.Id, r model.DebtRule) (out model.DebtRule, keep, changed bool) {
	swap := func(p **vo.Id) bool {
		if *p == nil || !(*p).Equal(oldID) {
			return false
		}
		id := newID
		*p = &id
		return true
	}
	switch kind {
	case KindCategory:
		if r.CategoryID.Equal(oldID) {
			r.CategoryID, changed = newID, true
		}
	case KindPayee:
		changed = swap(&r.PayeeID)
	case KindTag:
		changed = swap(&r.TagID)
	case KindLabel:
		changed = swap(&r.LabelID)
	}
	return r, true, changed
}

// heldNaming re-reads the budgets of the debts naming ids, under the
// classification row lock; LockForClassification must already hold each.
func (s *Service) heldNaming(ctx context.Context, kind string, ids ...vo.Id) ([]vo.Id, error) {
	budgets, err := s.budgetsNaming(ctx, kind, ids...)
	if err != nil {
		return nil, err
	}
	for _, b := range budgets {
		if !s.repo.HoldsBudgetLock(ctx, b) {
			return nil, ErrRulesChangedConcurrently
		}
	}
	return budgets, nil
}

// rewriteRules applies edit to every rule of the budget's debts, resolves the
// conflicts that leaves (model.NormalizeBudgetRules) and saves the debts that
// changed. It returns the budget's debts as they now are.
func (s *Service) rewriteRules(ctx context.Context, budgetID vo.Id, edit func(model.DebtRule) (model.DebtRule, bool, bool)) ([]*model.Debt, error) {
	debts, err := s.repo.ListByBudget(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	changed := map[vo.Id]bool{}
	for _, d := range debts {
		rules := make([]model.DebtRule, 0, len(d.Rules))
		for _, r := range d.Rules {
			out, keep, edited := edit(r)
			if edited {
				changed[d.ID] = true
			}
			if keep {
				rules = append(rules, out)
			}
		}
		d.Rules = rules
	}
	for _, d := range model.NormalizeBudgetRules(debts) {
		changed[d.ID] = true
	}
	now := s.clock.Now()
	for _, d := range debts {
		if !changed[d.ID] {
			continue
		}
		d.UpdatedAt = now
		if err := s.repo.Save(ctx, d); err != nil {
			return nil, err
		}
	}
	return debts, nil
}
