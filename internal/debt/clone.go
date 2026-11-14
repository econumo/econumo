package debt

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// CloneDebts copies the debts of one budget into another (clone-budget): every
// column, the snapshots and the rate changes, under new ids. A debt's element
// is mapped through elementIDs (the copy's envelopes have new ids); one not in
// it keeps its id (a category is the same in the copy). A copy keeps
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
		for _, d := range debts {
			cp := *d
			cp.ID, cp.BudgetID = s.repo.NextIdentity(), toBudgetID
			if mapped, ok := elementIDs[d.ElementID]; ok {
				cp.ElementID = mapped
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
