package debt

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateDebt saves a debt's name, icon, currency, rules, start date and terms. A
// debt not set up is set up by a body carrying the loan's facts (amount
// borrowed, start date, last payment date; the origin as of the start date,
// plus a snapshot as of today with a balance now); without them only its
// payment, schedule and start date change, each of the first two kept when
// left out. On a set-up debt each fact left out keeps the stored one, so a
// stale screen renaming the debt cannot write old ones back; a fact that
// differs is a correction: the origin is rewritten (a later snapshot taking
// the corrected payment, schedule and last payment date, its balance kept)
// and every payment is split again. A balance now appends a snapshot as of
// today. Rules left out are kept; a rule list replaces them.
func (s *Service) UpdateDebt(ctx context.Context, userID vo.Id, req model.UpdateDebtRequest) (*model.DebtResult, error) {
	return inOneTx(ctx, s, func(ctx context.Context) (*model.DebtResult, error) { return s.updateDebt(ctx, userID, req) })
}

func (s *Service) updateDebt(ctx context.Context, userID vo.Id, req model.UpdateDebtRequest) (*model.DebtResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	var rules []ruleInput
	if req.Rules != nil {
		if len(req.Rules) == 0 {
			return nil, fieldError("rules", "This value should not be blank.", errs.CodeIsBlank)
		}
		if len(req.Rules) > maxRules {
			return nil, tooManyRules()
		}
		if rules, err = parseRules(req.Rules); err != nil {
			return nil, err
		}
	}
	in, err := parseInput(debtBody{name: req.Name, currencyID: req.CurrencyId, start: req.StartDate, borrowed: req.AmountBorrowed,
		balance: req.Balance, balanceDate: req.BalanceDate, payment: req.Payment, schedule: req.Schedule, lastPayment: req.LastPaymentDate})
	if err != nil {
		return nil, err
	}
	_, b, err := s.editableDebt(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	// Payments not split yet are split under the terms they were made under,
	// before a new snapshot can change them.
	if _, err := s.syncBudget(ctx, scopeOf(b)); err != nil {
		return nil, err
	}

	today, now := s.today(ctx), s.clock.Now()
	var saved *model.Debt
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		cur, b, err := s.lockedDebt(ctx, userID, id)
		if err != nil {
			return err
		}
		// the amounts in the request are in the currency the debt ends up in
		oldCurrency, currency := cur.CurrencyID, cur.CurrencyID
		if !in.currency.IsZero() && !in.currency.Equal(cur.CurrencyID) {
			if currency, err = s.debtCurrency(ctx, userID, b, in.currency); err != nil {
				return err
			}
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
		budgetDebts, err := s.repo.ListByBudget(ctx, cur.BudgetID)
		if err != nil {
			return err
		}
		wasSole, hasSole := model.SoleWholeCategory(cur, budgetDebts)
		wasWhole := wholeCategories(cur, budgetDebts)
		wasSetUp := cur.IsSetUp()
		// the start of the months the debt row shows, before this update
		var wasFirst time.Time
		if len(wasWhole) > 0 && !sameDay(cur.StartDate, in.start) {
			if wasFirst, _, err = s.planningDays(ctx, cur); err != nil {
				return err
			}
		}
		cur.Name, cur.StartDate = in.name, in.start
		if icon := strings.TrimSpace(req.Icon); icon != "" {
			cur.Icon = icon
		}
		var fresh []model.DebtRule
		if rules != nil {
			cur.Rules, fresh = s.mergeRules(cur.Rules, rules, now)
		}
		if err := s.writeRules(ctx, b, cur, fresh, budgetDebts); err != nil {
			return err
		}
		// stored numbers are kept as they are and read in the new currency
		cur.CurrencyID = currency
		var added []model.DebtTerms
		recheck := false
		switch {
		case !cur.IsSetUp() && in.hasOrigin:
			if added, err = in.setupTerms(model.DebtTerms{Payment: cur.Payment, Schedule: cur.Schedule}, today, digits); err != nil {
				return err
			}
			cur.AmountBorrowed = added[0].Balance
			cur.SetTerms(added[len(added)-1])
		case cur.IsSetUp():
			if recheck, err = s.correctTerms(ctx, cur, in, today, digits); err != nil {
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
		// rate changes keep their own rate and payment over corrected terms
		if recheck {
			if err := s.checkRepayable(ctx, cur); err != nil {
				return err
			}
		}
		// the debt took every month of the category it covered alone before
		// setup, whatever rules the same request gives it now
		if !wasSetUp && cur.IsSetUp() && hasSole {
			if err := s.moveTailToCategory(ctx, cur, wasSole, oldCurrency); err != nil {
				return err
			}
		}
		isWhole := wholeCategories(cur, budgetDebts)
		if !wasFirst.IsZero() {
			var sole *vo.Id
			if hasSole {
				sole = &wasSole
			}
			if err := s.moveHead(ctx, cur, sole, intersect(isWhole, wasWhole), wasFirst, oldCurrency); err != nil {
				return err
			}
		}
		// after the moves back, which find the rows still in the old currency
		if currencyChanged {
			if err := s.setRowsCurrency(ctx, cur.ID, cur.CurrencyID); err != nil {
				return err
			}
		}
		// A rule now covering a whole category it did not cover before takes
		// that category's plans, as a conversion does. A rule removed or
		// narrowed leaves the debt's own: they plan the loan's payments.
		for _, c := range isWhole {
			if !slices.ContainsFunc(wasWhole, c.Equal) {
				if err := s.moveToDebt(ctx, cur, c); err != nil {
					return err
				}
			}
		}
		saved = cur
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, b, saved)
}

// wholeCategories are the categories d has a whole-category rule on.
func wholeCategories(d *model.Debt, budgetDebts []*model.Debt) []vo.Id {
	var out []vo.Id
	for _, r := range d.Rules {
		if model.IsWholeCategoryRule(d, r, budgetDebts) && !slices.ContainsFunc(out, r.CategoryID.Equal) {
			out = append(out, r.CategoryID)
		}
	}
	return out
}

func intersect(a, b []vo.Id) []vo.Id {
	var out []vo.Id
	for _, id := range a {
		if slices.ContainsFunc(b, id.Equal) {
			out = append(out, id)
		}
	}
	return out
}

// MarkDebtPaidOff writes a zero-balance snapshot as of today, keeping the
// payment, schedule and last payment date (the rate is irrelevant at zero).
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
		// a statement of 0 as of today: it replaces one already entered today,
		// and the splits after today are dropped like any statement's
		zero := *cur.Current
		zero.AsOf, zero.Balance = today, vo.NewDecimal("0")
		if err := s.storeStatement(ctx, cur, zero); err != nil {
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

// stateNow is the debt's state from what is stored, splitting new payments
// in memory: it writes nothing, so it can run inside a write's transaction.
func (s *Service) stateNow(ctx context.Context, d *model.Debt) (*debtState, error) {
	scope, err := s.scope(ctx, d.BudgetID)
	if err != nil {
		return nil, err
	}
	debts, err := s.repo.ListByBudget(ctx, d.BudgetID)
	if err != nil {
		return nil, err
	}
	states, err := s.computeDebts(ctx, scope, debts, false)
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
// follow the latest snapshot. A start after a rate change is refused; recheck:
// the terms changed under rate changes, so the caller checks the debt is still
// repaid once they are saved.
func (s *Service) correctTerms(ctx context.Context, cur *model.Debt, in debtInput, today time.Time, digits int) (recheck bool, err error) {
	snaps, err := s.repo.Snapshots(ctx, []vo.Id{cur.ID})
	if err != nil {
		return false, err
	}
	if len(snaps) == 0 {
		return false, errs.NewNotFound("Debt not found")
	}
	changes, err := s.repo.RateChanges(ctx, []vo.Id{cur.ID})
	if err != nil {
		return false, err
	}
	stored := roundedTerms(snapshotTerms(snaps[0]), digits)
	origin := in.origin(stored, digits)
	if termsDiffer(stored, origin) {
		if err := startRateConflict(changes, stored.AsOf, origin.AsOf); err != nil {
			return false, err
		}
		if err := validateOrigin(origin, today, digits); err != nil {
			return false, err
		}
		if changes, err = s.followStartRate(ctx, cur.ID, changes, stored, origin); err != nil {
			return false, err
		}
		for i, sn := range snaps {
			t := origin
			if i > 0 {
				t = snapshotTerms(sn)
				t.Payment, t.Schedule, t.LastPaymentDate = origin.Payment, origin.Schedule, origin.LastPaymentDate
				if err := laterConflict(t, origin); err != nil {
					return false, err
				}
				if err := validateTerms(t, origin, digits, changes); err != nil {
					return false, err
				}
			}
			sn.AsOf, sn.Balance, sn.Payment, sn.Schedule, sn.LastPaymentDate = t.AsOf, t.Balance, t.Payment, t.Schedule, t.LastPaymentDate
			if err := s.repo.UpdateSnapshot(ctx, sn); err != nil {
				return false, err
			}
		}
		cur.AmountBorrowed = origin.Balance
		cur.SetTerms(snapshotTerms(snaps[len(snaps)-1]))
		if err := s.dropSplits(ctx, cur.ID); err != nil {
			return false, err
		}
		recheck = len(changes) > 0
	}
	if in.hasOwed {
		return recheck, s.writeStatement(ctx, cur, in.statement(snapshotTerms(snaps[len(snaps)-1]), today, digits), today, digits)
	}
	return recheck, nil
}

// startRateConflict refuses a start after a rate change other than the one on
// the old start, which moves with it (followStartRate).
func startRateConflict(changes []*model.DebtRateChange, oldStart, newStart time.Time) error {
	for _, c := range changes {
		if c.EffectiveDate.Before(newStart) && !c.EffectiveDate.Equal(oldStart) {
			return fieldError("startDate", "This start date comes after a rate change. Delete that rate change first.", errs.CodeDebtConflictsWithRateChange)
		}
	}
	return nil
}

// followStartRate keeps the rate change on the start date the start's: it
// moves with a corrected start date (giving way to a change already on the
// new one) and takes a corrected payment, its rate kept. It returns the
// changes as stored after.
func (s *Service) followStartRate(ctx context.Context, debtID vo.Id, changes []*model.DebtRateChange, stored, origin model.DebtTerms) ([]*model.DebtRateChange, error) {
	var start *model.DebtRateChange
	taken := false
	for _, c := range changes {
		switch {
		case c.EffectiveDate.Equal(stored.AsOf):
			start = c
		case c.EffectiveDate.Equal(origin.AsOf):
			taken = true
		}
	}
	if start != nil && !origin.AsOf.Equal(stored.AsOf) {
		if _, err := s.repo.DeleteRateChange(ctx, debtID, start.ID); err != nil {
			return nil, err
		}
		if !taken {
			start.EffectiveDate = origin.AsOf
			if err := s.repo.SaveRateChange(ctx, start); err != nil {
				return nil, err
			}
		}
	}
	if !origin.Payment.Equals(stored.Payment) {
		for _, c := range changes {
			if c.EffectiveDate.Equal(origin.AsOf) {
				c.Payment = origin.Payment
				if err := s.repo.SaveRateChange(ctx, c); err != nil {
					return nil, err
				}
			}
		}
	}
	return s.repo.RateChanges(ctx, []vo.Id{debtID})
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
