package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateDebt creates a set-up debt when the request carries a balance and a
// last payment date, else one that collects its payments without terms
// until setup is finished.
func (s *Service) CreateDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	in, err := parseInput(debtBody{name: req.Name, currencyID: req.CurrencyId, categoryID: req.CategoryId,
		payeeID: req.PayeeId, tagID: req.TagId, labelID: req.LabelId, start: req.StartDate,
		balance: req.Balance, payment: req.Payment, schedule: req.Schedule, lastPayment: req.LastPaymentDate})
	if err != nil {
		return nil, err
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

	now := s.clock.Now()
	d := &model.Debt{ID: s.repo.NextIdentity(), OwnerID: userID, CurrencyID: in.currency, CategoryID: in.category,
		Name: in.name, PayeeID: in.payee, TagID: in.tag, LabelID: in.label, StartDate: in.start, CreatedAt: now, UpdatedAt: now}
	if in.hasTerms {
		terms := in.terms(s.today(ctx), digits)
		if err := validateTerms(terms); err != nil {
			return nil, err
		}
		d.SetTerms(terms)
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
		if d.IsSetUp() {
			if err := s.repo.AddSnapshot(ctx, s.snapshotOf(d, now)); err != nil {
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
