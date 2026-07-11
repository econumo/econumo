// Read side of the account+folder module: get-account-list and get-folder-list.
// Both build their results from the write-side repos (no separate read model) —
// the account list is small and the embed builder already centralizes the join
// work.
package account

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/vo"
)

// GetAccountList returns all the user's available accounts (each with the full
// embed) in reverse order (the list is reversed before returning).
func (s *Service) GetAccountList(ctx context.Context, userID vo.Id) (*model.GetAccountListResult, error) {
	items, err := s.buildAccountList(ctx, userID, true)
	if err != nil {
		return nil, err
	}
	return &model.GetAccountListResult{Items: items}, nil
}

// AccountListForUser returns the user's available accounts (each with the full
// embed) in reverse order — the same shape get-account-list returns. It is
// exported so other modules (notably transaction, whose create/update/delete
// results embed the full account list) can reuse the embed builder without
// duplicating the join logic.
func (s *Service) AccountListForUser(ctx context.Context, userID vo.Id) ([]model.AccountResult, error) {
	return s.buildAccountList(ctx, userID, true)
}

// AccountOwner returns the owner user id of an account (for cross-module access
// checks, e.g. transaction). Missing -> *errs.NotFoundError (from the repo).
func (s *Service) AccountOwner(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	acct, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return vo.Id{}, err
	}
	return acct.UserID, nil
}

// AccountCurrency returns an account's currency id (for cross-module use, e.g.
// transaction's transfer amount normalization). Missing -> *errs.NotFoundError
// (from the repo).
func (s *Service) AccountCurrency(ctx context.Context, accountID vo.Id) (vo.Id, error) {
	acct, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return vo.Id{}, err
	}
	return acct.CurrencyID, nil
}

// VisibleAccountIDs returns the ids of the user's available (non-deleted)
// accounts that are NOT in a hidden folder — the set whose transactions the user
// may list.
func (s *Service) VisibleAccountIDs(ctx context.Context, userID vo.Id) ([]vo.Id, error) {
	accts, err := s.accounts.ListAvailable(ctx, userID)
	if err != nil {
		return nil, err
	}
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	memberships, err := s.memberships.MembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	// account id -> is it in any hidden folder?
	hidden := make(map[string]bool)
	for _, f := range folders {
		if f.IsVisible {
			continue
		}
		for _, aid := range memberships[f.ID.String()] {
			hidden[aid] = true
		}
	}
	out := make([]vo.Id, 0, len(accts))
	for _, a := range accts {
		if hidden[a.ID.String()] {
			continue
		}
		out = append(out, a.ID)
	}
	return out, nil
}

// GetFolderList returns the user's folders ordered by position.
func (s *Service) GetFolderList(ctx context.Context, userID vo.Id) (*model.GetFolderListResult, error) {
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position < folders[j].Position })
	items := make([]model.AccountFolderResult, 0, len(folders))
	for _, f := range folders {
		items = append(items, toFolderResult(f))
	}
	return &model.GetFolderListResult{Items: items}, nil
}
