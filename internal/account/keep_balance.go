// Keep-balance use cases: a CSV import of older history would move the balance
// the user typed on Day 0, so the client snapshots balances before the import
// and restores them afterwards.
package account

import (
	"context"
	"errors"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetBalanceSnapshot reads each account's balance and whether it is still in
// its Day 0 state. Ids the caller may not write are omitted.
func (s *Service) GetBalanceSnapshot(ctx context.Context, userID vo.Id, accountIDs []string) (*model.BalanceSnapshotResult, error) {
	res := &model.BalanceSnapshotResult{
		TakenAt: s.clock.Now().UTC().Format(datetime.Layout),
		Items:   []model.BalanceSnapshotItem{},
	}
	seen := map[string]bool{}
	for _, raw := range accountIDs {
		id, err := vo.ParseId(raw)
		if err != nil || seen[id.String()] {
			continue
		}
		seen[id.String()] = true
		ok, err := s.canRestoreBalance(ctx, userID, id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		bal, err := s.balances.Balance(ctx, id, s.balanceBefore(ctx))
		if err != nil {
			return nil, err
		}
		count, onlyID, onlyIsCorrection, err := s.accounts.AccountTransactionShape(ctx, id)
		if err != nil {
			return nil, err
		}
		item := model.BalanceSnapshotItem{AccountId: id.String(), Balance: bal, Fresh: count == 0}
		if count == 1 && onlyIsCorrection {
			item.Fresh = true
			item.OpeningCorrectionId = onlyID
		}
		res.Items = append(res.Items, item)
	}
	return res, nil
}

// canRestoreBalance applies update-account's gate (owner or accepted admin) to
// a live account, answering false instead of a not-found/denied error.
func (s *Service) canRestoreBalance(ctx context.Context, userID, accountID vo.Id) (bool, error) {
	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		var nf *errs.NotFoundError
		var denied *errs.AccessDeniedError
		if errors.As(err, &nf) || errors.As(err, &denied) {
			return false, nil
		}
		return false, err
	}
	acct, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return false, err
	}
	return !acct.IsDeleted, nil
}

// RestoreBalance brings the account back to req.Balance after an import that
// started at req.Since. The difference lands just before the earliest
// transaction created since then: the untouched opening correction absorbs it
// (and is removed when it nets to zero); otherwise a new correction carries it.
func (s *Service) RestoreBalance(ctx context.Context, userID vo.Id, req model.RestoreBalanceRequest) (*model.RestoreBalanceResult, error) {
	id, err := vo.ParseId(req.AccountId)
	if err != nil {
		return nil, err
	}
	since, err := time.Parse(datetime.Layout, req.Since)
	if err != nil {
		return nil, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "since", Message: "Invalid date format, expected Y-m-d H:i:s", Code: errs.CodeInvalidDatetimeFormat})
	}
	requested := vo.NewDecimal(req.Balance)

	var (
		acct     *model.Account
		restored bool
	)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		if aerr := s.requireOwnerAdmin(ctx, userID, id); aerr != nil {
			return aerr
		}
		a, gerr := s.accounts.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if a.IsDeleted {
			return errs.NewNotFound("Account not found")
		}
		acct = a
		current, berr := s.balances.Balance(ctx, id, s.balanceBefore(ctx))
		if berr != nil {
			return berr
		}
		delta := requested.Sub(vo.NewDecimal(current))
		if delta.IsZero() {
			return nil
		}
		earliest, found, eerr := s.accounts.EarliestSpentSince(ctx, id, since, req.OpeningCorrectionId)
		if eerr != nil || !found {
			return eerr
		}
		at := earliest.Add(-time.Second)
		now := s.clock.Now()
		restored = true

		if req.OpeningCorrectionId != "" {
			opening, ok, cerr := s.accounts.GetCorrection(ctx, req.OpeningCorrectionId, id.String())
			if cerr != nil {
				return cerr
			}
			if ok {
				signed := vo.NewDecimal(opening.Amount)
				if opening.Type == 0 {
					signed = vo.NewDecimal("0").Sub(signed)
				}
				signed = signed.Add(delta)
				if signed.IsZero() {
					return s.accounts.DeleteCorrection(ctx, opening.ID.String(), id.String())
				}
				opening.Type = correctionType(signed)
				opening.Amount = signed.Abs().String()
				opening.SpentAt = at
				opening.UpdatedAt = now
				return s.accounts.UpdateCorrection(ctx, *opening)
			}
		}
		return s.accounts.SaveCorrection(ctx, model.AccountCorrection{
			ID:          s.accounts.NextIdentity(),
			UserID:      userID,
			AccountID:   id,
			Description: correctionComment,
			Type:        correctionType(delta),
			Amount:      delta.Abs().String(),
			SpentAt:     at,
			CreatedAt:   now,
		})
	}); err != nil {
		return nil, err
	}

	item, err := s.freshAccountResult(ctx, userID, acct)
	if err != nil {
		return nil, err
	}
	return &model.RestoreBalanceResult{Item: item, Restored: restored}, nil
}
