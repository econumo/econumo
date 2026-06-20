package connection

import (
	"context"

	"github.com/econumo/econumo/internal/domain/shared/vo"
)

// RevokeAccountAccess removes a connected user's grant on an account the
// requesting user owns (or admins). It also unwinds the affected user's view of
// the account: removes it from their folders and drops their accounts_options
// row -- mirroring PHP ConnectionAccountService::revokeAccountAccess.
func (s *Service) RevokeAccountAccess(ctx context.Context, userID vo.Id, req RevokeAccountAccessRequest) (*RevokeAccountAccessResult, error) {
	accountID, err := parseID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	affectedUserID, err := parseID("userId", req.UserId)
	if err != nil {
		return nil, err
	}

	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		return nil, err
	}

	if err := s.revokeGrant(ctx, accountID, affectedUserID); err != nil {
		return nil, err
	}
	return &RevokeAccountAccessResult{}, nil
}

// RevokeOwnAccess removes the caller's OWN grant on a shared account (the
// delete-account non-owner branch in PHP: connectionAccountService->
// revokeAccountAccess($userId, $accountId)). No owner/admin gate -- the caller is
// dropping their own access. The account module calls this via a port.
func (s *Service) RevokeOwnAccess(ctx context.Context, userID, accountID vo.Id) error {
	return s.revokeGrant(ctx, accountID, userID)
}

// revokeGrant removes affectedUserID's grant on accountID and unwinds their view
// of it (folder memberships + accounts_options), inside one tx. Shared by
// RevokeAccountAccess and RevokeOwnAccess. Mirrors PHP
// ConnectionAccountService::revokeAccountAccess.
func (s *Service) revokeGrant(ctx context.Context, accountID, affectedUserID vo.Id) error {
	return s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// Loads the grant first so a missing one surfaces NotFound (matches PHP,
		// which does accountAccessRepository->get before the cleanup).
		if _, gerr := s.access.Get(txCtx, accountID, affectedUserID); gerr != nil {
			return gerr
		}

		folderIDs, ferr := s.folders.FoldersContaining(txCtx, affectedUserID, accountID)
		if ferr != nil {
			return ferr
		}
		for _, fid := range folderIDs {
			if rerr := s.folders.RemoveAccount(txCtx, fid, accountID); rerr != nil {
				return rerr
			}
		}

		if oerr := s.access.DeleteOption(txCtx, accountID, affectedUserID); oerr != nil {
			return oerr
		}
		return s.access.Delete(txCtx, accountID, affectedUserID)
	})
}
