package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CloneDebts copies the debts of one budget into another (clone-budget): every
// column, the rules, the snapshots and the rate changes, under new ids. A copy
// keeps its original creation time and day, so the tiebreak between debts and
// the planning start are the same in the copy. Splits are not copied: the next
// sync rebuilds them on the new budget's accounts. It joins the caller's
// transaction and returns the new id of every debt.
func (s *Service) CloneDebts(ctx context.Context, fromBudgetID, toBudgetID vo.Id) (map[vo.Id]vo.Id, error) {
	ids := map[vo.Id]vo.Id{}
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		// The source's lock keeps its rules as read (a category delete
		// releases them under it); the categories' key-share locks make a
		// delete of one wait for the copies and then release them too.
		if err := s.repo.LockBudget(ctx, fromBudgetID); err != nil {
			return err
		}
		debts, err := s.repo.ListByBudget(ctx, fromBudgetID)
		if err != nil || len(debts) == 0 {
			return err
		}
		var categories []vo.Id
		for _, d := range debts {
			categories = append(categories, ruleCategories(d.Rules)...)
		}
		if err := s.repo.LockRuleCategories(ctx, categories); err != nil {
			return err
		}
		old := make([]vo.Id, 0, len(debts))
		for _, d := range debts {
			old = append(old, d.ID)
		}
		snaps, err := s.repo.Snapshots(ctx, old)
		if err != nil {
			return err
		}
		rates, err := s.repo.RateChanges(ctx, old)
		if err != nil {
			return err
		}
		for _, d := range debts {
			cp := *d
			cp.ID, cp.BudgetID = s.repo.NextIdentity(), toBudgetID
			if d.Current != nil {
				current := *d.Current
				cp.Current = &current
			}
			cp.Rules = make([]model.DebtRule, len(d.Rules))
			for i, r := range d.Rules {
				r.ID = s.repo.NextIdentity()
				cp.Rules[i] = r
			}
			if err := s.repo.Save(ctx, &cp); err != nil {
				return err
			}
			ids[d.ID] = cp.ID
		}
		for _, sn := range snaps {
			cp := *sn
			cp.ID, cp.DebtID = s.repo.NextIdentity(), ids[sn.DebtID]
			if err := s.repo.AddSnapshot(ctx, &cp); err != nil {
				return err
			}
		}
		for _, c := range rates {
			cp := *c
			cp.ID, cp.DebtID = s.repo.NextIdentity(), ids[c.DebtID]
			if err := s.repo.SaveRateChange(ctx, &cp); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
