package connection

import (
	"context"
	"errors"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// SetAccountAccess grants or updates a connected user's role on an account the
// requesting user owns (or admins). On a FIRST grant to the affected user it
// also seeds that user's per-account ordering row (accounts_options at max+1)
// and adds the account to their last folder.
func (s *Service) SetAccountAccess(ctx context.Context, userID vo.Id, req model.SetAccountAccessRequest) (*model.SetAccountAccessResult, error) {
	accountID, err := parseID("accountId", req.AccountId)
	if err != nil {
		return nil, err
	}
	affectedUserID, err := parseID("userId", req.UserId)
	if err != nil {
		return nil, err
	}
	role, err := model.RoleFromAlias(req.Role)
	if err != nil {
		return nil, err
	}

	if err := s.requireOwnerAdmin(ctx, userID, accountID); err != nil {
		return nil, err
	}

	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		grant, gerr := s.access.Get(txCtx, accountID, affectedUserID)
		if gerr != nil {
			var nf *errs.NotFoundError
			if !errors.As(gerr, &nf) {
				return gerr
			}
			// First grant: create it, seed the affected user's ordering row and
			// add the account to their last folder.
			grant = model.NewAccountAccess(accountID, affectedUserID, role, now)

			max, perr := s.options.MaxPosition(txCtx, affectedUserID)
			if perr != nil {
				return perr
			}
			if perr := s.options.SavePosition(txCtx, accountID, affectedUserID, max+1, now); perr != nil {
				return perr
			}

			if folderID, ok, ferr := s.folders.LastFolderID(txCtx, affectedUserID); ferr != nil {
				return ferr
			} else if ok {
				if aerr := s.folders.AddAccount(txCtx, folderID, accountID); aerr != nil {
					return aerr
				}
			}
		}

		grant.UpdateRole(role, now)
		return s.access.Save(txCtx, grant)
	})
	if err != nil {
		return nil, err
	}
	return &model.SetAccountAccessResult{}, nil
}
