package budget

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
	"github.com/econumo/econumo/internal/shared/sortkey"
	"github.com/econumo/econumo/internal/shared/vo"
)

// CreateFolder adds a budget folder (canUpdate) at the end, renumbering positions.
func (s *Service) CreateFolder(ctx context.Context, userID vo.Id, req model.CreateBudgetFolderRequest) (*model.CreateBudgetFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Folder", req.Name); err != nil {
		return nil, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	var created *model.BudgetFolder
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if ferr := s.requireFreeFolderID(txCtx, folderID); ferr != nil {
			return ferr
		}
		// A new folder lands at the FRONT, not appended at the end. With sort keys
		// that is one write: a key below the current first, with no renumbering.
		existing := append([]*model.BudgetFolder(nil), b.folders...)
		sortBudgetFolders(existing)
		key, kerr := sortkey.Prepend(existing, budgetFolderItem, sortkey.GrowsDown)
		if kerr != nil {
			return kerr
		}
		created = model.NewBudgetFolder(folderID, budgetID, req.Name, now)
		created.SetSortKey(key)
		return s.folders.SaveFolder(txCtx, created)
	})
	if err != nil {
		return nil, err
	}
	return &model.CreateBudgetFolderResult{Item: model.BudgetFolderResult{
		Id: created.ID.String(), Name: created.Name, Position: folderIndex(b.folders, created),
	}}, nil
}

// UpdateFolder renames a budget folder (canUpdate).
func (s *Service) UpdateFolder(ctx context.Context, userID vo.Id, req model.UpdateBudgetFolderRequest) (*model.UpdateBudgetFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	if err := model.ValidateName("Folder", req.Name); err != nil {
		return nil, err
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if !b.hasFolder(folderID) {
		return nil, accessDenied()
	}
	now := s.clock.Now()
	var updated *model.BudgetFolder
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		f, gerr := s.folders.GetFolder(txCtx, folderID)
		if gerr != nil {
			return gerr
		}
		f.UpdateName(req.Name, now)
		updated = f
		return s.folders.SaveFolder(txCtx, f)
	})
	if err != nil {
		return nil, err
	}
	return &model.UpdateBudgetFolderResult{Item: model.BudgetFolderResult{
		Id: updated.ID.String(), Name: updated.Name, Position: folderIndex(b.folders, updated),
	}}, nil
}

// DeleteFolder removes a budget folder (canUpdate) and renumbers the rest.
func (s *Service) DeleteFolder(ctx context.Context, userID vo.Id, req model.DeleteFolderRequest) (*model.DeleteFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"id": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	if !b.hasFolder(folderID) {
		return nil, accessDenied()
	}
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		// Removing a row leaves the surviving keys correctly ordered, so there is
		// nothing to renumber.
		return s.folders.DeleteFolder(txCtx, folderID)
	})
	if err != nil {
		return nil, err
	}
	return &model.DeleteFolderResult{}, nil
}

// MoveFolder places one budget folder immediately after req.AfterId (nil =
// first). Budget folders are shared per budget, so this is gated on canUpdate:
// any editor reorders for everyone. Exactly one row is written -- the endpoint
// this replaces saved every folder in the request unconditionally.
func (s *Service) MoveFolder(ctx context.Context, userID vo.Id, req model.MoveBudgetFolderRequest) (*model.MoveBudgetFolderResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	folderID, err := vo.ParseId(req.Id)
	if err != nil {
		return nil, err
	}
	afterID := ""
	if req.AfterId != nil {
		afterID = *req.AfterId
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}

	folders := append([]*model.BudgetFolder(nil), b.folders...)
	sortBudgetFolders(folders)
	key, found, err := sortkey.Relocate(folders, folderID.String(), afterID, budgetFolderItem, sortkey.GrowsDown)
	if err != nil {
		return nil, err
	}
	if !found {
		return &model.MoveBudgetFolderResult{}, nil
	}
	now := s.clock.Now()
	if err := s.tx.WithTx(ctx, func(txCtx context.Context) error {
		for _, f := range folders {
			if !f.ID.Equal(folderID) {
				continue
			}
			f.UpdateSortKey(key, now)
			return s.folders.SaveFolder(txCtx, f)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &model.MoveBudgetFolderResult{}, nil
}

func budgetFolderItem(f *model.BudgetFolder) sortkey.Item {
	return sortkey.Item{ID: f.ID.String(), Key: f.SortKey}
}

// sortBudgetFolders orders folders by key with an id tie-break, matching the
// query's ORDER BY so in-memory and SQL ordering cannot diverge.
func sortBudgetFolders(folders []*model.BudgetFolder) {
	sort.SliceStable(folders, func(i, j int) bool {
		if folders[i].SortKey != folders[j].SortKey {
			return folders[i].SortKey < folders[j].SortKey
		}
		return folders[i].ID.String() < folders[j].ID.String()
	})
}

// folderIndex returns the dense 0-based index target occupies once folders are
// key-ordered. Single-item responses must derive "position" the same way the
// budget structure does, because the folder no longer stores one. target may be
// absent from folders (a just-created row), in which case its own key decides
// where it would land.
func folderIndex(folders []*model.BudgetFolder, target *model.BudgetFolder) int {
	all := append([]*model.BudgetFolder(nil), folders...)
	seen := false
	for _, f := range all {
		if f.ID.Equal(target.ID) {
			seen = true
			break
		}
	}
	if !seen {
		all = append(all, target)
	}
	sortBudgetFolders(all)
	for i, f := range all {
		if f.ID.Equal(target.ID) {
			return i
		}
	}
	return 0
}
