package debt

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateDebt creates a set-up debt when the request carries the loan's facts
// (amount borrowed, start date, last payment date): its first snapshot, the
// origin, is as of the start date, and a balance now adds one as of today.
// Without them the debt collects its payments without terms until setup is
// finished.
func (s *Service) CreateDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	in, err := parseInput(debtBody{name: req.Name, currencyID: req.CurrencyId, categoryID: req.CategoryId,
		payeeID: req.PayeeId, tagID: req.TagId, labelID: req.LabelId, start: req.StartDate, borrowed: req.AmountBorrowed,
		balance: req.Balance, payment: req.Payment, schedule: req.Schedule, lastPayment: req.LastPaymentDate})
	if err != nil {
		return nil, err
	}
	if err := in.requirePaymentAndSchedule(); err != nil {
		return nil, err
	}
	if in.hasOwed && !in.hasOrigin {
		return nil, missingFacts(true, in.start == nil, true)
	}
	if err := s.currencies.EnsureUsable(ctx, userID.String(), in.currency.String()); err != nil {
		return nil, err
	}
	digits, err := s.rates.FractionDigits(ctx, in.currency)
	if err != nil {
		return nil, err
	}
	if err := s.checkClassifications(ctx, userID, in); err != nil {
		return nil, err
	}

	loc, err := s.ownerLocation(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	d := &model.Debt{ID: s.repo.NextIdentity(), OwnerID: userID, CurrencyID: in.currency, CategoryID: in.category,
		Name: in.name, PayeeID: in.payee, TagID: in.tag, LabelID: in.label, StartDate: in.start,
		CreatedDay: debtDate(now.In(loc)), CreatedAt: now, UpdatedAt: now}
	var snaps []model.DebtTerms
	if in.hasOrigin {
		if snaps, err = in.setupTerms(model.DebtTerms{Payment: in.payment, Schedule: in.schedule}, s.today(ctx), digits); err != nil {
			return nil, err
		}
		d.AmountBorrowed = snaps[0].Balance
		d.SetTerms(snaps[len(snaps)-1])
	} else {
		d.Payment, d.Schedule = in.payment.Round(digits), in.schedule
		if err := validatePayment(d.Payment); err != nil {
			return nil, err
		}
	}
	err = s.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockOwner(ctx, userID); err != nil {
			return err
		}
		others, err := s.repo.ListByCategory(ctx, d.CategoryID)
		if err != nil {
			return err
		}
		if err := checkKeys(d, others); err != nil {
			return err
		}
		if err := s.repo.Save(ctx, d); err != nil {
			return err
		}
		for _, t := range snaps {
			if err := s.repo.AddSnapshot(ctx, s.snapshotOf(d.ID, t, now)); err != nil {
				return err
			}
		}
		if isWholeCategory(d) {
			return s.moveToDebt(ctx, d)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.resultOf(ctx, d)
}

// setupTerms are the snapshots that set a debt up: the origin from the body's
// facts (base filling the payment and schedule it leaves out) and, with a
// balance now, one as of today under the same terms.
func (in debtInput) setupTerms(base model.DebtTerms, today time.Time, digits int) ([]model.DebtTerms, error) {
	origin := in.origin(base, digits)
	if err := validateOrigin(origin, today, digits); err != nil {
		return nil, err
	}
	out := []model.DebtTerms{origin}
	if in.hasOwed {
		owed := in.owedNow(origin, today, digits)
		if err := validateTerms(owed, digits); err != nil {
			return nil, err
		}
		out = append(out, owed)
	}
	return out, nil
}
