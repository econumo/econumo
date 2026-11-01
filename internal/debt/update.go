package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) UpdateDebt(ctx context.Context, userID vo.Id, req model.UpdateDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	in, err := parseInput(req.Name, "", req.CategoryId, req.PayeeId, req.TagId, req.StartDate,
		req.Balance, req.Payment, req.Schedule, req.LastPaymentDate)
	if err != nil {
		return nil, err
	}
	d, err := s.ownedDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	// Payments not split yet are split under the terms they were made under,
	// before a new snapshot can change them.
	if _, err := s.syncCategory(ctx, d.CategoryID); err != nil {
		return nil, err
	}
	digits, err := s.rates.FractionDigits(ctx, d.CurrencyID)
	if err != nil {
		return nil, err
	}
	if err := s.checkClassifications(ctx, userID, in.category, in.payee, in.tag); err != nil {
		return nil, err
	}

	today, now := s.today(ctx), s.clock.Now()
	oldCategory := d.CategoryID
	var saved *model.Debt
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockOwner(ctx, userID); err != nil {
			return err
		}
		cur, err := s.ownedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		keyChanged := !cur.CategoryID.Equal(in.category) || !sameIdPtr(cur.PayeeID, in.payee) ||
			!sameIdPtr(cur.TagID, in.tag) || !sameDay(cur.StartDate, in.start)
		cur.Name, cur.CategoryID, cur.PayeeID, cur.TagID, cur.StartDate = in.name, in.category, in.payee, in.tag, in.start
		if keyChanged {
			others, err := s.repo.ListByCategory(ctx, cur.CategoryID)
			if err != nil {
				return err
			}
			if err := checkKeys(cur, others); err != nil {
				return err
			}
		}
		terms := in.terms(today, digits)
		changed := termsDiffer(cur.Current, terms)
		if changed {
			if err := validateTerms(terms); err != nil {
				return err
			}
			cur.Current = terms
		}
		cur.UpdatedAt = now
		if err := s.repo.Save(ctx, cur); err != nil {
			return err
		}
		if changed {
			if err := s.repo.AddSnapshot(ctx, s.snapshotOf(cur, now)); err != nil {
				return err
			}
		}
		saved = cur
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !saved.CategoryID.Equal(oldCategory) {
		// Debts left behind may now cover a wider range.
		if _, err := s.syncCategory(ctx, oldCategory); err != nil {
			return nil, err
		}
	}
	return s.resultOf(ctx, saved)
}

// MarkDebtPaidOff writes a zero-balance snapshot as of today, keeping the
// payment, schedule and last payment date (the rate is irrelevant at zero).
func (s *Service) MarkDebtPaidOff(ctx context.Context, userID vo.Id, req model.MarkDebtPaidOffRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	d, err := s.ownedDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.syncCategory(ctx, d.CategoryID); err != nil {
		return nil, err
	}
	today, now := s.today(ctx), s.clock.Now()
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockOwner(ctx, userID); err != nil {
			return err
		}
		cur, err := s.ownedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		if cur.Current.Balance.IsZero() {
			d = cur
			return nil
		}
		cur.Current.AsOf, cur.Current.Balance = today, vo.NewDecimal("0")
		cur.UpdatedAt = now
		if err := s.repo.Save(ctx, cur); err != nil {
			return err
		}
		d = cur
		return s.repo.AddSnapshot(ctx, s.snapshotOf(cur, now))
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, d)
}
