package debt

import (
	"context"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

func (s *Service) CreateDebt(ctx context.Context, userID vo.Id, req model.CreateDebtRequest) (*model.DebtResult, error) {
	in, err := parseInput(req.Name, req.CurrencyId, req.CategoryId, req.PayeeId, req.TagId, req.StartDate,
		req.Balance, req.Payment, req.Schedule, req.LastPaymentDate)
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
	if err := s.checkClassifications(ctx, userID, in.category, in.payee, in.tag); err != nil {
		return nil, err
	}
	terms := in.terms(s.today(ctx), digits)
	if err := validateTerms(terms); err != nil {
		return nil, err
	}

	now := s.clock.Now()
	d := &model.Debt{ID: s.repo.NextIdentity(), OwnerID: userID, CurrencyID: in.currency, CategoryID: in.category,
		Name: in.name, PayeeID: in.payee, TagID: in.tag, StartDate: in.start, CreatedAt: now, UpdatedAt: now}
	d.SetTerms(terms)
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
		if err := s.repo.AddSnapshot(ctx, s.snapshotOf(d, now)); err != nil {
			return err
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
