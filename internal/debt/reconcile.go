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

// ReconcileDebt states a set-up debt's balance from a statement, as of its
// date (today when none): a snapshot under the latest terms that overrides
// what the recorded payments give from that date on, at the loan's rate, so
// it moves the payoff rather than the rate. A statement on a date that
// already has one replaces it.
func (s *Service) ReconcileDebt(ctx context.Context, userID vo.Id, req model.ReconcileDebtRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.reconcileDebt(ctx, userID, req) })
}

func (s *Service) reconcileDebt(ctx context.Context, userID vo.Id, req model.ReconcileDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	in := debtInput{owed: vo.NewDecimal(req.Balance.String()), hasOwed: true}
	if strings.TrimSpace(req.BalanceDate) != "" {
		if in.owedOn, err = time.Parse(datetime.DateLayout, req.BalanceDate); err != nil {
			return nil, err
		}
	}
	return s.withSetUpDebt(ctx, userID, id, func(ctx context.Context, cur *model.Debt, snaps []*model.DebtSnapshot, today time.Time, digits int) error {
		return s.writeStatement(ctx, cur, in.statement(snapshotTerms(snaps[len(snaps)-1]), today, digits), today, digits)
	})
}

// DeleteDebtReconciliation removes a statement balance; the payments after
// its date are split again from what comes before it. The origin is no
// statement and cannot be deleted.
func (s *Service) DeleteDebtReconciliation(ctx context.Context, userID vo.Id, req model.DeleteDebtReconciliationRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) {
		return s.deleteDebtReconciliation(ctx, userID, req)
	})
}

func (s *Service) deleteDebtReconciliation(ctx context.Context, userID vo.Id, req model.DeleteDebtReconciliationRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	snapID, err := vo.ParseId(req.ReconciliationId)
	if err != nil {
		return nil, err
	}
	return s.withSetUpDebt(ctx, userID, id, func(ctx context.Context, cur *model.Debt, snaps []*model.DebtSnapshot, _ time.Time, _ int) error {
		for i, sn := range snaps {
			if i == 0 || !sn.ID.Equal(snapID) {
				continue
			}
			if err := s.repo.DeleteSnapshot(ctx, sn.ID); err != nil {
				return err
			}
			if err := s.dropSplitsAfter(ctx, cur, sn.AsOf); err != nil {
				return err
			}
			return s.followLatest(ctx, cur)
		}
		return errs.NewNotFound("Reconciliation not found")
	})
}

// withSetUpDebt runs write on an active set-up debt the user may edit, under its
// budget's lock, after its pending payments are split under the terms they
// were made under, then saves the debt and presents it.
func (s *Service) withSetUpDebt(ctx context.Context, userID, id vo.Id,
	write func(ctx context.Context, cur *model.Debt, snaps []*model.DebtSnapshot, today time.Time, digits int) error) (*model.DebtResult, error) {
	d, b, err := s.editableDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if d.IsPaidOff() {
		return nil, paidOff()
	}
	if !d.IsSetUp() {
		return nil, notSetUp()
	}
	if _, err := s.syncBudget(ctx, scopeOf(b)); err != nil {
		return nil, err
	}
	today, now := s.today(ctx), s.clock.Now()
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		cur, _, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		if cur.IsPaidOff() {
			return paidOff()
		}
		if !cur.IsSetUp() {
			return notSetUp()
		}
		digits, err := s.rates.FractionDigits(ctx, cur.CurrencyID)
		if err != nil {
			return err
		}
		snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
		if err != nil {
			return err
		}
		if len(snaps) == 0 {
			return errs.NewNotFound("Debt not found")
		}
		if err := write(ctx, cur, snaps, today, digits); err != nil {
			return err
		}
		cur.UpdatedAt = now
		d = cur
		return s.repo.Save(ctx, cur)
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, b, d)
}

// validateStatement checks a statement balance as of t.AsOf, which must lie
// after the loan's start (a balance on the start day is the amount borrowed,
// corrected in the loan's facts) and no later than today.
func validateStatement(t, origin model.DebtTerms, today time.Time, digits int, changes []*model.DebtRateChange) error {
	if !t.AsOf.After(origin.AsOf) || t.AsOf.After(today) {
		return fieldError("balanceDate", "This value is out of range.", errs.CodeOutOfRange)
	}
	return validateTerms(t, origin, digits, changes)
}

// writeStatement validates a statement balance on a set-up debt and stores it.
func (s *Service) writeStatement(ctx context.Context, cur *model.Debt, t model.DebtTerms, today time.Time, digits int) error {
	snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		return errs.NewNotFound("Debt not found")
	}
	changes, err := s.repo.RateChanges(ctx, []vo.Id{cur.ID})
	if err != nil {
		return err
	}
	t.Payment = paymentInForce(snaps, changes, t.AsOf, digits)
	if err := validateStatement(t, snapshotTerms(snaps[0]), today, digits, changes); err != nil {
		return err
	}
	return s.storeStatement(ctx, cur, t)
}

// paymentInForce is the regular payment in force on day, the one a statement
// stores; the math never reads it back (WithRates).
func paymentInForce(snaps []*model.DebtSnapshot, changes []*model.DebtRateChange, day time.Time, digits int) vo.DecimalNumber {
	later := make([]model.DebtTerms, 0, len(snaps)-1)
	for _, sn := range snaps[1:] {
		later = append(later, snapshotTerms(sn))
	}
	return decimalOf(model.PaymentOn(mathTerms(changes, digits, snapshotTerms(snaps[0]), later...), day), digits)
}

// storeStatement replaces the statement of the same date (never the origin,
// the first snapshot), else adds one; the splits of the payments after its
// date are dropped so the next sync splits them from it, and the debt's
// current terms follow the latest snapshot.
func (s *Service) storeStatement(ctx context.Context, cur *model.Debt, t model.DebtTerms) error {
	snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		return errs.NewNotFound("Debt not found")
	}
	changes, err := s.repo.RateChanges(ctx, []vo.Id{cur.ID})
	if err != nil {
		return err
	}
	digits, err := s.rates.FractionDigits(ctx, cur.CurrencyID)
	if err != nil {
		return err
	}
	t.Payment = paymentInForce(snaps, changes, t.AsOf, digits)
	replaced := false
	for _, sn := range snaps[1:] {
		if sn.AsOf.Equal(t.AsOf) {
			sn.Balance, sn.Payment, sn.Schedule, sn.LastPaymentDate = t.Balance, t.Payment, t.Schedule, t.LastPaymentDate
			if err := s.repo.UpdateSnapshot(ctx, sn); err != nil {
				return err
			}
			replaced = true
			break
		}
	}
	if !replaced {
		if err := s.repo.AddSnapshot(ctx, s.snapshotOf(cur.ID, t, s.clock.Now())); err != nil {
			return err
		}
	}
	if err := s.dropSplitsAfter(ctx, cur, t.AsOf); err != nil {
		return err
	}
	return s.followLatest(ctx, cur)
}

// followLatest copies the latest snapshot into the debt's current terms,
// keeping the origin's payment: a statement stores the payment in force on its
// date, which a rate change may have set, while the debt's own payment is the
// loan contract's.
func (s *Service) followLatest(ctx context.Context, cur *model.Debt) error {
	snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		return errs.NewNotFound("Debt not found")
	}
	t := snapshotTerms(snaps[len(snaps)-1])
	t.Payment = snaps[0].Payment
	cur.SetTerms(t)
	return nil
}

// dropSplitsAfter removes the stored splits of the debt's payments dated
// after day: a snapshot written or removed as of day changes how they split.
func (s *Service) dropSplitsAfter(ctx context.Context, d *model.Debt, day time.Time) error {
	return s.dropSplitsFrom(ctx, d, debtDate(day).AddDate(0, 0, 1))
}

// dropSplitsFrom removes the stored splits of the debt's payments dated on or
// after day.
func (s *Service) dropSplitsFrom(ctx context.Context, d *model.Debt, day time.Time) error {
	st, err := s.stateNow(ctx, d)
	if err != nil {
		return err
	}
	var ids []vo.Id
	for _, p := range st.pays {
		if !p.date.Before(day) {
			ids = append(ids, p.txID)
		}
	}
	return s.repo.DeleteSplits(ctx, d.ID, ids)
}
