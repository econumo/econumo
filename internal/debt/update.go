package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateDebt saves a debt's name, matching filters, start date and terms. A
// change to a set-up debt's terms needs all of them and starts a new
// snapshot; a body without a balance and a last payment date keeps the stored
// terms, so a stale screen renaming the debt cannot write old ones back. A
// debt not set up is set up by a body carrying a balance and a last payment
// date (its first snapshot, as of today); without them only its payment and
// schedule change, each kept when left out.
func (s *Service) UpdateDebt(ctx context.Context, userID vo.Id, req model.UpdateDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	in, err := parseInput(debtBody{name: req.Name, categoryID: req.CategoryId,
		payeeID: req.PayeeId, tagID: req.TagId, labelID: req.LabelId, start: req.StartDate,
		balance: req.Balance, payment: req.Payment, schedule: req.Schedule, lastPayment: req.LastPaymentDate})
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
	if err := s.checkClassifications(ctx, userID, in); err != nil {
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
		if cur.IsSetUp() && !in.hasTerms && in.changesTerms(*cur.Current, digits) {
			return missingTerms(true, true)
		}
		wasWhole, wasCategory, wasSetUp := isWholeCategory(cur), cur.CategoryID, cur.IsSetUp()
		keyChanged := !cur.CategoryID.Equal(in.category) || !sameIdPtr(cur.PayeeID, in.payee) ||
			!sameIdPtr(cur.TagID, in.tag) || !sameIdPtr(cur.LabelID, in.label) || !sameDay(cur.StartDate, in.start)
		cur.Name, cur.CategoryID, cur.PayeeID, cur.TagID, cur.LabelID, cur.StartDate = in.name, in.category, in.payee, in.tag, in.label, in.start
		if keyChanged {
			others, err := s.repo.ListByCategory(ctx, cur.CategoryID)
			if err != nil {
				return err
			}
			if err := checkKeys(cur, others); err != nil {
				return err
			}
		}
		changed := false
		if in.hasTerms {
			terms := in.terms(today, digits)
			if changed = !cur.IsSetUp() || termsDiffer(*cur.Current, terms); changed {
				if err := validateTerms(terms); err != nil {
					return err
				}
				cur.SetTerms(terms)
			}
		} else if !cur.IsSetUp() {
			if in.hasPayment {
				cur.Payment = in.payment.Round(digits)
				if err := validatePayment(cur.Payment); err != nil {
					return err
				}
			}
			if in.hasSchedule {
				cur.Schedule = in.schedule
			}
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
		if !wasSetUp && cur.IsSetUp() {
			if err := s.dropSplits(ctx, cur.ID); err != nil {
				return err
			}
			if wasWhole && isWholeCategory(cur) && cur.CategoryID.Equal(wasCategory) {
				if err := s.moveTailToCategory(ctx, cur); err != nil {
					return err
				}
			}
		}
		// Now covering a whole category it did not cover before, the debt
		// takes that category's plans, as a conversion does. Going the other
		// way the debt keeps its own: they plan the loan's payments.
		if isWholeCategory(cur) && (!wasWhole || !cur.CategoryID.Equal(wasCategory)) {
			if err := s.moveToDebt(ctx, cur); err != nil {
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
	if !d.IsSetUp() {
		return nil, notSetUp()
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
		if !cur.IsSetUp() {
			return notSetUp()
		}
		// A zero snapshot followed by a payment is reopened: only a debt
		// that is paid off right now needs nothing written.
		st, err := s.stateNow(ctx, cur)
		if err != nil {
			return err
		}
		if st.project(today, nil, nil).PaidOff {
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

// stateNow is the debt's state from what is stored, splitting new payments
// in memory: it writes nothing, so it can run inside a write's transaction.
func (s *Service) stateNow(ctx context.Context, d *model.Debt) (*debtState, error) {
	debts, err := s.repo.ListByCategory(ctx, d.CategoryID)
	if err != nil {
		return nil, err
	}
	states, err := s.computeDebts(ctx, d.CategoryID, debts, false)
	if err != nil {
		return nil, err
	}
	for _, st := range states {
		if st.debt.ID.Equal(d.ID) {
			return st, nil
		}
	}
	return nil, errs.NewNotFound("Debt not found")
}

// dropSplits removes the provisional zero-interest splits a debt collected
// before setup, so the next sync splits those payments by the new terms.
func (s *Service) dropSplits(ctx context.Context, id vo.Id) error {
	splits, err := s.repo.Splits(ctx, []vo.Id{id})
	if err != nil {
		return err
	}
	txIDs := make([]vo.Id, 0, len(splits))
	for _, sp := range splits {
		txIDs = append(txIDs, sp.TransactionID)
	}
	return s.repo.DeleteSplits(ctx, id, txIDs)
}

// notSetUp refuses an action that needs the debt's terms.
func notSetUp() error {
	return &errs.ValidationError{Msg: "Finish setting up this debt first.", MsgCode: errs.CodeDebtNotSetUp}
}
