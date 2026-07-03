// Order use case: reposition the user's accounts and move them between folders.
package account

import (
	"context"

	"github.com/econumo/econumo/internal/shared/vo"
)

// OrderAccountList applies each {id, folderId, position} change: it moves the
// account into the named folder (removing it from any other of the user's
// folders) and updates its per-user position (accounts_options). Changes
// referencing an account the user does not own are ignored. Returns the full
// account list (NOT reversed, unlike get-account-list).
func (s *Service) OrderAccountList(ctx context.Context, userID vo.Id, req OrderAccountListRequest) (*OrderAccountListResult, error) {
	// Parse + validate all ids up front.
	type change struct {
		accountID vo.Id
		folderID  vo.Id
		position  int16
	}
	changes := make([]change, 0, len(req.Changes))
	for _, c := range req.Changes {
		aid, err := vo.ParseId(c.Id)
		if err != nil {
			return nil, err
		}
		fid, err := vo.ParseId(c.FolderId)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change{accountID: aid, folderID: fid, position: int16(c.Position)})
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		accts, err := s.repo.ListAvailable(ctx, userID)
		if err != nil {
			return err
		}
		owned := make(map[string]*Account, len(accts))
		for _, a := range accts {
			owned[a.Id().String()] = a
		}
		folders, err := s.folders.ListByUser(ctx, userID)
		if err != nil {
			return err
		}
		memberships, err := s.folders.MembershipsByUser(ctx, userID)
		if err != nil {
			return err
		}
		now := s.clock.Now()

		for _, ch := range changes {
			acct, ok := owned[ch.accountID.String()]
			if !ok {
				continue // not the user's account — ignore
			}
			// Move folders: for every user folder, ensure membership matches the
			// requested folderId (add to the target, remove from the others).
			for _, f := range folders {
				ids := memberships[f.Id().String()]
				contains := false
				for _, aid := range ids {
					if aid == ch.accountID.String() {
						contains = true
						break
					}
				}
				if f.Id().Equal(ch.folderID) {
					if !contains {
						if aerr := s.folders.AddAccount(ctx, f.Id(), ch.accountID); aerr != nil {
							return aerr
						}
					}
				} else if contains {
					if rerr := s.folders.RemoveAccount(ctx, f.Id(), ch.accountID); rerr != nil {
						return rerr
					}
				}
			}
			// Update the per-user position (upsert accounts_options).
			if serr := s.repo.SavePosition(ctx, acct.Id(), userID, ch.position, now); serr != nil {
				return serr
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	list, err := s.buildAccountList(ctx, userID, false)
	if err != nil {
		return nil, err
	}
	return &OrderAccountListResult{Items: list}, nil
}
