package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CloneDebts copies the debts of one budget into another (clone-budget): every
// column, the snapshots and the rate changes, under new ids. A debt's element
// is mapped through elementIDs (the copy's envelopes have new ids); a category
// keeps its id (it is the same in the copy), and a debt on an envelope the
// map does not name is left out, never attached to an envelope of another
// budget. A copy keeps
// its original creation time and day, so the planning start is the same in
// the copy. Splits are not copied: the next sync rebuilds them on the new
// budget's accounts. It joins the caller's transaction and returns the new id
// of every debt.
func (s *Service) CloneDebts(ctx context.Context, fromBudgetID, toBudgetID vo.Id, elementIDs map[vo.Id]vo.Id) (map[vo.Id]vo.Id, error) {
	ids := map[vo.Id]vo.Id{}
	err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockBudget(ctx, fromBudgetID); err != nil {
			return err
		}
		debts, err := s.repo.ListByBudget(ctx, fromBudgetID)
		if err != nil || len(debts) == 0 {
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
		// a category delete committing meanwhile would leave a copy on
		// nothing: it waits for this write and then deletes the copies
		var categories []vo.Id
		for _, d := range debts {
			if d.ElementType == model.DebtElementCategory {
				categories = append(categories, d.ElementID)
			}
		}
		if len(categories) > 0 {
			if err := s.repo.LockCategories(ctx, categories); err != nil {
				return err
			}
		}
		for _, d := range debts {
			cp := *d
			cp.ID, cp.BudgetID = s.repo.NextIdentity(), toBudgetID
			if mapped, ok := elementIDs[d.ElementID]; ok {
				cp.ElementID = mapped
			} else if d.ElementType == model.DebtElementEnvelope {
				continue
			}
			if d.Current != nil {
				current := *d.Current
				cp.Current = &current
			}
			if err := s.repo.Save(ctx, &cp); err != nil {
				return err
			}
			ids[d.ID] = cp.ID
		}
		for _, sn := range snaps {
			newID, ok := ids[sn.DebtID]
			if !ok {
				continue
			}
			cp := *sn
			cp.ID, cp.DebtID = s.repo.NextIdentity(), newID
			if err := s.repo.AddSnapshot(ctx, &cp); err != nil {
				return err
			}
		}
		for _, c := range rates {
			newID, ok := ids[c.DebtID]
			if !ok {
				continue
			}
			cp := *c
			cp.ID, cp.DebtID = s.repo.NextIdentity(), newID
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
