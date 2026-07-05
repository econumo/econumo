// Update use case: change an account's name/icon/currency, then reconcile its
// balance to the requested value by writing a correction transaction.
package account

import (
	"context"
	"time"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/datetime"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// UpdateAccount updates the account (ownership required) and reconciles its
// balance. If the current computed balance differs from the requested one, it
// writes a correction transaction of (actualBalance - requestedBalance) dated at
// the request's updatedAt, and returns it; otherwise transaction is null.
func (s *Service) UpdateAccount(ctx context.Context, userID vo.Id, req model.UpdateAccountRequest) (*model.UpdateAccountResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newAccountName(req.Name)
	if err != nil {
		return nil, err
	}
	icon, err := newIcon(req.Icon)
	if err != nil {
		return nil, err
	}
	var currencyID *vo.Id
	if req.CurrencyId != nil && *req.CurrencyId != "" {
		cid, perr := vo.ParseId(*req.CurrencyId)
		if perr != nil {
			return nil, perr
		}
		currencyID = &cid
	}
	updatedAt, err := time.Parse(datetime.Layout, req.UpdatedAt)
	if err != nil {
		return nil, errs.NewValidation("Invalid updatedAt",
			errs.FieldError{Key: "updatedAt", Message: "Invalid date format, expected Y-m-d H:i:s"})
	}
	requested := vo.NewDecimal(req.Balance.String())

	var (
		updated    *model.Account
		correction *model.CorrectionResult
	)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		acct, gerr := s.accounts.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !acct.UserID.Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		now := s.clock.Now()
		acct.UpdateName(name, now)
		acct.UpdateIcon(icon, now)
		if currencyID != nil {
			acct.UpdateCurrency(*currencyID, now)
		}
		if serr := s.accounts.Save(ctx, acct); serr != nil {
			return serr
		}

		// reconcile balance: correction = actual - requested. A correction of 0
		// writes nothing.
		actualStr, berr := s.balances.Balance(ctx, id, s.balanceBefore(ctx))
		if berr != nil {
			return berr
		}
		diff := vo.NewDecimal(actualStr).Sub(requested)
		if !diff.IsZero() {
			// Correction sign rule (opposite of a normal transaction): diff =
			// actual - requested. diff<0 means the account has less than requested,
			// so add money -> INCOME(1); diff>0 means it has more, so remove ->
			// EXPENSE(0).
			corrType := int16(0) // expense
			if diff.IsNegative() {
				corrType = 1 // income
			}
			corrID := s.accounts.NextIdentity()
			corr := model.AccountCorrection{
				ID:          corrID,
				UserID:      userID,
				AccountID:   id,
				Description: correctionComment,
				Type:        corrType,
				Amount:      diff.Abs().String(),
				SpentAt:     updatedAt,
				CreatedAt:   now,
			}
			if cerr := s.accounts.SaveCorrection(ctx, corr); cerr != nil {
				return cerr
			}
			typeAlias := "expense"
			if corrType == 1 {
				typeAlias = "income"
			}
			// amountRecipient falls back to amount when null; accountRecipientId/
			// categoryId/payeeId/tagId are null for a balance correction. author is
			// filled in after the tx (needs a UserLookup read).
			correction = &model.CorrectionResult{
				Id:                 corrID.String(),
				Type:               typeAlias,
				AccountId:          id.String(),
				AccountRecipientId: nil,
				Amount:             corr.Amount,
				AmountRecipient:    corr.Amount,
				CategoryId:         nil,
				Description:        correctionComment,
				PayeeId:            nil,
				TagId:              nil,
				Date:               updatedAt.Format(datetime.Layout),
			}
		}
		updated = acct
		return nil
	}); err != nil {
		return nil, err
	}

	folders, err := s.sortedFolders(ctx, userID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.memberships.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	bal, err := s.balances.Balance(ctx, id, s.balanceBefore(ctx))
	if err != nil {
		return nil, err
	}
	item, err := s.buildAccountResult(ctx, userID, updated, bal, folders, memberships, nil)
	if err != nil {
		return nil, err
	}
	// Fill the correction's author (the account owner = the requesting user).
	if correction != nil {
		owner, oerr := s.users.GetOwner(ctx, userID.String())
		if oerr != nil {
			return nil, oerr
		}
		correction.Author = model.UserResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name}
	}
	return &model.UpdateAccountResult{Item: item, Transaction: correction}, nil
}
