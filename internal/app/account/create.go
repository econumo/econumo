// Create use case: create an account (idempotent on the request id), optionally
// seeding its balance via a correction transaction, and place it in a folder.
package account

import (
	"context"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/errs"
	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// CreateAccount creates an account for the current user and returns {item}.
//
// The request `id` is the OPERATION/idempotency id, NOT the entity id: a FRESH
// UUIDv7 is minted for the account, and req.Id is used only to Claim/MarkHandled
// the operation guard.
//
// Steps inside one tx: claim the operation id (idempotency); compute the
// position (max accounts_options.position for the user, else count of available
// accounts); create the account + its accounts_options row; add it to the
// requested folder (which must be owned by the user); if the requested balance is
// non-zero, write a balance-correction transaction dated at the account's
// creation time; mark the operation handled. Returns {item} only (no accounts
// list).
func (s *Service) CreateAccount(ctx context.Context, userID vo.Id, req CreateAccountRequest) (*CreateAccountResult, error) {
	opID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	id := vo.NewId() // fresh entity id (UUIDv7); req.Id is the operation id only
	name, err := newAccountName(req.Name)
	if err != nil {
		return nil, err
	}
	currencyID, err := vo.ParseId(req.CurrencyId)
	if err != nil {
		return nil, err
	}
	icon, err := newIcon(req.Icon)
	if err != nil {
		return nil, err
	}
	folderID, err := vo.ParseId(req.FolderId)
	if err != nil {
		return nil, err
	}
	balance := vo.NewDecimal(req.Balance.String())

	var created *domaccount.Account
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}

		// position: highest existing accounts_options.position for the user, or
		// (when none) the count of available accounts.
		maxPos, perr := s.repo.MaxPosition(ctx, userID)
		if perr != nil {
			return perr
		}
		position := maxPos
		if position == 0 {
			n, cerr := s.repo.CountAvailable(ctx, userID)
			if cerr != nil {
				return cerr
			}
			position = int16(n)
		}

		now := s.clock.Now()
		acct := domaccount.NewAccount(id, userID, currencyID, name, icon, now)
		if serr := s.repo.Save(ctx, acct); serr != nil {
			return serr
		}
		if serr := s.repo.SavePosition(ctx, id, userID, position, now); serr != nil {
			return serr
		}

		// place in the folder (must belong to the user).
		folder, ferr := s.folders.GetByID(ctx, folderID)
		if ferr != nil {
			return ferr
		}
		if !folder.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		if aerr := s.folders.AddAccount(ctx, folderID, id); aerr != nil {
			return aerr
		}

		// seed the balance via a correction transaction (only when non-zero).
		if !balance.IsZero() {
			corr := domaccount.Correction{
				ID:          s.repo.NextIdentity(),
				UserID:      userID,
				AccountID:   id,
				Description: "",
				Type:        correctionType(balance),
				Amount:      balance.Abs().String(),
				SpentAt:     acct.CreatedAt(),
				CreatedAt:   now,
			}
			if cerr := s.repo.SaveCorrection(ctx, corr); cerr != nil {
				return cerr
			}
		}

		if merr := s.ops.MarkHandled(ctx, opID, now); merr != nil {
			return merr
		}
		created = acct
		return nil
	}); err != nil {
		return nil, err
	}

	// Build the result outside the write tx (reads on the pool see the committed
	// rows). Returns {item} ONLY (no accounts list).
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
	item, err := s.buildAccountResult(ctx, userID, created, bal, folders, memberships, nil)
	if err != nil {
		return nil, err
	}
	return &CreateAccountResult{Item: item}, nil
}

// correctionType returns the transaction type for a balance correction: a
// positive balance is income (1), a negative balance is expense (0).
func correctionType(balance vo.DecimalNumber) int16 {
	if balance.IsNegative() {
		return 0 // expense
	}
	return 1 // income
}
