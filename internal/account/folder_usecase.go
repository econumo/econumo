// Folder use cases: create, update, hide, show, replace, move. Folders group a
// user's accounts; they have a per-user sort key and a visibility flag.
package account

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateFolder creates a folder for the user. The name must be unique among the
// user's folders; the new folder appends to the end. Returns {item}.
func (s *Service) CreateFolder(ctx context.Context, userID vo.Id, req model.CreateFolderRequest) (*model.CreateFolderResult, error) {
	name, err := newFolderName(req.Name)
	if err != nil {
		return nil, err
	}

	var created *model.Folder
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		f, cerr := s.createFolderTx(ctx, userID, name)
		if cerr != nil {
			return cerr
		}
		created = f
		return nil
	}); err != nil {
		return nil, err
	}
	item := toFolderResult(created)
	idx, err := s.folderIndex(ctx, userID, created.ID)
	if err != nil {
		return nil, err
	}
	item.Position = idx
	return &model.CreateFolderResult{Item: item}, nil
}

// createFolderTx creates a folder for the user within the caller's tx: the name
// must be unique among the user's folders and the position is last+1. Shared by
// CreateFolder and the first-account default-folder path in CreateAccount.
func (s *Service) createFolderTx(ctx context.Context, userID vo.Id, name string) (*model.Folder, error) {
	folders, lerr := s.folders.ListByUser(ctx, userID)
	if lerr != nil {
		return nil, lerr
	}
	for _, f := range folders {
		if f.Name == name {
			return nil, &errs.ValidationError{Msg: "Folder already exists.", MsgCode: errs.CodeAccountFolderAlreadyExists}
		}
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].SortKey < folders[j].SortKey })
	key, kerr := sortkey.Append(folders, accountFolderItem, sortkey.GrowsUp)
	if kerr != nil {
		return nil, kerr
	}
	now := s.clock.Now()
	f := model.NewFolder(s.folders.NextIdentity(), userID, name, now)
	f.SetSortKey(key)
	if serr := s.folders.Save(ctx, f); serr != nil {
		return nil, serr
	}
	return f, nil
}

// UpdateFolder renames a folder the user owns. The new name must be unique among
// the user's other folders. Returns {item}.
func (s *Service) UpdateFolder(ctx context.Context, userID vo.Id, req model.UpdateFolderRequest) (*model.UpdateFolderResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newFolderName(req.Name)
	if err != nil {
		return nil, err
	}

	var updated *model.Folder
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		f, gerr := s.folders.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !f.UserID.Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		folders, lerr := s.folders.ListByUser(ctx, userID)
		if lerr != nil {
			return lerr
		}
		for _, other := range folders {
			if other.Name == name && !other.ID.Equal(id) {
				return &errs.ValidationError{Msg: "Folder already exists.", MsgCode: errs.CodeAccountFolderAlreadyExists}
			}
		}
		f.UpdateName(name, s.clock.Now())
		if serr := s.folders.Save(ctx, f); serr != nil {
			return serr
		}
		updated = f
		return nil
	}); err != nil {
		return nil, err
	}
	item := toFolderResult(updated)
	idx, err := s.folderIndex(ctx, userID, updated.ID)
	if err != nil {
		return nil, err
	}
	item.Position = idx
	return &model.UpdateFolderResult{Item: item}, nil
}

// HideFolder marks a folder (and its accounts) hidden. Ownership required.
func (s *Service) HideFolder(ctx context.Context, userID vo.Id, req model.HideFolderRequest) (*model.HideFolderResult, error) {
	if err := s.toggleVisibility(ctx, userID, req.Id, false); err != nil {
		return nil, err
	}
	return &model.HideFolderResult{}, nil
}

// ShowFolder clears a folder's hidden flag. Ownership required.
func (s *Service) ShowFolder(ctx context.Context, userID vo.Id, req model.ShowFolderRequest) (*model.ShowFolderResult, error) {
	if err := s.toggleVisibility(ctx, userID, req.Id, true); err != nil {
		return nil, err
	}
	return &model.ShowFolderResult{}, nil
}

func (s *Service) toggleVisibility(ctx context.Context, userID vo.Id, rawID string, visible bool) error {
	id, err := vo.ParseId(rawID)
	if err != nil {
		return err
	}
	return s.tx.WithTx(ctx, func(ctx context.Context) error {
		f, gerr := s.folders.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !f.UserID.Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		now := s.clock.Now()
		if visible {
			f.MakeVisible(now)
		} else {
			f.MakeInvisible(now)
		}
		return s.folders.Save(ctx, f)
	})
}

// ReplaceFolder moves all of a folder's accounts into replaceId, deletes the
// folder, and re-numbers the remaining folders' positions (0..n by position).
// Both folders must belong to the user. Returns {}.
func (s *Service) ReplaceFolder(ctx context.Context, userID vo.Id, req model.ReplaceFolderRequest) (*model.ReplaceFolderResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	replaceID, err := vo.ParseId(req.ReplaceId)
	if err != nil {
		return nil, err
	}

	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		f, gerr := s.folders.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !f.UserID.Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		replace, rerr := s.folders.GetByID(ctx, replaceID)
		if rerr != nil {
			return rerr
		}
		if !replace.UserID.Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}

		// Move the folder's accounts into the replacement (idempotent add).
		accountIDs, aerr := s.memberships.FolderAccountIDs(ctx, id)
		if aerr != nil {
			return aerr
		}
		for _, aid := range accountIDs {
			accountID, perr := vo.ParseId(aid)
			if perr != nil {
				return perr
			}
			if addErr := s.memberships.AddAccount(ctx, replaceID, accountID); addErr != nil {
				return addErr
			}
		}

		// Delete the folder (its accounts_folders rows cascade). Removing a row
		// leaves the surviving keys untouched and still correctly ordered, so
		// there is nothing to renumber.
		return s.folders.Delete(ctx, id)
	}); err != nil {
		return nil, err
	}
	return &model.ReplaceFolderResult{}, nil
}

// MoveFolder places the folder immediately after req.AfterId (nil = first) and
// returns the full ordered list. Exactly one row is written; deleting or
// creating a folder no longer disturbs its siblings either, which is why the
// old renumbering helper is gone.
func (s *Service) MoveFolder(ctx context.Context, userID vo.Id, req model.MoveAccountFolderRequest) (*model.MoveAccountFolderResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}

	var items []model.AccountFolderResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		folders, lerr := s.sortedFolders(ctx, userID)
		if lerr != nil {
			return lerr
		}
		key, found, kerr := sortkey.Relocate(folders, id.String(), afterID, accountFolderItem, sortkey.GrowsUp)
		if kerr != nil {
			return kerr
		}
		if found {
			for _, f := range folders {
				if !f.ID.Equal(id) {
					continue
				}
				f.UpdateSortKey(key, s.clock.Now())
				if serr := s.folders.Save(ctx, f); serr != nil {
					return serr
				}
				break
			}
		}
		refreshed, rerr := s.sortedFolders(ctx, userID)
		if rerr != nil {
			return rerr
		}
		items = make([]model.AccountFolderResult, 0, len(refreshed))
		for i, f := range refreshed {
			res := toFolderResult(f)
			res.Position = i
			items = append(items, res)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.MoveAccountFolderResult{Items: items}, nil
}

func accountFolderItem(f *model.Folder) sortkey.Item {
	return sortkey.Item{ID: f.ID.String(), Key: f.SortKey}
}
