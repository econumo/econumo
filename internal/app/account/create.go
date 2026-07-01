// Create use case: create an account (idempotent on the request id), optionally
// seeding its balance via a correction transaction, and place it in a folder.
package account

import (
	"context"
	"strings"

	domaccount "github.com/econumo/econumo/internal/domain/account"
	"github.com/econumo/econumo/internal/domain/shared/datetime"
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
// accounts); create the account + its accounts_options row; place it in a folder
// (the requested one, which must be owned by the user, or a freshly-created
// default folder when this is the user's first account); if the requested balance is
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
	balance := vo.NewDecimal(req.Balance.String())

	var (
		created    *domaccount.Account
		correction *CorrectionResult
	)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		already, cerr := s.ops.Claim(ctx, opID, s.clock.Now())
		if cerr != nil {
			return cerr
		}
		if already {
			return errs.NewValidation("Operation is locked")
		}

		// position: append after the current last, i.e. maxPos+1. When the user has
		// no accounts_options rows yet (maxPos==0 — no own accounts, or only
		// option-less shared ones), fall back to the count of available accounts so
		// the new account still sorts last.
		maxPos, perr := s.repo.MaxPosition(ctx, userID)
		if perr != nil {
			return perr
		}
		position := maxPos + 1
		if maxPos == 0 {
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

		// place in a folder (auto-creating the user's first one when needed).
		folderID, ferr := s.resolveAccountFolder(ctx, userID, req.FolderId)
		if ferr != nil {
			return ferr
		}
		if aerr := s.folders.AddAccount(ctx, folderID, id); aerr != nil {
			return aerr
		}

		// seed the balance via a correction transaction (only when non-zero).
		if !balance.IsZero() {
			corrID := s.repo.NextIdentity()
			corrType := correctionType(balance)
			spentAt := s.localNow(ctx)
			corr := domaccount.Correction{
				ID:          corrID,
				UserID:      userID,
				AccountID:   id,
				Description: "",
				Type:        corrType,
				Amount:      balance.Abs().String(),
				SpentAt:     spentAt,
				CreatedAt:   now,
			}
			if cerr := s.repo.SaveCorrection(ctx, corr); cerr != nil {
				return cerr
			}
			typeAlias := "expense"
			if corrType == 1 {
				typeAlias = "income"
			}
			// amountRecipient falls back to amount; accountRecipientId/categoryId/
			// payeeId/tagId are null for a balance correction. author is filled in
			// after the tx (needs a UserLookup read).
			correction = &CorrectionResult{
				Id:                 corrID.String(),
				Type:               typeAlias,
				AccountId:          id.String(),
				AccountRecipientId: nil,
				Amount:             corr.Amount,
				AmountRecipient:    corr.Amount,
				CategoryId:         nil,
				Description:        "",
				PayeeId:            nil,
				TagId:              nil,
				Date:               spentAt.Format(datetime.Layout),
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
	// Fill the correction's author (the account owner = the requesting user).
	if correction != nil {
		owner, oerr := s.users.GetOwner(ctx, userID.String())
		if oerr != nil {
			return nil, oerr
		}
		correction.Author = OwnerResult{Id: owner.ID, Avatar: owner.Avatar, Name: owner.Name}
	}
	return &CreateAccountResult{Item: item, Transaction: correction}, nil
}

// defaultFolderName is the folder auto-created for a user's very first account
// (registration/onboarding never creates one). The frontend labels a user's
// primary folder itself, so this is only a fallback name.
const defaultFolderName = "General"

// resolveAccountFolder picks the folder to place a new account in, running in the
// caller's tx. A blank or unknown folderId is tolerated ONLY when the user has no
// folders at all — the first-account/onboarding case — in which a default folder
// is created. Once the user owns any folder, a blank/unknown folderId is the same
// error it always was, preserving the frozen contract.
func (s *Service) resolveAccountFolder(ctx context.Context, userID vo.Id, rawFolderID string) (vo.Id, error) {
	if strings.TrimSpace(rawFolderID) == "" {
		return s.defaultFolderOr(ctx, userID, errs.NewValidation("Validation failed",
			errs.FieldError{Key: "folderId", Message: "This value should not be blank.", Code: "IS_BLANK_ERROR"}))
	}
	folderID, err := vo.ParseId(rawFolderID)
	if err != nil {
		return vo.Id{}, err
	}
	folder, ferr := s.folders.GetByID(ctx, folderID)
	if ferr != nil {
		if _, ok := errs.AsNotFound(ferr); ok {
			return s.defaultFolderOr(ctx, userID, ferr)
		}
		return vo.Id{}, ferr
	}
	if !folder.UserId().Equal(userID) {
		return vo.Id{}, errs.NewAccessDenied("Access denied")
	}
	return folderID, nil
}

// defaultFolderOr returns the id of a freshly-created default folder when the user
// has none yet, otherwise returns whenHasFolders — the error that applies when the
// user already owns folders (blank/unknown folderId is then a real error).
func (s *Service) defaultFolderOr(ctx context.Context, userID vo.Id, whenHasFolders error) (vo.Id, error) {
	count, cerr := s.folders.CountByUser(ctx, userID)
	if cerr != nil {
		return vo.Id{}, cerr
	}
	if count > 0 {
		return vo.Id{}, whenHasFolders
	}
	f, ferr := s.createFolderTx(ctx, userID, defaultFolderName)
	if ferr != nil {
		return vo.Id{}, ferr
	}
	return f.Id(), nil
}

// correctionType returns the transaction type for a balance correction: a
// positive balance is income (1), a negative balance is expense (0).
func correctionType(balance vo.DecimalNumber) int16 {
	if balance.IsNegative() {
		return 0 // expense
	}
	return 1 // income
}
