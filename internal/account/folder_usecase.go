// Folder use cases: create, update, hide, show, replace, order. Folders group a
// user's accounts; they have a per-user position and a visibility flag.
package account

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateFolder creates a folder for the user. The name must be unique among the
// user's folders; the position is last+1. Returns {item}.
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
	return &model.CreateFolderResult{Item: toFolderResult(created)}, nil
}

// createFolderTx creates a folder for the user within the caller's tx: the name
// must be unique among the user's folders and the position is last+1. Shared by
// CreateFolder and the first-account default-folder path in CreateAccount.
func (s *Service) createFolderTx(ctx context.Context, userID vo.Id, name string) (*model.Folder, error) {
	folders, lerr := s.folders.ListByUser(ctx, userID)
	if lerr != nil {
		return nil, lerr
	}
	var maxPos int16
	for _, f := range folders {
		if f.Name == name {
			return nil, &errs.ValidationError{Msg: "Folder already exists.", MsgCode: errs.CodeAccountFolderAlreadyExists}
		}
		if f.Position > maxPos {
			maxPos = f.Position
		}
	}
	now := s.clock.Now()
	f := model.NewFolder(s.folders.NextIdentity(), userID, name, now)
	f.SetPosition(maxPos + 1)
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
	return &model.UpdateFolderResult{Item: toFolderResult(updated)}, nil
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

		// Delete the folder (its accounts_folders rows cascade).
		if derr := s.folders.Delete(ctx, id); derr != nil {
			return derr
		}

		// Re-number remaining folders 0..n by current position.
		return s.resetFolderPositions(ctx, userID)
	}); err != nil {
		return nil, err
	}
	return &model.ReplaceFolderResult{}, nil
}

// resetFolderPositions renumbers the user's folders to 0..n-1 ordered by their
// current position. Runs inside the caller's tx.
func (s *Service) resetFolderPositions(ctx context.Context, userID vo.Id) error {
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position < folders[j].Position })
	now := s.clock.Now()
	for i, f := range folders {
		f.UpdatePosition(int16(i), now)
		if serr := s.folders.Save(ctx, f); serr != nil {
			return serr
		}
	}
	return nil
}

// OrderFolderList applies {id, position} changes to the user's folders, saving
// only those that actually changed, then returns the full ordered list.
func (s *Service) OrderFolderList(ctx context.Context, userID vo.Id, req model.OrderFolderListRequest) (*model.OrderFolderListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, c := range req.Changes {
		fid, err := vo.ParseId(c.Id)
		if err != nil {
			return nil, err
		}
		positions[fid.String()] = int16(c.Position)
	}

	var items []model.AccountFolderResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		folders, lerr := s.folders.ListByUser(ctx, userID)
		if lerr != nil {
			return lerr
		}
		now := s.clock.Now()
		for _, f := range folders {
			pos, ok := positions[f.ID.String()]
			if !ok {
				continue
			}
			before := f.Position
			f.UpdatePosition(pos, now)
			if f.Position != before {
				if serr := s.folders.Save(ctx, f); serr != nil {
					return serr
				}
			}
		}
		sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position < folders[j].Position })
		items = make([]model.AccountFolderResult, 0, len(folders))
		for _, f := range folders {
			items = append(items, toFolderResult(f))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.OrderFolderListResult{Items: items}, nil
}
