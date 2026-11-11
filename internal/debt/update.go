package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateDebt saves a debt's name, currency, matching filters, start date and
// terms. A debt not set up is set up by a body carrying the loan's facts
// (amount borrowed, start date, last payment date; the origin as of the start
// date, plus a snapshot as of today with a balance now); without them only its
// payment, schedule and start date change, each of the first two kept when
// left out. On a set-up debt each fact left out keeps the stored one, so a
// stale screen renaming the debt cannot write old ones back; a fact that
// differs is a correction: the origin is rewritten (a later snapshot taking
// the corrected payment, schedule and last payment date, its balance kept)
// and every payment is split again. A balance now appends a snapshot as of
// today.
func (s *Service) UpdateDebt(ctx context.Context, userID vo.Id, req model.UpdateDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	in, err := parseInput(debtBody{name: req.Name, currencyID: req.CurrencyId, categoryID: req.CategoryId,
		payeeID: req.PayeeId, tagID: req.TagId, labelID: req.LabelId, start: req.StartDate, borrowed: req.AmountBorrowed,
		balance: req.Balance, balanceDate: req.BalanceDate, payment: req.Payment, schedule: req.Schedule, lastPayment: req.LastPaymentDate})
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
		// the amounts in the request are in the currency the debt ends up in
		oldCurrency, currency := cur.CurrencyID, cur.CurrencyID
		if !in.currency.IsZero() && !in.currency.Equal(cur.CurrencyID) {
			if err := s.currencies.EnsureUsable(ctx, userID.String(), in.currency.String()); err != nil {
				return err
			}
			currency = in.currency
		}
		currencyChanged := !currency.Equal(oldCurrency)
		digits, err := s.rates.FractionDigits(ctx, currency)
		if err != nil {
			return err
		}
		// a set-up debt always has its start: left out, the stored one stays
		if cur.IsSetUp() && in.start == nil {
			in.start = cur.StartDate
		}
		if !cur.IsSetUp() && ((!in.hasOrigin && in.hasOwed) || (in.hasOrigin && in.start == nil)) {
			return missingFacts(!in.hasOrigin, in.start == nil, !in.hasOrigin)
		}
		wasWhole, err := s.wholeCategory(ctx, cur)
		if err != nil {
			return err
		}
		wasCategory, wasSetUp := cur.CategoryID, cur.IsSetUp()
		keyChanged := !cur.CategoryID.Equal(in.category) || !sameIdPtr(cur.PayeeID, in.payee) ||
			!sameIdPtr(cur.TagID, in.tag) || !sameIdPtr(cur.LabelID, in.label) || !sameDay(cur.StartDate, in.start)
		// the start of the months the debt row shows, before this update
		var wasFirst time.Time
		if wasWhole && (!sameDay(cur.StartDate, in.start) || !cur.CategoryID.Equal(in.category)) {
			if wasFirst, err = s.firstPlanningDay(ctx, cur); err != nil {
				return err
			}
		}
		cur.Name, cur.CategoryID, cur.PayeeID, cur.TagID, cur.LabelID, cur.StartDate = in.name, in.category, in.payee, in.tag, in.label, in.start
		// stored numbers are kept as they are and read in the new currency
		cur.CurrencyID = currency
		if keyChanged {
			others, err := s.repo.ListByCategory(ctx, cur.CategoryID)
			if err != nil {
				return err
			}
			if err := checkKeys(cur, others); err != nil {
				return err
			}
		}
		var added []model.DebtTerms
		switch {
		case !cur.IsSetUp() && in.hasOrigin:
			if added, err = in.setupTerms(model.DebtTerms{Payment: cur.Payment, Schedule: cur.Schedule}, today, digits); err != nil {
				return err
			}
			cur.AmountBorrowed = added[0].Balance
			cur.SetTerms(added[len(added)-1])
		case cur.IsSetUp():
			if err := s.correctTerms(ctx, cur, in, today, digits); err != nil {
				return err
			}
		default:
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
		for _, t := range added {
			if err := s.repo.AddSnapshot(ctx, s.snapshotOf(cur.ID, t, now)); err != nil {
				return err
			}
		}
		// splits are in the debt's currency: the next sync splits again
		if currencyChanged || (!wasSetUp && cur.IsSetUp()) {
			if err := s.dropSplits(ctx, cur.ID); err != nil {
				return err
			}
		}
		if !wasSetUp && cur.IsSetUp() {
			// the debt took every month of its former category before setup,
			// whatever key the same request gives it now
			if wasWhole {
				if err := s.moveTailToCategory(ctx, cur, wasCategory, oldCurrency); err != nil {
					return err
				}
			}
		}
		if !wasFirst.IsZero() {
			if err := s.moveHead(ctx, cur, wasCategory, wasFirst, oldCurrency); err != nil {
				return err
			}
		}
		// after the moves back, which find the rows still in the old currency
		if currencyChanged {
			if err := s.setRowsCurrency(ctx, cur.ID, cur.CurrencyID); err != nil {
				return err
			}
		}
		// Now covering a whole category it did not cover before, the debt
		// takes that category's plans, as a conversion does. Going the other
		// way the debt keeps its own: they plan the loan's payments.
		isWhole, err := s.wholeCategory(ctx, cur)
		if err != nil {
			return err
		}
		if isWhole && (!wasWhole || !cur.CategoryID.Equal(wasCategory)) {
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
		return s.repo.AddSnapshot(ctx, s.snapshotOf(cur.ID, *cur.Current, now))
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

// correctTerms applies a body to a set-up debt: a fact that differs from the
// stored origin (amounts compared as the request can state them in the
// debt's currency) rewrites the origin and, with the corrected payment,
// schedule and last payment date, every later snapshot, their dates and
// balances kept, and drops every split so the next sync splits again. A
// statement balance is then written as of its date. cur's current terms
// follow the latest snapshot.
func (s *Service) correctTerms(ctx context.Context, cur *model.Debt, in debtInput, today time.Time, digits int) error {
	snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		return errs.NewNotFound("Debt not found")
	}
	stored := roundedTerms(snapshotTerms(snaps[0]), digits)
	origin := in.origin(stored, digits)
	if termsDiffer(stored, origin) {
		if err := validateOrigin(origin, today, digits); err != nil {
			return err
		}
		for i, sn := range snaps {
			t := origin
			if i > 0 {
				t = snapshotTerms(sn)
				t.Payment, t.Schedule, t.LastPaymentDate = origin.Payment, origin.Schedule, origin.LastPaymentDate
				if err := laterConflict(t, origin); err != nil {
					return err
				}
				if err := validateTerms(t, digits); err != nil {
					return err
				}
			}
			sn.AsOf, sn.Balance, sn.Payment, sn.Schedule, sn.LastPaymentDate = t.AsOf, t.Balance, t.Payment, t.Schedule, t.LastPaymentDate
			if err := s.repo.UpdateSnapshot(ctx, sn); err != nil {
				return err
			}
		}
		cur.AmountBorrowed = origin.Balance
		cur.SetTerms(snapshotTerms(snaps[len(snaps)-1]))
		if err := s.dropSplits(ctx, cur.ID); err != nil {
			return err
		}
	}
	if in.hasOwed {
		return s.writeStatement(ctx, cur, in.statement(snapshotTerms(snaps[len(snaps)-1]), today, digits), today, digits)
	}
	return nil
}

// laterConflict refuses a correction that leaves a statement balance before
// the new start, or on or after the new last payment date while something is
// still owed.
func laterConflict(t, origin model.DebtTerms) error {
	msg := "This conflicts with a balance you entered from a statement. Change or delete that reconciliation first."
	if t.AsOf.Before(origin.AsOf) {
		return fieldError("startDate", msg, errs.CodeDebtConflictsWithReconciliation)
	}
	if t.Balance.IsGreaterThan(vo.NewDecimal("0")) && !t.LastPaymentDate.After(t.AsOf) {
		return fieldError("lastPaymentDate", msg, errs.CodeDebtConflictsWithReconciliation)
	}
	return nil
}

// dropSplits removes a debt's stored splits (the provisional zero-interest
// ones collected before setup, or those a correction or a currency change
// made stale), so the next sync splits those payments by the terms now.
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

// roundedTerms is t with its amounts rounded to digits.
func roundedTerms(t model.DebtTerms, digits int) model.DebtTerms {
	t.Balance, t.Payment = t.Balance.Round(digits), t.Payment.Round(digits)
	return t
}

// notSetUp refuses an action that needs the debt's terms.
func notSetUp() error {
	return &errs.ValidationError{Msg: "Finish setting up this debt first.", MsgCode: errs.CodeDebtNotSetUp}
}
