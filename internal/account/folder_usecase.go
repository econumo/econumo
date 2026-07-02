// Folder use cases: create, update, hide, show, replace, order. Folders group a
// user's accounts; they have a per-user position and a visibility flag.
package account

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/shared/errs"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateFolder creates a folder for the user. The name must be unique among the
// user's folders; the position is last+1. Returns {item}.
func (s *Service) CreateFolder(ctx context.Context, userID vo.Id, req CreateFolderRequest) (*CreateFolderResult, error) {
	name, err := newFolderName(req.Name)
	if err != nil {
		return nil, err
	}

	var created *Folder
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
	return &CreateFolderResult{Item: toFolderResult(created)}, nil
}

// createFolderTx creates a folder for the user within the caller's tx: the name
// must be unique among the user's folders and the position is last+1. Shared by
// CreateFolder and the first-account default-folder path in CreateAccount.
func (s *Service) createFolderTx(ctx context.Context, userID vo.Id, name string) (*Folder, error) {
	folders, lerr := s.folders.ListByUser(ctx, userID)
	if lerr != nil {
		return nil, lerr
	}
	var maxPos int16
	for _, f := range folders {
		if f.Name() == name {
			return nil, errs.NewValidation("Folder already exists.")
		}
		if f.Position() > maxPos {
			maxPos = f.Position()
		}
	}
	now := s.clock.Now()
	f := NewFolder(s.folders.NextIdentity(), userID, name, now)
	f.SetPosition(maxPos + 1)
	if serr := s.folders.Save(ctx, f); serr != nil {
		return nil, serr
	}
	return f, nil
}

// UpdateFolder renames a folder the user owns. The new name must be unique among
// the user's other folders. Returns {item}.
func (s *Service) UpdateFolder(ctx context.Context, userID vo.Id, req UpdateFolderRequest) (*UpdateFolderResult, error) {
	id, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	name, err := newFolderName(req.Name)
	if err != nil {
		return nil, err
	}

	var updated *Folder
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		f, gerr := s.folders.GetByID(ctx, id)
		if gerr != nil {
			return gerr
		}
		if !f.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		folders, lerr := s.folders.ListByUser(ctx, userID)
		if lerr != nil {
			return lerr
		}
		for _, other := range folders {
			if other.Name() == name && !other.Id().Equal(id) {
				return errs.NewValidation("Folder already exists.")
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
	return &UpdateFolderResult{Item: toFolderResult(updated)}, nil
}

// HideFolder marks a folder (and its accounts) hidden. Ownership required.
func (s *Service) HideFolder(ctx context.Context, userID vo.Id, req HideFolderRequest) (*HideFolderResult, error) {
	if err := s.toggleVisibility(ctx, userID, req.Id, false); err != nil {
		return nil, err
	}
	return &HideFolderResult{}, nil
}

// ShowFolder clears a folder's hidden flag. Ownership required.
func (s *Service) ShowFolder(ctx context.Context, userID vo.Id, req ShowFolderRequest) (*ShowFolderResult, error) {
	if err := s.toggleVisibility(ctx, userID, req.Id, true); err != nil {
		return nil, err
	}
	return &ShowFolderResult{}, nil
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
		if !f.UserId().Equal(userID) {
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
func (s *Service) ReplaceFolder(ctx context.Context, userID vo.Id, req ReplaceFolderRequest) (*ReplaceFolderResult, error) {
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
		if !f.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}
		replace, rerr := s.folders.GetByID(ctx, replaceID)
		if rerr != nil {
			return rerr
		}
		if !replace.UserId().Equal(userID) {
			return errs.NewAccessDenied("Access denied")
		}

		// Move the folder's accounts into the replacement (idempotent add).
		accountIDs, aerr := s.folders.FolderAccountIDs(ctx, id)
		if aerr != nil {
			return aerr
		}
		for _, aid := range accountIDs {
			accountID, perr := vo.ParseId(aid)
			if perr != nil {
				return perr
			}
			if addErr := s.folders.AddAccount(ctx, replaceID, accountID); addErr != nil {
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
	return &ReplaceFolderResult{}, nil
}

// resetFolderPositions renumbers the user's folders to 0..n-1 ordered by their
// current position. Runs inside the caller's tx.
func (s *Service) resetFolderPositions(ctx context.Context, userID vo.Id) error {
	folders, err := s.folders.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position() < folders[j].Position() })
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
func (s *Service) OrderFolderList(ctx context.Context, userID vo.Id, req OrderFolderListRequest) (*OrderFolderListResult, error) {
	positions := make(map[string]int16, len(req.Changes))
	for _, c := range req.Changes {
		fid, err := vo.ParseId(c.Id)
		if err != nil {
			return nil, err
		}
		positions[fid.String()] = int16(c.Position)
	}

	var items []FolderResult
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		folders, lerr := s.folders.ListByUser(ctx, userID)
		if lerr != nil {
			return lerr
		}
		now := s.clock.Now()
		for _, f := range folders {
			pos, ok := positions[f.Id().String()]
			if !ok {
				continue
			}
			before := f.Position()
			f.UpdatePosition(pos, now)
			if f.Position() != before {
				if serr := s.folders.Save(ctx, f); serr != nil {
					return serr
				}
			}
		}
		sort.SliceStable(folders, func(i, j int) bool { return folders[i].Position() < folders[j].Position() })
		items = make([]FolderResult, 0, len(folders))
		for _, f := range folders {
			items = append(items, toFolderResult(f))
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &OrderFolderListResult{Items: items}, nil
}
