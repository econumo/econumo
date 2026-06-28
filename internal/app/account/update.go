// Update use case: change an account's name/icon/currency, then reconcile its
// balance to the requested value by writing a correction transaction.
package account

import (
	"context"
	"time"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// UpdateAccount updates the account (ownership required) and reconciles its
// balance. If the current computed balance differs from the requested one, it
// writes a correction transaction of (actualBalance - requestedBalance) dated at
// the request's updatedAt, and returns it; otherwise transaction is null.
func (s *Service) UpdateAccount(ctx context.Context, userID vo.Id, req UpdateAccountRequest) (*UpdateAccountResult, error) {
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
		updated    *domaccount.Account
		correction *CorrectionResult
	)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		acct, gerr := s.repo.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !acct.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		now := s.clock.Now()
		acct.UpdateName(name, now)
		acct.UpdateIcon(icon, now)
		if currencyID != nil {
			acct.UpdateCurrency(*currencyID, now)
		}
		if serr := s.repo.Save(ctx, acct); serr != nil {
			return serr
		}

		// reconcile balance: correction = actual - requested (PHP
		// AccountService::updateBalance -> transactionService.updateBalance with
		// actualBalance.sub(balance)). A correction of 0 writes nothing.
		actualStr, berr := s.repo.Balance(ctx, id, s.balanceBefore(ctx))
		if berr != nil {
			return berr
		}
		diff := vo.NewDecimal(actualStr).Sub(requested)
		if !diff.IsZero() {
			// PHP createCorrection sign rule: correction < 0 -> INCOME(1),
			// else EXPENSE(0). (Opposite of createTransaction.) diff = actual -
			// requested: diff<0 means the account has less than requested, so add
			// money (income); diff>0 means it has more, so remove (expense).
			corrType := int16(0) // expense
			if diff.IsNegative() {
				corrType = 1 // income
			}
			corrID := s.repo.NextIdentity()
			corr := domaccount.Correction{
				ID:          corrID,
				UserID:      userID,
				AccountID:   id,
				Description: correctionComment,
				Type:        corrType,
				Amount:      diff.Abs().String(),
				SpentAt:     updatedAt,
				CreatedAt:   now,
			}
			if cerr := s.repo.SaveCorrection(ctx, corr); cerr != nil {
				return cerr
			}
			typeAlias := "expense"
			if corrType == 1 {
				typeAlias = "income"
			}
			// Mirror PHP TransactionToDtoResultAssembler exactly: amountRecipient
			// falls back to amount when null; accountRecipientId/categoryId/payeeId/
			// tagId are null for a balance correction. author is filled in after the
			// tx (needs a UserLookup read).
			correction = &CorrectionResult{
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
	memberships, err := s.folders.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	bal, err := s.repo.Balance(ctx, id, s.balanceBefore(ctx))
	if err != nil {
		return nil, err
	}
	item, err := s.buildAccountResult(ctx, userID, updated, bal, folders, memberships, nil)
	if err != nil {
		return nil, err
	}
	// Fill the correction's author (the account owner = the requesting user), to
	// mirror PHP's TransactionToDtoResultAssembler embedding $transaction->getUser().
	if correction != nil {
		owner, oerr := s.users.GetOwner(ctx, userID.String())
		if oerr != nil {
			return nil, oerr
		}
		correction.Author = OwnerResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name}
	}
	return &UpdateAccountResult{Item: item, Transaction: correction}, nil
}
