package debt

import (
	"context"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MarkDebtPaidOff ends a set-up debt as of a date (default today), after its
// start, on or after its latest statement and no later than today: a statement of 0 as of that date (replacing
// one entered that day), and the debt becomes paid off, its payments after
// the date no longer counting. A debt that still owes may be marked: the
// zero says the loan was closed (paid off or refinanced).
func (s *Service) MarkDebtPaidOff(ctx context.Context, userID vo.Id, req model.MarkDebtPaidOffRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.markDebtPaidOff(ctx, userID, req) })
}

func (s *Service) markDebtPaidOff(ctx context.Context, userID vo.Id, req model.MarkDebtPaidOffRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	d, b, err := s.editableDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if err := paidOffWritable(d); err != nil {
		return nil, err
	}
	var date time.Time
	if strings.TrimSpace(req.Date) != "" {
		if date, err = time.Parse(datetime.DateLayout, req.Date); err != nil {
			return nil, fieldError("date", "This value is not valid.", errs.CodeInvalidFormat)
		}
	}
	if _, err := s.syncBudget(ctx, scopeOf(b)); err != nil {
		return nil, err
	}
	today, now := s.today(ctx), s.clock.Now()
	if date.IsZero() {
		date = today
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		cur, _, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		if err := paidOffWritable(cur); err != nil {
			return err
		}
		if !date.After(*cur.StartDate) || date.After(today) {
			return fieldError("date", "This value is out of range.", errs.CodeOutOfRange)
		}
		// the closing 0 must be the latest snapshot: a later statement would
		// stay the debt's current terms
		snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
		if err != nil {
			return err
		}
		if n := len(snaps); n > 1 && date.Before(snaps[n-1].AsOf) {
			return fieldError("date", "This value is out of range.", errs.CodeOutOfRange)
		}
		zero := *cur.Current
		zero.AsOf, zero.Balance = date, vo.NewDecimal("0")
		if err := s.storeStatement(ctx, cur, zero); err != nil {
			return err
		}
		end := date
		cur.Status, cur.EndedAt, cur.UpdatedAt = model.DebtPaidOff, &end, now
		d = cur
		return s.repo.Save(ctx, cur)
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, b, d)
}

func paidOffWritable(d *model.Debt) error {
	if d.IsPaidOff() {
		return paidOff()
	}
	if !d.IsSetUp() {
		return notSetUp()
	}
	return nil
}

// ReopenDebt takes a paid-off debt back to active: its end goes, and so does
// the statement of 0 it stored on its end date. It is refused while its
// element carries another active debt or one started after it. Reopening an
// active debt changes nothing.
func (s *Service) ReopenDebt(ctx context.Context, userID vo.Id, req model.ReopenDebtRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.reopenDebt(ctx, userID, req) })
}

func (s *Service) reopenDebt(ctx context.Context, userID vo.Id, req model.ReopenDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	_, b, err := s.editableDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	var d *model.Debt
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		cur, _, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		d = cur
		if !cur.IsPaidOff() {
			return nil
		}
		budgetDebts, err := s.repo.ListByBudget(ctx, cur.BudgetID)
		if err != nil {
			return err
		}
		for _, o := range onElement(budgetDebts, cur.ElementID, cur) {
			if !o.IsPaidOff() || createdBefore(cur, o) {
				return cannotReopen()
			}
		}
		if cur.EndedAt != nil {
			if err := s.dropClosingStatement(ctx, cur, *cur.EndedAt); err != nil {
				return err
			}
		}
		cur.Status, cur.EndedAt, cur.UpdatedAt = model.DebtActive, nil, s.clock.Now()
		return s.repo.Save(ctx, cur)
	})
	if err != nil {
		return nil, err
	}
	// the payments after the old end count again
	return s.resultOf(ctx, b, d)
}

// dropClosingStatement removes the statement of 0 dated on the debt's end
// (never the origin), and the splits after it.
func (s *Service) dropClosingStatement(ctx context.Context, d *model.Debt, end time.Time) error {
	snaps, err := s.repo.Snapshots(ctx, []vo.Id{d.ID})
	if err != nil {
		return err
	}
	for i, sn := range snaps {
		if i == 0 || !sn.AsOf.Equal(end) || !sn.Balance.IsZero() {
			continue
		}
		if err := s.repo.DeleteSnapshot(ctx, sn.ID); err != nil {
			return err
		}
		if err := s.dropSplitsAfter(ctx, d, end); err != nil {
			return err
		}
		return s.followLatest(ctx, d)
	}
	return nil
}
