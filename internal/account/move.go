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

	// Legacy data can hold accounts with NO accounts_options row (the old read
	// path tolerated it as "position 0", so the migration had nothing to
	// backfill). Their empty keys all compare equal and sort first, so no real
	// key can land BETWEEN two of them -- anchoring on one would drop the moved
	// account after every keyless sibling. Give them keys in their current
	// order first; Resequence rewrites exactly the rows whose key breaks the
	// sequence, which here is exactly the keyless ones.
	if len(items) > 0 && items[0].Key == "" {
		order := make([]string, 0, len(items))
		for _, it := range items {
			order = append(order, it.ID)
		}
		changed, rerr := sortkey.Resequence(items, order, func(i sortkey.Item) sortkey.Item { return i }, sortkey.GrowsUp)
		if rerr != nil {
			return rerr
		}
		now := s.clock.Now()
		for i := range items {
			k, ok := changed[items[i].ID]
			if !ok {
				continue
			}
			id, perr := vo.ParseId(items[i].ID)
			if perr != nil {
				return perr
			}
			if serr := s.positions.SaveSortKey(ctx, id, userID, k, now); serr != nil {
				return serr
			}
			items[i].Key = k
		}
	}

	// The moved item is discarded: an account's key lives in accounts_options,
	// keyed by id, so there is no entity here to apply it to.
	_, key, ok, err := sortkey.MoveWithin(items, accountID.String(), afterID, func(i sortkey.Item) sortkey.Item { return i }, sortkey.GrowsUp)
	if err != nil || !ok {
		return err
	}
	return s.positions.SaveSortKey(ctx, accountID, userID, key, s.clock.Now())
}
