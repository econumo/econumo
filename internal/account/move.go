// Move use case: reposition one account and set which folder it lives in.
package account

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// MoveAccount places the account immediately after req.AfterId (nil = first) and
// moves it into req.FolderId (nil = no folder), removing it from any other of
// the user's folders. An account the user cannot see is ignored rather than
// rejected, matching the endpoint this replaces.
//
// Both the ordering key and the folder membership are per-user, so a shared
// account can sit in a different slot and folder for each participant. The
// response list is rebuilt INSIDE the transaction, unlike the endpoint this
// replaces, so it can never observe a partially applied move.
func (s *Service) MoveAccount(ctx context.Context, userID vo.Id, req model.MoveAccountRequest) (*model.MoveAccountResult, error) {
	accountID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	var targetFolder *vo.Id
	if req.FolderId != nil {
		fid, ferr := vo.ParseId(*req.FolderId)
		if ferr != nil {
			return nil, ferr
		}
		targetFolder = &fid
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.AccountResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		accts, aerr := s.accounts.ListAvailable(ctx, userID)
		if aerr != nil {
			return aerr
		}
		visible := false
		for _, a := range accts {
			if a.ID.Equal(accountID) {
				visible = true
				break
			}
		}
		if visible {
			if merr := s.setAccountFolder(ctx, userID, accountID, targetFolder); merr != nil {
				return merr
			}
			if kerr := s.reorderAccount(ctx, userID, accountID, afterID, accts); kerr != nil {
				return kerr
			}
		}
		built, berr := s.buildAccountList(ctx, userID, false)
		if berr != nil {
			return berr
		}
		items = built
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.MoveAccountResult{Items: items}, nil
}

// setAccountFolder makes target the ONLY one of the user's folders holding the
// account; a nil target removes it from all of them.
func (s *Service) setAccountFolder(ctx context.Context, userID, accountID vo.Id, target *vo.Id) error {
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	memberships, err := s.memberships.MembershipsByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, f := range folders {
		contains := false
		for _, aid := range memberships[f.ID.String()] {
			if aid == accountID.String() {
				contains = true
				break
			}
		}
		wanted := target != nil && f.ID.Equal(*target)
		switch {
		case wanted && !contains:
			if aerr := s.memberships.AddAccount(ctx, f.ID, accountID); aerr != nil {
				return aerr
			}
		case !wanted && contains:
			if rerr := s.memberships.RemoveAccount(ctx, f.ID, accountID); rerr != nil {
				return rerr
			}
		}
	}
	return nil
}

// reorderAccount writes the moved account's new per-user key, and nothing else.
func (s *Service) reorderAccount(ctx context.Context, userID, accountID vo.Id, afterID string, accts []*model.Account) error {
	keys, err := s.positions.SortKeysByUser(ctx, userID)
	if err != nil {
		return err
	}
	items := make([]sortkey.Item, 0, len(accts))
	for _, a := range accts {
		items = append(items, sortkey.Item{ID: a.ID.String(), Key: keys[a.ID.String()]})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Key != items[j].Key {
			return items[i].Key < items[j].Key
		}
		return items[i].ID < items[j].ID
	})
	key, found, err := sortkey.Relocate(items, accountID.String(), afterID, func(i sortkey.Item) sortkey.Item { return i }, sortkey.GrowsUp)
	if err != nil || !found {
		return err
	}
	return s.positions.SaveSortKey(ctx, accountID, userID, key, s.clock.Now())
}
