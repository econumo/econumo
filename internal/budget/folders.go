package budget

import (
	"context"
	"sort"

	"github.com/econumo/econumo/internal/model"
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
		// Insert the new folder at position 0 and renumber the existing folders
		// 1,2,3,... in their current position-ASC order, so the new folder lands at
		// the FRONT, not appended at the end.
		created = model.NewBudgetFolder(folderID, budgetID, req.Name, 0, now)
		if serr := s.folders.SaveFolder(txCtx, created); serr != nil {
			return serr
		}
		existing := append([]*model.BudgetFolder(nil), b.folders...)
		sort.SliceStable(existing, func(i, j int) bool { return existing[i].Position < existing[j].Position })
		pos := int16(0)
		for _, f := range existing {
			pos++
			if f.Position == pos {
				continue
			}
			f.UpdatePosition(pos, now)
			if serr := s.folders.SaveFolder(txCtx, f); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.CreateBudgetFolderResult{Item: model.BudgetFolderResult{Id: created.ID.String(), Name: created.Name, Position: int(created.Position)}}, nil
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
	return &model.UpdateBudgetFolderResult{Item: model.BudgetFolderResult{Id: updated.ID.String(), Name: updated.Name, Position: int(updated.Position)}}, nil
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
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		if derr := s.folders.DeleteFolder(txCtx, folderID); derr != nil {
			return derr
		}
		// Renumber remaining folders 0..n by their current position order.
		remaining := make([]*model.BudgetFolder, 0, len(b.folders))
		for _, f := range b.folders {
			if !f.ID.Equal(folderID) {
				remaining = append(remaining, f)
			}
		}
		sort.SliceStable(remaining, func(i, j int) bool { return remaining[i].Position < remaining[j].Position })
		for i, f := range remaining {
			f.UpdatePosition(int16(i), now)
			if serr := s.folders.SaveFolder(txCtx, f); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.DeleteFolderResult{}, nil
}

// OrderFolderList applies new positions to budget folders (canUpdate).
func (s *Service) OrderFolderList(ctx context.Context, userID vo.Id, req model.OrderBudgetFolderListRequest) (*model.OrderBudgetFolderListResult, error) {
	budgetID, err := vo.ParseId(req.BudgetId)
	if err != nil {
		return nil, model.ValidateBlank(map[string]string{"budgetId": ""})
	}
	b, err := s.loadAggregate(ctx, budgetID)
	if err != nil {
		return nil, err
	}
	if !s.canUpdate(b, userID) {
		return nil, accessDenied()
	}
	byID := map[string]*model.BudgetFolder{}
	for _, f := range b.folders {
		byID[f.ID.String()] = f
	}
	now := s.clock.Now()
	err = s.tx.WithTx(ctx, func(txCtx context.Context) error {
		for _, item := range req.Items {
			f := byID[item.Id]
			if f == nil {
				continue
			}
			f.UpdatePosition(int16(item.Position), now)
			if serr := s.folders.SaveFolder(txCtx, f); serr != nil {
				return serr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.OrderBudgetFolderListResult{}, nil
}
